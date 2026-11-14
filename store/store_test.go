package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newTestStore creates an in-memory store for testing.
// Fails the test immediately if store creation fails.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("New(:memory:) failed: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// testPage returns a Page with sensible defaults for testing.
func testPage(name string) *Page {
	return &Page{
		Name:         name,
		OriginalName: name,
		SourceID:     "disk-local",
		SourceDocID:  name + ".md",
		Properties:   `{"tags": ["test"]}`,
		ContentHash:  "abc123",
		IsJournal:    false,
		CreatedAt:    1000,
		UpdatedAt:    1000,
	}
}

func TestNew_InMemory(t *testing.T) {
	s, err := New("")
	if err != nil {
		t.Fatalf("New('') failed: %v", err)
	}
	defer func() { _ = s.Close() }()

	// Verify returned store is non-nil with accessible DB handle.
	if s == nil {
		t.Fatal("New('') returned nil store")
	}
	if s.DB() == nil {
		t.Fatal("store.DB() returned nil")
	}

	// Verify WAL mode is set.
	var journalMode string
	if err := s.db.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	// In-memory databases may report "memory" instead of "wal".
	if journalMode != "wal" && journalMode != "memory" {
		t.Errorf("journal_mode = %q, want wal or memory", journalMode)
	}

	// Verify foreign keys are enabled.
	var fk int
	if err := s.db.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("query foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys = %d, want 1", fk)
	}

	// Verify Close releases resources without error.
	if err := s.Close(); err != nil {
		t.Errorf("Close() returned error: %v", err)
	}
}

func TestNew_InvalidPath(t *testing.T) {
	// Non-existent nested directory should fail.
	_, err := New("/nonexistent/deeply/nested/path/that/cannot/exist/test.db")
	if err == nil {
		t.Fatal("New with invalid path should return error")
	}
}

func TestNew_FileBacked_CloseReleasesResources(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "close-test.db")

	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("New(%q): %v", dbPath, err)
	}

	// Verify store is functional.
	if err := s.InsertPage(testPage("close-page")); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	// Close should release resources without error.
	if err := s.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}

	// After close, re-opening should succeed (lock released).
	s2, err := New(dbPath)
	if err != nil {
		t.Fatalf("New after Close: %v (lock may not have been released)", err)
	}
	defer func() { _ = s2.Close() }()

	// Verify data persisted.
	got, err := s2.GetPage("close-page")
	if err != nil {
		t.Fatalf("GetPage after reopen: %v", err)
	}
	if got == nil {
		t.Fatal("page should persist after close and reopen")
	}
}

func TestNew_SchemaVersion(t *testing.T) {
	s := newTestStore(t)

	version, err := s.GetMeta("schema_version")
	if err != nil {
		t.Fatalf("GetMeta(schema_version): %v", err)
	}
	want := fmt.Sprintf("%d", schemaVersion)
	if version != want {
		t.Errorf("schema_version = %q, want %q", version, want)
	}
}

func TestNew_IdempotentMigration(t *testing.T) {
	s := newTestStore(t)

	// Insert a page to verify data survives re-migration.
	if err := s.InsertPage(testPage("test-page")); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	// Re-run migration manually.
	if err := s.migrate(); err != nil {
		t.Fatalf("second migrate() failed: %v", err)
	}

	// Verify page still exists.
	p, err := s.GetPage("test-page")
	if err != nil {
		t.Fatalf("GetPage after re-migrate: %v", err)
	}
	if p == nil {
		t.Fatal("page lost after re-migration")
	}
}

// --- Page CRUD Tests ---

func TestInsertPage_Success(t *testing.T) {
	s := newTestStore(t)
	p := testPage("my-page")

	if err := s.InsertPage(p); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	got, err := s.GetPage("my-page")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if got == nil {
		t.Fatal("GetPage returned nil")
	}
	if got.Name != "my-page" {
		t.Errorf("Name = %q, want %q", got.Name, "my-page")
	}
	if got.OriginalName != "my-page" {
		t.Errorf("OriginalName = %q, want %q", got.OriginalName, "my-page")
	}
	if got.SourceID != "disk-local" {
		t.Errorf("SourceID = %q, want %q", got.SourceID, "disk-local")
	}
	if got.ContentHash != "abc123" {
		t.Errorf("ContentHash = %q, want %q", got.ContentHash, "abc123")
	}
	if got.IsJournal {
		t.Error("IsJournal = true, want false")
	}
	if got.CreatedAt != 1000 {
		t.Errorf("CreatedAt = %d, want 1000", got.CreatedAt)
	}
}

func TestInsertPage_Duplicate(t *testing.T) {
	s := newTestStore(t)
	p := testPage("dup-page")

	if err := s.InsertPage(p); err != nil {
		t.Fatalf("first InsertPage: %v", err)
	}
	if err := s.InsertPage(p); err == nil {
		t.Fatal("second InsertPage should fail for duplicate")
	}
}

func TestGetPageFoldCase_FindsPageRegardlessOfCase(t *testing.T) {
	// Learning page names carry an uppercase "T" in their ISO timestamps,
	// and the in-memory vault tier keys pages case-insensitively — the
	// store fallback must therefore fold case in SQL rather than by
	// normalizing the input (which would miss the stored name entirely).
	s := newTestStore(t)
	if err := s.InsertPage(&Page{
		Name:         "learning/auth-20261101T235959-alice",
		OriginalName: "auth",
		SourceID:     "learning",
		ContentHash:  "abc123",
		CreatedAt:    1000,
	}); err != nil {
		t.Fatalf("InsertPage() error = %v", err)
	}

	for _, lookup := range []string{
		"learning/auth-20261101T235959-alice", // exact
		"learning/auth-20261101t235959-alice", // lowercase t
		"LEARNING/AUTH-20261101T235959-ALICE", // full uppercase
	} {
		p, err := s.GetPageFoldCase(lookup)
		if err != nil {
			t.Errorf("GetPageFoldCase(%q) error = %v", lookup, err)
			continue
		}
		if p == nil {
			t.Errorf("GetPageFoldCase(%q) = nil, want the stored page", lookup)
			continue
		}
		if p.Name != "learning/auth-20261101T235959-alice" {
			t.Errorf("GetPageFoldCase(%q).Name = %q, want canonical stored name", lookup, p.Name)
		}
	}

	// The exact-match lookup keeps its case sensitivity — folding is
	// scoped to the fold-case variant, not the whole store API.
	if p, err := s.GetPage("learning/auth-20261101t235959-alice"); err != nil || p != nil {
		t.Errorf("GetPage(wrong case) = (%p, %v), want (nil, nil)", p, err)
	}
}

func TestGetPageFoldCase_NotFound(t *testing.T) {
	s := newTestStore(t)
	p, err := s.GetPageFoldCase("learning/does-not-exist")
	if err != nil {
		t.Fatalf("GetPageFoldCase() error = %v", err)
	}
	if p != nil {
		t.Errorf("GetPageFoldCase() = %v, want nil", p)
	}
}

func TestGetPage_NotFound(t *testing.T) {
	s := newTestStore(t)

	got, err := s.GetPage("nonexistent")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for nonexistent page, got %+v", got)
	}
}

func TestListPages_Empty(t *testing.T) {
	s := newTestStore(t)

	pages, err := s.ListPages()
	if err != nil {
		t.Fatalf("ListPages: %v", err)
	}
	if len(pages) != 0 {
		t.Errorf("ListPages returned %d pages, want 0", len(pages))
	}
}

func TestListPages_Multiple(t *testing.T) {
	s := newTestStore(t)

	for _, name := range []string{"alpha", "beta", "gamma"} {
		if err := s.InsertPage(testPage(name)); err != nil {
			t.Fatalf("InsertPage(%s): %v", name, err)
		}
	}

	pages, err := s.ListPages()
	if err != nil {
		t.Fatalf("ListPages: %v", err)
	}
	if len(pages) != 3 {
		t.Fatalf("ListPages returned %d pages, want 3", len(pages))
	}

	// Verify alphabetical ordering.
	if pages[0].Name != "alpha" {
		t.Errorf("pages[0].Name = %q, want %q", pages[0].Name, "alpha")
	}
	if pages[1].Name != "beta" {
		t.Errorf("pages[1].Name = %q, want %q", pages[1].Name, "beta")
	}
	if pages[2].Name != "gamma" {
		t.Errorf("pages[2].Name = %q, want %q", pages[2].Name, "gamma")
	}
}

func TestUpdatePage_Success(t *testing.T) {
	s := newTestStore(t)
	p := testPage("update-me")

	if err := s.InsertPage(p); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	p.ContentHash = "new-hash-456"
	p.Properties = `{"tags": ["updated"]}`
	if err := s.UpdatePage(p); err != nil {
		t.Fatalf("UpdatePage: %v", err)
	}

	got, err := s.GetPage("update-me")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if got.ContentHash != "new-hash-456" {
		t.Errorf("ContentHash = %q, want %q", got.ContentHash, "new-hash-456")
	}
	if got.Properties != `{"tags": ["updated"]}` {
		t.Errorf("Properties = %q, want updated value", got.Properties)
	}
	if got.UpdatedAt <= p.CreatedAt {
		t.Error("UpdatedAt should be greater than CreatedAt after update")
	}
}

func TestUpdatePage_NotFound(t *testing.T) {
	s := newTestStore(t)
	p := testPage("ghost")

	if err := s.UpdatePage(p); err == nil {
		t.Fatal("UpdatePage should fail for nonexistent page")
	}
}

func TestDeletePage_Success(t *testing.T) {
	s := newTestStore(t)
	p := testPage("delete-me")

	if err := s.InsertPage(p); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}
	if err := s.DeletePage("delete-me"); err != nil {
		t.Fatalf("DeletePage: %v", err)
	}

	got, err := s.GetPage("delete-me")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if got != nil {
		t.Error("page should be nil after deletion")
	}
}

func TestDeletePage_NotFound(t *testing.T) {
	s := newTestStore(t)

	if err := s.DeletePage("ghost"); err == nil {
		t.Fatal("DeletePage should fail for nonexistent page")
	}
}

func TestDeletePage_CascadeBlocks(t *testing.T) {
	s := newTestStore(t)

	if err := s.InsertPage(testPage("cascade-page")); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}
	if err := s.InsertBlock(&Block{
		UUID:     "block-1",
		PageName: "cascade-page",
		Content:  "test content",
	}); err != nil {
		t.Fatalf("InsertBlock: %v", err)
	}

	// Delete the page — blocks should cascade.
	if err := s.DeletePage("cascade-page"); err != nil {
		t.Fatalf("DeletePage: %v", err)
	}

	block, err := s.GetBlock("block-1")
	if err != nil {
		t.Fatalf("GetBlock: %v", err)
	}
	if block != nil {
		t.Error("block should be nil after page cascade delete")
	}
}

func TestDeletePage_CascadeLinks(t *testing.T) {
	s := newTestStore(t)

	if err := s.InsertPage(testPage("link-source")); err != nil {
		t.Fatalf("InsertPage(link-source): %v", err)
	}
	if err := s.InsertPage(testPage("link-target")); err != nil {
		t.Fatalf("InsertPage(link-target): %v", err)
	}
	if err := s.InsertBlock(&Block{
		UUID:     "link-block",
		PageName: "link-source",
		Content:  "has a [[link-target]]",
	}); err != nil {
		t.Fatalf("InsertBlock: %v", err)
	}
	if err := s.InsertLink(&Link{
		FromPage:  "link-source",
		ToPage:    "link-target",
		BlockUUID: "link-block",
	}); err != nil {
		t.Fatalf("InsertLink: %v", err)
	}

	// Delete source page — links should cascade.
	if err := s.DeletePage("link-source"); err != nil {
		t.Fatalf("DeletePage: %v", err)
	}

	links, err := s.GetForwardLinks("link-source")
	if err != nil {
		t.Fatalf("GetForwardLinks: %v", err)
	}
	if len(links) != 0 {
		t.Errorf("expected 0 forward links after cascade, got %d", len(links))
	}
}

// --- Block CRUD Tests ---

func TestInsertBlock_Success(t *testing.T) {
	s := newTestStore(t)

	if err := s.InsertPage(testPage("block-page")); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	b := &Block{
		UUID:         "uuid-1",
		PageName:     "block-page",
		Content:      "## Heading\nSome content",
		HeadingLevel: 2,
		Position:     0,
	}
	if err := s.InsertBlock(b); err != nil {
		t.Fatalf("InsertBlock: %v", err)
	}

	got, err := s.GetBlock("uuid-1")
	if err != nil {
		t.Fatalf("GetBlock: %v", err)
	}
	if got == nil {
		t.Fatal("GetBlock returned nil")
	}
	if got.UUID != "uuid-1" {
		t.Errorf("UUID = %q, want %q", got.UUID, "uuid-1")
	}
	if got.PageName != "block-page" {
		t.Errorf("PageName = %q, want %q", got.PageName, "block-page")
	}
	if got.Content != "## Heading\nSome content" {
		t.Errorf("Content = %q, want expected value", got.Content)
	}
	if got.HeadingLevel != 2 {
		t.Errorf("HeadingLevel = %d, want 2", got.HeadingLevel)
	}
}

func TestInsertBlock_WithParent(t *testing.T) {
	s := newTestStore(t)

	if err := s.InsertPage(testPage("parent-page")); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	parent := &Block{
		UUID:     "parent-uuid",
		PageName: "parent-page",
		Content:  "# Parent",
	}
	if err := s.InsertBlock(parent); err != nil {
		t.Fatalf("InsertBlock(parent): %v", err)
	}

	child := &Block{
		UUID:       "child-uuid",
		PageName:   "parent-page",
		ParentUUID: sql.NullString{String: "parent-uuid", Valid: true},
		Content:    "## Child",
		Position:   0,
	}
	if err := s.InsertBlock(child); err != nil {
		t.Fatalf("InsertBlock(child): %v", err)
	}

	got, err := s.GetBlock("child-uuid")
	if err != nil {
		t.Fatalf("GetBlock: %v", err)
	}
	if !got.ParentUUID.Valid || got.ParentUUID.String != "parent-uuid" {
		t.Errorf("ParentUUID = %v, want parent-uuid", got.ParentUUID)
	}
}

func TestGetBlock_NotFound(t *testing.T) {
	s := newTestStore(t)

	got, err := s.GetBlock("nonexistent")
	if err != nil {
		t.Fatalf("GetBlock: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for nonexistent block, got %+v", got)
	}
}

func TestGetBlocksByPage_Ordered(t *testing.T) {
	s := newTestStore(t)

	if err := s.InsertPage(testPage("ordered-page")); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	for i, content := range []string{"third", "first", "second"} {
		pos := []int{2, 0, 1}[i]
		if err := s.InsertBlock(&Block{
			UUID:     content + "-uuid",
			PageName: "ordered-page",
			Content:  content,
			Position: pos,
		}); err != nil {
			t.Fatalf("InsertBlock(%s): %v", content, err)
		}
	}

	blocks, err := s.GetBlocksByPage("ordered-page")
	if err != nil {
		t.Fatalf("GetBlocksByPage: %v", err)
	}
	if len(blocks) != 3 {
		t.Fatalf("got %d blocks, want 3", len(blocks))
	}
	if blocks[0].Content != "first" {
		t.Errorf("blocks[0].Content = %q, want %q", blocks[0].Content, "first")
	}
	if blocks[1].Content != "second" {
		t.Errorf("blocks[1].Content = %q, want %q", blocks[1].Content, "second")
	}
	if blocks[2].Content != "third" {
		t.Errorf("blocks[2].Content = %q, want %q", blocks[2].Content, "third")
	}
}

func TestDeleteBlocksByPage(t *testing.T) {
	s := newTestStore(t)

	if err := s.InsertPage(testPage("del-blocks-page")); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := s.InsertBlock(&Block{
			UUID:     fmt.Sprintf("block-%d", i),
			PageName: "del-blocks-page",
			Content:  "content",
			Position: i,
		}); err != nil {
			t.Fatalf("InsertBlock: %v", err)
		}
	}

	if err := s.DeleteBlocksByPage("del-blocks-page"); err != nil {
		t.Fatalf("DeleteBlocksByPage: %v", err)
	}

	blocks, err := s.GetBlocksByPage("del-blocks-page")
	if err != nil {
		t.Fatalf("GetBlocksByPage: %v", err)
	}
	if len(blocks) != 0 {
		t.Errorf("got %d blocks after delete, want 0", len(blocks))
	}
}

// --- Block Content Search Tests (mcp-internal-store-access) ---

func TestSearchBlocksByContent_Match(t *testing.T) {
	s := newTestStore(t)

	if err := s.InsertPage(testPage("compiled/gateway")); err != nil {
		t.Fatalf("InsertPage(compiled/gateway): %v", err)
	}
	if err := s.InsertPage(testPage("notes/other")); err != nil {
		t.Fatalf("InsertPage(notes/other): %v", err)
	}

	// Matching blocks on two pages plus one non-matching block.
	insert := []Block{
		{UUID: "gw-uuid-1", PageName: "compiled/gateway", Content: "# Gateway\n\nBody text with unique-phrase.", HeadingLevel: 1, Position: 0},
		{UUID: "gw-uuid-2", PageName: "compiled/gateway", Content: "The gateway mentions unique-phrase again.", Position: 1},
		{UUID: "other-uuid", PageName: "notes/other", Content: "notes mentioning unique-phrase", Position: 0},
		{UUID: "unrelated-uuid", PageName: "notes/other", Content: "completely different topic", Position: 1},
	}
	for i := range insert {
		if err := s.InsertBlock(&insert[i]); err != nil {
			t.Fatalf("InsertBlock(%s): %v", insert[i].UUID, err)
		}
	}

	got, err := s.SearchBlocksByContent("unique-phrase", 20)
	if err != nil {
		t.Fatalf("SearchBlocksByContent: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d blocks, want 3", len(got))
	}

	// Results are ordered by page name then position, with page names
	// populated and stored content intact.
	wantUUIDs := []string{"gw-uuid-1", "gw-uuid-2", "other-uuid"}
	wantPages := []string{"compiled/gateway", "compiled/gateway", "notes/other"}
	for i, b := range got {
		if b.UUID != wantUUIDs[i] {
			t.Errorf("got[%d].UUID = %q, want %q", i, b.UUID, wantUUIDs[i])
		}
		if b.PageName != wantPages[i] {
			t.Errorf("got[%d].PageName = %q, want %q", i, b.PageName, wantPages[i])
		}
	}
	if got[0].Content != "# Gateway\n\nBody text with unique-phrase." {
		t.Errorf("got[0].Content = %q, want stored content", got[0].Content)
	}
}

func TestSearchBlocksByContent_CaseInsensitive(t *testing.T) {
	s := newTestStore(t)

	if err := s.InsertPage(testPage("ci-page")); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}
	if err := s.InsertBlock(&Block{
		UUID:     "ci-uuid-1",
		PageName: "ci-page",
		Content:  "Use Parameterized SQL to prevent injection",
		Position: 0,
	}); err != nil {
		t.Fatalf("InsertBlock(ci-uuid-1): %v", err)
	}
	if err := s.InsertBlock(&Block{
		UUID:     "ci-uuid-2",
		PageName: "ci-page",
		Content:  "sqlite like is case-insensitive for ascii",
		Position: 1,
	}); err != nil {
		t.Fatalf("InsertBlock(ci-uuid-2): %v", err)
	}

	// Lowercase query against mixed-case content.
	got, err := s.SearchBlocksByContent("parameterized", 10)
	if err != nil {
		t.Fatalf("SearchBlocksByContent(lowercase): %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("lowercase query: got %d blocks, want 1", len(got))
	}
	if got[0].UUID != "ci-uuid-1" {
		t.Errorf("lowercase query: UUID = %q, want %q", got[0].UUID, "ci-uuid-1")
	}

	// Uppercase query against mixed-case content.
	got, err = s.SearchBlocksByContent("PARAMETERIZED", 10)
	if err != nil {
		t.Fatalf("SearchBlocksByContent(uppercase): %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("uppercase query: got %d blocks, want 1", len(got))
	}
	if got[0].UUID != "ci-uuid-1" {
		t.Errorf("uppercase query: UUID = %q, want %q", got[0].UUID, "ci-uuid-1")
	}

	// Mixed-case query against lowercase content.
	got, err = s.SearchBlocksByContent("Case-Insensitive", 10)
	if err != nil {
		t.Fatalf("SearchBlocksByContent(mixed): %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("mixed-case query: got %d blocks, want 1", len(got))
	}
	if got[0].UUID != "ci-uuid-2" {
		t.Errorf("mixed-case query: UUID = %q, want %q", got[0].UUID, "ci-uuid-2")
	}
}

func TestSearchBlocksByContent_NoMatch(t *testing.T) {
	s := newTestStore(t)

	if err := s.InsertPage(testPage("nm-page")); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}
	if err := s.InsertBlock(&Block{
		UUID:     "nm-uuid",
		PageName: "nm-page",
		Content:  "existing content",
		Position: 0,
	}); err != nil {
		t.Fatalf("InsertBlock: %v", err)
	}

	got, err := s.SearchBlocksByContent("nonexistent-phrase", 10)
	if err != nil {
		t.Fatalf("SearchBlocksByContent: %v", err)
	}
	if got == nil {
		t.Error("got nil slice, want empty (non-nil) slice")
	}
	if len(got) != 0 {
		t.Errorf("got %d blocks, want 0", len(got))
	}
}

func TestSearchBlocksByContent_Limit(t *testing.T) {
	s := newTestStore(t)

	if err := s.InsertPage(testPage("limit-page")); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}
	for i := 0; i < 5; i++ {
		if err := s.InsertBlock(&Block{
			UUID:     fmt.Sprintf("limit-uuid-%d", i),
			PageName: "limit-page",
			Content:  fmt.Sprintf("needle in block %d", i),
			Position: i,
		}); err != nil {
			t.Fatalf("InsertBlock(%d): %v", i, err)
		}
	}

	// Limit below the match count truncates results in order.
	got, err := s.SearchBlocksByContent("needle", 3)
	if err != nil {
		t.Fatalf("SearchBlocksByContent(limit=3): %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("limit 3: got %d blocks, want 3", len(got))
	}
	for i, b := range got {
		want := fmt.Sprintf("limit-uuid-%d", i)
		if b.UUID != want {
			t.Errorf("limit 3: got[%d].UUID = %q, want %q", i, b.UUID, want)
		}
	}

	// Limit above the match count returns all matches.
	got, err = s.SearchBlocksByContent("needle", 100)
	if err != nil {
		t.Fatalf("SearchBlocksByContent(limit=100): %v", err)
	}
	if len(got) != 5 {
		t.Errorf("limit 100: got %d blocks, want 5", len(got))
	}

	// limit <= 0 means no cap (SearchSimilar convention).
	got, err = s.SearchBlocksByContent("needle", 0)
	if err != nil {
		t.Fatalf("SearchBlocksByContent(limit=0): %v", err)
	}
	if len(got) != 5 {
		t.Errorf("limit 0: got %d blocks, want 5", len(got))
	}
}

func TestSearchBlocksByContent_LikeWildcardsLiteral(t *testing.T) {
	s := newTestStore(t)

	if err := s.InsertPage(testPage("wild-page")); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	insert := []Block{
		{UUID: "wild-percent", PageName: "wild-page", Content: "completed 100% of the work", Position: 0},
		{UUID: "wild-underscore", PageName: "wild-page", Content: "task_a is done", Position: 1},
		{UUID: "wild-backslash", PageName: "wild-page", Content: `path C:\Users\jay`, Position: 2},
		// Would match the queries above if % and _ acted as LIKE
		// wildcards instead of literal text.
		{UUID: "wild-decoy", PageName: "wild-page", Content: "completed 100x of taskza", Position: 3},
	}
	for i := range insert {
		if err := s.InsertBlock(&insert[i]); err != nil {
			t.Fatalf("InsertBlock(%s): %v", insert[i].UUID, err)
		}
	}

	cases := []struct {
		name  string
		query string
		want  string // UUID of the single expected match.
	}{
		{"percent literal", "100%", "wild-percent"},
		{"underscore literal", "task_a", "wild-underscore"},
		{"backslash literal", `C:\Users`, "wild-backslash"},
	}
	for _, tc := range cases {
		got, err := s.SearchBlocksByContent(tc.query, 10)
		if err != nil {
			t.Fatalf("%s: SearchBlocksByContent: %v", tc.name, err)
		}
		if len(got) != 1 {
			t.Errorf("%s: got %d blocks, want 1", tc.name, len(got))
			continue
		}
		if got[0].UUID != tc.want {
			t.Errorf("%s: UUID = %q, want %q", tc.name, got[0].UUID, tc.want)
		}
	}
}

// --- Link Tests ---

func TestInsertLink_Success(t *testing.T) {
	s := newTestStore(t)

	if err := s.InsertPage(testPage("from-page")); err != nil {
		t.Fatalf("InsertPage(from): %v", err)
	}
	if err := s.InsertBlock(&Block{
		UUID:     "link-block-1",
		PageName: "from-page",
		Content:  "[[to-page]]",
	}); err != nil {
		t.Fatalf("InsertBlock: %v", err)
	}

	// Note: to_page intentionally has no FK — dangling links are valid.
	l := &Link{
		FromPage:  "from-page",
		ToPage:    "to-page",
		BlockUUID: "link-block-1",
	}
	if err := s.InsertLink(l); err != nil {
		t.Fatalf("InsertLink: %v", err)
	}

	// Verify forward links.
	fwd, err := s.GetForwardLinks("from-page")
	if err != nil {
		t.Fatalf("GetForwardLinks: %v", err)
	}
	if len(fwd) != 1 {
		t.Fatalf("got %d forward links, want 1", len(fwd))
	}
	if fwd[0].ToPage != "to-page" {
		t.Errorf("ToPage = %q, want %q", fwd[0].ToPage, "to-page")
	}
	if fwd[0].BlockUUID != "link-block-1" {
		t.Errorf("BlockUUID = %q, want %q", fwd[0].BlockUUID, "link-block-1")
	}

	// Verify backward links.
	bwd, err := s.GetBackwardLinks("to-page")
	if err != nil {
		t.Fatalf("GetBackwardLinks: %v", err)
	}
	if len(bwd) != 1 {
		t.Fatalf("got %d backward links, want 1", len(bwd))
	}
	if bwd[0].FromPage != "from-page" {
		t.Errorf("FromPage = %q, want %q", bwd[0].FromPage, "from-page")
	}
}

func TestInsertLink_Duplicate(t *testing.T) {
	s := newTestStore(t)

	if err := s.InsertPage(testPage("dup-from")); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}
	if err := s.InsertBlock(&Block{
		UUID:     "dup-link-block",
		PageName: "dup-from",
		Content:  "[[dup-to]]",
	}); err != nil {
		t.Fatalf("InsertBlock: %v", err)
	}

	l := &Link{FromPage: "dup-from", ToPage: "dup-to", BlockUUID: "dup-link-block"}
	if err := s.InsertLink(l); err != nil {
		t.Fatalf("first InsertLink: %v", err)
	}
	// INSERT OR IGNORE should silently skip duplicates.
	if err := s.InsertLink(l); err != nil {
		t.Fatalf("duplicate InsertLink should not error: %v", err)
	}
}

func TestDeleteLinksByPage(t *testing.T) {
	s := newTestStore(t)

	if err := s.InsertPage(testPage("del-link-page")); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}
	if err := s.InsertBlock(&Block{
		UUID:     "del-link-block",
		PageName: "del-link-page",
		Content:  "[[target]]",
	}); err != nil {
		t.Fatalf("InsertBlock: %v", err)
	}
	if err := s.InsertLink(&Link{
		FromPage:  "del-link-page",
		ToPage:    "target",
		BlockUUID: "del-link-block",
	}); err != nil {
		t.Fatalf("InsertLink: %v", err)
	}

	if err := s.DeleteLinksByPage("del-link-page"); err != nil {
		t.Fatalf("DeleteLinksByPage: %v", err)
	}

	links, err := s.GetForwardLinks("del-link-page")
	if err != nil {
		t.Fatalf("GetForwardLinks: %v", err)
	}
	if len(links) != 0 {
		t.Errorf("got %d links after delete, want 0", len(links))
	}
}

// --- Metadata Tests ---

func TestSetMeta_InsertAndUpdate(t *testing.T) {
	s := newTestStore(t)

	// Insert new key.
	if err := s.SetMeta("page_count", "42"); err != nil {
		t.Fatalf("SetMeta(insert): %v", err)
	}
	val, err := s.GetMeta("page_count")
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if val != "42" {
		t.Errorf("GetMeta = %q, want %q", val, "42")
	}

	// Update existing key.
	if err := s.SetMeta("page_count", "99"); err != nil {
		t.Fatalf("SetMeta(update): %v", err)
	}
	val, err = s.GetMeta("page_count")
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if val != "99" {
		t.Errorf("GetMeta = %q, want %q", val, "99")
	}
}

func TestGetMeta_NotFound(t *testing.T) {
	s := newTestStore(t)

	val, err := s.GetMeta("nonexistent")
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if val != "" {
		t.Errorf("GetMeta = %q, want empty string", val)
	}
}

// --- Content Hash Change Detection ---

func TestContentHash_ChangeDetection(t *testing.T) {
	s := newTestStore(t)

	p := testPage("hash-page")
	p.ContentHash = "hash-v1"
	if err := s.InsertPage(p); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	// Simulate checking if content changed.
	got, err := s.GetPage("hash-page")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if got.ContentHash != "hash-v1" {
		t.Errorf("initial ContentHash = %q, want %q", got.ContentHash, "hash-v1")
	}

	// Simulate content change.
	p.ContentHash = "hash-v2"
	if err := s.UpdatePage(p); err != nil {
		t.Fatalf("UpdatePage: %v", err)
	}

	got, err = s.GetPage("hash-page")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if got.ContentHash != "hash-v2" {
		t.Errorf("ContentHash = %q, want %q", got.ContentHash, "hash-v2")
	}
}

// --- Schema Migration Tests ---

func TestMigrate_FutureVersion(t *testing.T) {
	s := newTestStore(t)

	// Set a future schema version.
	if err := s.SetMeta("schema_version", "999"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}

	// Re-running migrate should fail for a future version.
	err := s.migrate()
	if err == nil {
		t.Fatal("migrate() should fail for future schema version")
	}
}

// --- Journal Page Tests ---

func TestInsertPage_JournalFlag(t *testing.T) {
	s := newTestStore(t)

	p := testPage("daily-note")
	p.IsJournal = true
	if err := s.InsertPage(p); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	got, err := s.GetPage("daily-note")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if !got.IsJournal {
		t.Error("IsJournal = false, want true")
	}
}

// --- Page with Null Optional Fields ---

// --- Corruption Detection Tests (T026A) ---

func TestMigrate_CorruptedSchemaVersion(t *testing.T) {
	s := newTestStore(t)

	// Corrupt the schema version with a non-numeric value.
	if err := s.SetMeta("schema_version", "not-a-number"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}

	err := s.migrate()
	if err == nil {
		t.Fatal("migrate() should fail with corrupted schema_version")
	}
}

func TestMigrate_MissingSchemaVersion(t *testing.T) {
	s := newTestStore(t)

	// Delete the schema_version key.
	_, err := s.db.Exec(`DELETE FROM metadata WHERE key = 'schema_version'`)
	if err != nil {
		t.Fatalf("delete schema_version: %v", err)
	}

	// Re-running migrate should re-create the schema (treat as fresh).
	if err := s.migrate(); err != nil {
		t.Fatalf("migrate() should succeed with missing schema_version: %v", err)
	}

	// Verify schema_version is restored.
	version, err := s.GetMeta("schema_version")
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	want := fmt.Sprintf("%d", schemaVersion)
	if version != want {
		t.Errorf("schema_version = %q, want %q", version, want)
	}
}

func TestMigrate_IncompatibleFutureVersion(t *testing.T) {
	s := newTestStore(t)

	// Set a future version that the current code doesn't support.
	if err := s.SetMeta("schema_version", "999"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}

	err := s.migrate()
	if err == nil {
		t.Fatal("migrate() should fail for incompatible future version")
	}
	if !strings.Contains(err.Error(), "newer than supported") {
		t.Errorf("error = %q, want to contain 'newer than supported'", err.Error())
	}
}

func TestNew_CorruptedDatabase_Recovery(t *testing.T) {
	// Create a store, corrupt it, then verify a new store can be opened
	// (simulating the recovery path where the caller discards and re-creates).
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create a valid store.
	s1, err := New(dbPath)
	if err != nil {
		t.Fatalf("first New: %v", err)
	}
	if err := s1.InsertPage(testPage("test-page")); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}
	_ = s1.Close()

	// Corrupt the database by writing garbage.
	if err := os.WriteFile(dbPath, []byte("this is not a database"), 0o644); err != nil {
		t.Fatalf("corrupt database: %v", err)
	}

	// Opening the corrupted database should fail.
	_, err = New(dbPath)
	if err == nil {
		t.Fatal("New should fail with corrupted database")
	}

	// Recovery: remove the corrupted file and create a fresh one.
	_ = os.Remove(dbPath)
	s2, err := New(dbPath)
	if err != nil {
		t.Fatalf("New after recovery: %v", err)
	}
	defer func() { _ = s2.Close() }()

	// Verify the fresh store works.
	if err := s2.InsertPage(testPage("recovered-page")); err != nil {
		t.Fatalf("InsertPage after recovery: %v", err)
	}
	got, err := s2.GetPage("recovered-page")
	if err != nil || got == nil {
		t.Fatal("recovered page not found")
	}
}

// --- Disk Space Exhaustion Tests (T026B) ---

func TestStore_WriteFailure_Graceful(t *testing.T) {
	// Test that the store handles write failures gracefully.
	// We simulate this by closing the database and attempting writes.
	s := newTestStore(t)

	// Insert a page successfully first.
	if err := s.InsertPage(testPage("before-close")); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	// Close the database to simulate write failure.
	_ = s.db.Close()

	// Writes should fail with an error (not panic).
	err := s.InsertPage(testPage("after-close"))
	if err == nil {
		t.Fatal("InsertPage should fail after database close")
	}

	// SetMeta should also fail gracefully.
	err = s.SetMeta("key", "value")
	if err == nil {
		t.Fatal("SetMeta should fail after database close")
	}
}

// --- Concurrent Access Tests (T026C) ---

func TestStore_ConcurrentReads(t *testing.T) {
	s := newTestStore(t)

	// Insert test data.
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("concurrent-page-%d", i)
		if err := s.InsertPage(testPage(name)); err != nil {
			t.Fatalf("InsertPage(%s): %v", name, err)
		}
	}

	// Spawn multiple concurrent readers.
	done := make(chan bool)
	const numReaders = 5

	for i := 0; i < numReaders; i++ {
		go func(id int) {
			for j := 0; j < 20; j++ {
				_, err := s.ListPages()
				if err != nil {
					t.Errorf("reader %d: ListPages: %v", id, err)
				}
				name := fmt.Sprintf("concurrent-page-%d", j%10)
				_, err = s.GetPage(name)
				if err != nil {
					t.Errorf("reader %d: GetPage(%s): %v", id, name, err)
				}
			}
			done <- true
		}(i)
	}

	for i := 0; i < numReaders; i++ {
		<-done
	}
}

func TestStore_ConcurrentReadWrite(t *testing.T) {
	s := newTestStore(t)

	// Insert initial data.
	if err := s.InsertPage(testPage("shared-page")); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	done := make(chan bool)

	// Reader goroutine.
	go func() {
		for i := 0; i < 50; i++ {
			_, _ = s.GetPage("shared-page")
			_, _ = s.ListPages()
		}
		done <- true
	}()

	// Writer goroutine — update metadata.
	go func() {
		for i := 0; i < 50; i++ {
			_ = s.SetMeta("counter", fmt.Sprintf("%d", i))
		}
		done <- true
	}()

	<-done
	<-done

	// Verify no corruption.
	page, err := s.GetPage("shared-page")
	if err != nil {
		t.Fatalf("GetPage after concurrent access: %v", err)
	}
	if page == nil {
		t.Fatal("page should still exist after concurrent access")
	}
}

func TestInsertPage_NullOptionalFields(t *testing.T) {
	s := newTestStore(t)

	p := &Page{
		Name:         "minimal-page",
		OriginalName: "minimal-page",
		SourceID:     "disk-local",
		CreatedAt:    1000,
		UpdatedAt:    1000,
	}
	if err := s.InsertPage(p); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	got, err := s.GetPage("minimal-page")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if got == nil {
		t.Fatal("GetPage returned nil")
	}
	if got.SourceDocID != "" {
		t.Errorf("SourceDocID = %q, want empty", got.SourceDocID)
	}
	if got.Properties != "" {
		t.Errorf("Properties = %q, want empty", got.Properties)
	}
	if got.ContentHash != "" {
		t.Errorf("ContentHash = %q, want empty", got.ContentHash)
	}
}

func TestFindSubstring_CaseInsensitive(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		substr string
		want   bool
	}{
		{name: "exact_match_lowercase", s: "hello", substr: "hello", want: true},
		{name: "exact_match_uppercase", s: "HELLO", substr: "HELLO", want: true},
		{name: "mixed_case_match", s: "Hello World", substr: "hello world", want: true},
		{name: "substr_uppercase_s_lowercase", s: "disk is full", substr: "DISK IS FULL", want: true},
		{name: "substr_lowercase_s_uppercase", s: "SQLITE_FULL", substr: "sqlite_full", want: true},
		{name: "partial_match_at_start", s: "database or disk is full", substr: "disk is full", want: true},
		{name: "partial_match_in_middle", s: "error: no space left on device", substr: "no space left", want: true},
		{name: "no_match", s: "hello world", substr: "goodbye", want: false},
		{name: "substr_longer_than_s", s: "hi", substr: "hello", want: false},
		{name: "single_char_match", s: "abc", substr: "B", want: true},
		{name: "single_char_no_match", s: "abc", substr: "z", want: false},
		{name: "empty_s_nonempty_substr", s: "", substr: "a", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := findSubstring(tc.s, tc.substr)
			if got != tc.want {
				t.Errorf("findSubstring(%q, %q) = %v, want %v", tc.s, tc.substr, got, tc.want)
			}
		})
	}
}

func TestContains_EdgeCases(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		substr string
		want   bool
	}{
		{name: "empty_substr_always_matches", s: "anything", substr: "", want: true},
		{name: "empty_both", s: "", substr: "", want: true},
		{name: "equal_strings", s: "hello", substr: "hello", want: true},
		{name: "case_insensitive_equal", s: "Hello", substr: "hello", want: true},
		{name: "s_shorter_than_substr", s: "hi", substr: "hello", want: false},
		{name: "substr_at_end", s: "SQLITE_FULL", substr: "full", want: true},
		{name: "no_match", s: "abc", substr: "xyz", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := contains(tc.s, tc.substr)
			if got != tc.want {
				t.Errorf("contains(%q, %q) = %v, want %v", tc.s, tc.substr, got, tc.want)
			}
		})
	}
}

func TestIsDiskSpaceError_Recognition(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil_error", err: nil, want: false},
		{name: "disk_full_lowercase", err: fmt.Errorf("database or disk is full"), want: true},
		{name: "disk_full_mixed_case", err: fmt.Errorf("Disk Is Full"), want: true},
		{name: "no_space_left", err: fmt.Errorf("no space left on device"), want: true},
		{name: "sqlite_full_uppercase", err: fmt.Errorf("SQLITE_FULL (18)"), want: true},
		{name: "sqlite_full_lowercase", err: fmt.Errorf("sqlite_full"), want: true},
		{name: "unrelated_error", err: fmt.Errorf("connection refused"), want: false},
		{name: "empty_error_message", err: fmt.Errorf(""), want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := IsDiskSpaceError(tc.err)
			if got != tc.want {
				t.Errorf("IsDiskSpaceError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// --- Source CRUD Tests ---

// testSource returns a SourceRecord with sensible defaults for testing.
func testSource(id, name string) *SourceRecord {
	return &SourceRecord{
		ID:              id,
		Type:            "disk",
		Name:            name,
		Config:          `{"path": "."}`,
		RefreshInterval: "daily",
		Status:          "active",
	}
}

func TestListSources_Empty(t *testing.T) {
	s := newTestStore(t)

	sources, err := s.ListSources()
	if err != nil {
		t.Fatalf("ListSources: %v", err)
	}
	if len(sources) != 0 {
		t.Errorf("ListSources returned %d sources, want 0", len(sources))
	}
}

func TestListSources_Multiple(t *testing.T) {
	s := newTestStore(t)

	// Insert sources in non-alphabetical order to verify ORDER BY id.
	for _, id := range []string{"github-test", "disk-local", "web-docs"} {
		src := testSource(id, id+"-name")
		src.Type = strings.SplitN(id, "-", 2)[0] // "github", "disk", "web"
		if err := s.InsertSource(src); err != nil {
			t.Fatalf("InsertSource(%s): %v", id, err)
		}
	}

	sources, err := s.ListSources()
	if err != nil {
		t.Fatalf("ListSources: %v", err)
	}
	if len(sources) != 3 {
		t.Fatalf("ListSources returned %d sources, want 3", len(sources))
	}

	// Verify ORDER BY id (alphabetical).
	if sources[0].ID != "disk-local" {
		t.Errorf("sources[0].ID = %q, want %q", sources[0].ID, "disk-local")
	}
	if sources[1].ID != "github-test" {
		t.Errorf("sources[1].ID = %q, want %q", sources[1].ID, "github-test")
	}
	if sources[2].ID != "web-docs" {
		t.Errorf("sources[2].ID = %q, want %q", sources[2].ID, "web-docs")
	}
}

func TestListSources_FieldValues(t *testing.T) {
	s := newTestStore(t)

	src := &SourceRecord{
		ID:              "disk-local",
		Type:            "disk",
		Name:            "local vault",
		Config:          `{"path": "/vault"}`,
		RefreshInterval: "hourly",
		LastFetchedAt:   1700000000,
		Status:          "active",
		ErrorMessage:    "",
	}
	if err := s.InsertSource(src); err != nil {
		t.Fatalf("InsertSource: %v", err)
	}

	sources, err := s.ListSources()
	if err != nil {
		t.Fatalf("ListSources: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("ListSources returned %d sources, want 1", len(sources))
	}

	got := sources[0]
	if got.ID != "disk-local" {
		t.Errorf("ID = %q, want %q", got.ID, "disk-local")
	}
	if got.Type != "disk" {
		t.Errorf("Type = %q, want %q", got.Type, "disk")
	}
	if got.Name != "local vault" {
		t.Errorf("Name = %q, want %q", got.Name, "local vault")
	}
	if got.Config != `{"path": "/vault"}` {
		t.Errorf("Config = %q, want %q", got.Config, `{"path": "/vault"}`)
	}
	if got.RefreshInterval != "hourly" {
		t.Errorf("RefreshInterval = %q, want %q", got.RefreshInterval, "hourly")
	}
	if got.LastFetchedAt != 1700000000 {
		t.Errorf("LastFetchedAt = %d, want %d", got.LastFetchedAt, 1700000000)
	}
	if got.Status != "active" {
		t.Errorf("Status = %q, want %q", got.Status, "active")
	}
	if got.ErrorMessage != "" {
		t.Errorf("ErrorMessage = %q, want empty", got.ErrorMessage)
	}
}

func TestUpdateSourceStatus_ExistingSource(t *testing.T) {
	s := newTestStore(t)

	src := testSource("disk-local", "local")
	if err := s.InsertSource(src); err != nil {
		t.Fatalf("InsertSource: %v", err)
	}

	// Update status to error with a message.
	if err := s.UpdateSourceStatus("disk-local", "error", "connection timeout"); err != nil {
		t.Fatalf("UpdateSourceStatus: %v", err)
	}

	// Verify the status and error message changed.
	got, err := s.GetSource("disk-local")
	if err != nil {
		t.Fatalf("GetSource: %v", err)
	}
	if got == nil {
		t.Fatal("GetSource returned nil after update")
	}
	if got.Status != "error" {
		t.Errorf("Status = %q, want %q", got.Status, "error")
	}
	if got.ErrorMessage != "connection timeout" {
		t.Errorf("ErrorMessage = %q, want %q", got.ErrorMessage, "connection timeout")
	}
}

func TestUpdateSourceStatus_ClearError(t *testing.T) {
	s := newTestStore(t)

	src := testSource("disk-local", "local")
	src.Status = "error"
	src.ErrorMessage = "previous failure"
	if err := s.InsertSource(src); err != nil {
		t.Fatalf("InsertSource: %v", err)
	}

	// Clear the error by updating to active with empty error message.
	if err := s.UpdateSourceStatus("disk-local", "active", ""); err != nil {
		t.Fatalf("UpdateSourceStatus: %v", err)
	}

	got, err := s.GetSource("disk-local")
	if err != nil {
		t.Fatalf("GetSource: %v", err)
	}
	if got.Status != "active" {
		t.Errorf("Status = %q, want %q", got.Status, "active")
	}
	if got.ErrorMessage != "" {
		t.Errorf("ErrorMessage = %q, want empty", got.ErrorMessage)
	}
}

func TestUpdateSourceStatus_NonExistent(t *testing.T) {
	s := newTestStore(t)

	err := s.UpdateSourceStatus("nonexistent-source", "active", "")
	if err == nil {
		t.Fatal("UpdateSourceStatus should return error for non-existent source")
	}
	if !strings.Contains(err.Error(), "source not found") {
		t.Errorf("error = %q, want to contain 'source not found'", err.Error())
	}
}

func TestUpdateLastFetched_ExistingSource(t *testing.T) {
	s := newTestStore(t)

	src := testSource("disk-local", "local")
	src.LastFetchedAt = 1000
	if err := s.InsertSource(src); err != nil {
		t.Fatalf("InsertSource: %v", err)
	}

	// Update last fetched to a new timestamp.
	newTimestamp := int64(1700000000)
	if err := s.UpdateLastFetched("disk-local", newTimestamp); err != nil {
		t.Fatalf("UpdateLastFetched: %v", err)
	}

	got, err := s.GetSource("disk-local")
	if err != nil {
		t.Fatalf("GetSource: %v", err)
	}
	if got == nil {
		t.Fatal("GetSource returned nil after update")
	}
	if got.LastFetchedAt != newTimestamp {
		t.Errorf("LastFetchedAt = %d, want %d", got.LastFetchedAt, newTimestamp)
	}
}

func TestUpdateLastFetched_ZeroTimestamp(t *testing.T) {
	s := newTestStore(t)

	src := testSource("disk-local", "local")
	src.LastFetchedAt = 1700000000
	if err := s.InsertSource(src); err != nil {
		t.Fatalf("InsertSource: %v", err)
	}

	// Setting to zero should succeed (reset to "never fetched").
	if err := s.UpdateLastFetched("disk-local", 0); err != nil {
		t.Fatalf("UpdateLastFetched(0): %v", err)
	}

	got, err := s.GetSource("disk-local")
	if err != nil {
		t.Fatalf("GetSource: %v", err)
	}
	if got.LastFetchedAt != 0 {
		t.Errorf("LastFetchedAt = %d, want 0", got.LastFetchedAt)
	}
}

func TestUpdateLastFetched_NonExistent(t *testing.T) {
	s := newTestStore(t)

	err := s.UpdateLastFetched("nonexistent-source", 1700000000)
	if err == nil {
		t.Fatal("UpdateLastFetched should return error for non-existent source")
	}
	if !strings.Contains(err.Error(), "source not found") {
		t.Errorf("error = %q, want to contain 'source not found'", err.Error())
	}
}

func TestUpdateLastFetched_DoesNotAffectOtherFields(t *testing.T) {
	s := newTestStore(t)

	src := &SourceRecord{
		ID:              "disk-local",
		Type:            "disk",
		Name:            "local",
		Config:          `{"path": "."}`,
		RefreshInterval: "daily",
		LastFetchedAt:   1000,
		Status:          "active",
		ErrorMessage:    "some error",
	}
	if err := s.InsertSource(src); err != nil {
		t.Fatalf("InsertSource: %v", err)
	}

	if err := s.UpdateLastFetched("disk-local", 2000); err != nil {
		t.Fatalf("UpdateLastFetched: %v", err)
	}

	got, err := s.GetSource("disk-local")
	if err != nil {
		t.Fatalf("GetSource: %v", err)
	}

	// Verify only LastFetchedAt changed.
	if got.LastFetchedAt != 2000 {
		t.Errorf("LastFetchedAt = %d, want 2000", got.LastFetchedAt)
	}
	if got.Status != "active" {
		t.Errorf("Status = %q, want %q (should be unchanged)", got.Status, "active")
	}
	if got.ErrorMessage != "some error" {
		t.Errorf("ErrorMessage = %q, want %q (should be unchanged)", got.ErrorMessage, "some error")
	}
	if got.Name != "local" {
		t.Errorf("Name = %q, want %q (should be unchanged)", got.Name, "local")
	}
}

// --- Source-level page operations (004-unified-content-serve) ---

func TestListPagesExcludingSource(t *testing.T) {
	s := newTestStore(t)

	// Insert pages from different sources.
	localPage := testPage("local-page")
	localPage.SourceID = "disk-local"
	if err := s.InsertPage(localPage); err != nil {
		t.Fatalf("InsertPage(local): %v", err)
	}

	extPage1 := testPage("github-org/issue-1")
	extPage1.SourceID = "github-org"
	if err := s.InsertPage(extPage1); err != nil {
		t.Fatalf("InsertPage(ext1): %v", err)
	}

	extPage2 := testPage("web-docs/api-ref")
	extPage2.SourceID = "web-docs"
	if err := s.InsertPage(extPage2); err != nil {
		t.Fatalf("InsertPage(ext2): %v", err)
	}

	// Exclude disk-local → should return the two external pages.
	pages, err := s.ListPagesExcludingSource("disk-local")
	if err != nil {
		t.Fatalf("ListPagesExcludingSource: %v", err)
	}
	if len(pages) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(pages))
	}

	// Verify ordering (alphabetical by name).
	if pages[0].Name != "github-org/issue-1" {
		t.Errorf("pages[0].Name = %q, want %q", pages[0].Name, "github-org/issue-1")
	}
	if pages[1].Name != "web-docs/api-ref" {
		t.Errorf("pages[1].Name = %q, want %q", pages[1].Name, "web-docs/api-ref")
	}
}

func TestListPagesExcludingSource_NoMatches(t *testing.T) {
	s := newTestStore(t)

	localPage := testPage("only-local")
	localPage.SourceID = "disk-local"
	if err := s.InsertPage(localPage); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	pages, err := s.ListPagesExcludingSource("disk-local")
	if err != nil {
		t.Fatalf("ListPagesExcludingSource: %v", err)
	}
	if len(pages) != 0 {
		t.Errorf("expected 0 pages, got %d", len(pages))
	}
}

func TestDeletePagesBySource(t *testing.T) {
	s := newTestStore(t)

	// Insert pages from two sources.
	for i := 0; i < 3; i++ {
		p := testPage(fmt.Sprintf("github-org/issue-%d", i))
		p.SourceID = "github-org"
		p.SourceDocID = fmt.Sprintf("issue-%d", i)
		if err := s.InsertPage(p); err != nil {
			t.Fatalf("InsertPage(github %d): %v", i, err)
		}
	}

	localPage := testPage("local-page")
	localPage.SourceID = "disk-local"
	if err := s.InsertPage(localPage); err != nil {
		t.Fatalf("InsertPage(local): %v", err)
	}

	// Add a block to one of the github pages to verify CASCADE.
	block := &Block{
		UUID:     "block-uuid-1",
		PageName: "github-org/issue-0",
		Content:  "test block content",
	}
	if err := s.InsertBlock(block); err != nil {
		t.Fatalf("InsertBlock: %v", err)
	}

	// Delete all github pages.
	deleted, err := s.DeletePagesBySource("github-org")
	if err != nil {
		t.Fatalf("DeletePagesBySource: %v", err)
	}
	if deleted != 3 {
		t.Errorf("deleted = %d, want 3", deleted)
	}

	// Verify local page still exists.
	remaining, err := s.ListPages()
	if err != nil {
		t.Fatalf("ListPages: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("expected 1 remaining page, got %d", len(remaining))
	}
	if remaining[0].Name != "local-page" {
		t.Errorf("remaining page = %q, want %q", remaining[0].Name, "local-page")
	}

	// Verify CASCADE deleted the block.
	b, err := s.GetBlock("block-uuid-1")
	if err != nil {
		t.Fatalf("GetBlock: %v", err)
	}
	if b != nil {
		t.Error("expected block to be CASCADE deleted, but it still exists")
	}
}

func TestDeletePagesBySource_NoMatches(t *testing.T) {
	s := newTestStore(t)

	deleted, err := s.DeletePagesBySource("nonexistent-source")
	if err != nil {
		t.Fatalf("DeletePagesBySource: %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0", deleted)
	}
}

func TestListPagesBySource(t *testing.T) {
	s := newTestStore(t)

	// Insert pages from different sources.
	for i := 0; i < 2; i++ {
		p := testPage(fmt.Sprintf("github-org/issue-%d", i))
		p.SourceID = "github-org"
		p.SourceDocID = fmt.Sprintf("issue-%d", i)
		if err := s.InsertPage(p); err != nil {
			t.Fatalf("InsertPage(github %d): %v", i, err)
		}
	}

	localPage := testPage("local-page")
	localPage.SourceID = "disk-local"
	if err := s.InsertPage(localPage); err != nil {
		t.Fatalf("InsertPage(local): %v", err)
	}

	// List only github pages.
	pages, err := s.ListPagesBySource("github-org")
	if err != nil {
		t.Fatalf("ListPagesBySource: %v", err)
	}
	if len(pages) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(pages))
	}
	for _, p := range pages {
		if p.SourceID != "github-org" {
			t.Errorf("page %q has sourceID %q, want %q", p.Name, p.SourceID, "github-org")
		}
	}
}

func TestListPagesBySource_NoMatches(t *testing.T) {
	s := newTestStore(t)

	pages, err := s.ListPagesBySource("nonexistent")
	if err != nil {
		t.Fatalf("ListPagesBySource: %v", err)
	}
	if len(pages) != 0 {
		t.Errorf("expected 0 pages, got %d", len(pages))
	}
}

// --- GetPagesWithProperty tests (FR-SAN-009, FR-SAN-010) ---

func TestGetPagesWithProperty_Exists(t *testing.T) {
	s := newTestStore(t)

	// Insert a page with sanitize_findings in its properties JSON.
	p := testPage("web-docs/api-auth")
	p.Properties = `{"sanitize_findings": [{"pattern": "ignore previous", "severity": "critical"}]}`
	if err := s.InsertPage(p); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	// Insert another page with the same property to verify multiple results.
	p2 := testPage("web-docs/api-users")
	p2.SourceDocID = "api-users.md"
	p2.Properties = `{"sanitize_findings": [{"pattern": "you are now", "severity": "high"}], "title": "Users API"}`
	if err := s.InsertPage(p2); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	pages, err := s.GetPagesWithProperty("sanitize_findings")
	if err != nil {
		t.Fatalf("GetPagesWithProperty: %v", err)
	}
	if len(pages) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(pages))
	}

	// Verify the returned pages are the ones we inserted.
	names := make(map[string]bool)
	for _, pg := range pages {
		names[pg.Name] = true
	}
	if !names["web-docs/api-auth"] {
		t.Error("expected page web-docs/api-auth in results")
	}
	if !names["web-docs/api-users"] {
		t.Error("expected page web-docs/api-users in results")
	}
}

func TestGetPagesWithProperty_NotExists(t *testing.T) {
	s := newTestStore(t)

	// Insert a page with a different property — no sanitize_findings.
	p := testPage("clean-page")
	p.Properties = `{"title": "foo", "tags": ["docs"]}`
	if err := s.InsertPage(p); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	pages, err := s.GetPagesWithProperty("sanitize_findings")
	if err != nil {
		t.Fatalf("GetPagesWithProperty: %v", err)
	}
	if len(pages) != 0 {
		t.Errorf("expected 0 pages, got %d", len(pages))
	}
}

func TestGetPagesWithProperty_EmptyIndex(t *testing.T) {
	s := newTestStore(t)

	// Query on an empty store — should return empty slice, no error.
	pages, err := s.GetPagesWithProperty("sanitize_findings")
	if err != nil {
		t.Fatalf("GetPagesWithProperty: %v", err)
	}
	if pages == nil {
		t.Fatal("expected non-nil empty slice, got nil")
	}
	if len(pages) != 0 {
		t.Errorf("expected 0 pages, got %d", len(pages))
	}
}

// --- Promote tier operations ---

func insertResolvePage(t *testing.T, s *Store, name, sourceID, sourceDocID, tier string) {
	t.Helper()
	if err := s.InsertPage(&Page{
		Name:         name,
		OriginalName: name,
		SourceID:     sourceID,
		SourceDocID:  sourceDocID,
		Tier:         tier,
	}); err != nil {
		t.Fatalf("InsertPage(%q): %v", name, err)
	}
}

func TestPromotePagesTiers_AtomicRollback(t *testing.T) {
	s := newTestStore(t)
	insertResolvePage(t, s, "compiled/a", "compiled", "a", "draft")
	insertResolvePage(t, s, "compiled/b", "compiled", "b", "validated")

	err := s.PromotePagesTiers([]string{"compiled/a", "compiled/b"})
	if err == nil {
		t.Fatal("expected transaction failure for a non-draft page")
	}

	a, _ := s.GetPage("compiled/a")
	if a.Tier != "draft" {
		t.Errorf("compiled/a tier = %q, want draft (rollback)", a.Tier)
	}
}

// --- Index usage verification (dewey-distill items 10+14) ---

// explainQueryPlan runs EXPLAIN QUERY PLAN for the given statement and
// returns the concatenated plan detail rows. These tests pin the
// resolution queries to index seeks: a regression that reintroduces a
// full-table SCAN (e.g. a per-row LIKE) fails loudly here instead of
// only showing up as latency in production vaults.
func explainQueryPlan(t *testing.T, s *Store, query string, args ...any) string {
	t.Helper()
	rows, err := s.db.Query("EXPLAIN QUERY PLAN "+query, args...)
	if err != nil {
		t.Fatalf("EXPLAIN QUERY PLAN %q: %v", query, err)
	}
	defer func() { _ = rows.Close() }()
	var plan strings.Builder
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatalf("scan plan row: %v", err)
		}
		if plan.Len() > 0 {
			plan.WriteString("; ")
		}
		plan.WriteString(detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("plan rows: %v", err)
	}
	return plan.String()
}

// assertIndexSearch verifies a query plan performs an index SEARCH on the
// named index rather than a full-table SCAN.
func assertIndexSearch(t *testing.T, plan, index string) {
	t.Helper()
	if !strings.Contains(plan, "SEARCH") {
		t.Errorf("query plan = %q, want SEARCH via %s", plan, index)
	}
	if !strings.Contains(plan, index) {
		t.Errorf("query plan = %q, want index %s", plan, index)
	}
	if strings.Contains(plan, "SCAN") {
		t.Errorf("query plan = %q, full table SCAN not expected", plan)
	}
}

func TestListPagesByNameFoldCase_UsesIndex(t *testing.T) {
	s := newTestStore(t)
	insertResolvePage(t, s, "compiled/gateway", "compiled", "gateway", "draft")

	plan := explainQueryPlan(t, s, listPagesByNameFoldCaseSQL, "compiled/gateway")
	assertIndexSearch(t, plan, "idx_pages_name_nocase")

	pages, err := s.ListPagesByNameFoldCase("COMPILED/GATEWAY")
	if err != nil {
		t.Fatalf("ListPagesByNameFoldCase: %v", err)
	}
	if len(pages) != 1 || pages[0].Name != "compiled/gateway" {
		t.Errorf("pages = %+v, want compiled/gateway", pages)
	}
}

func TestListKnowledgePagesFoldCase_UsesIndex(t *testing.T) {
	s := newTestStore(t)
	insertResolvePage(t, s, "knowledge/team/authentication", "knowledge-team", "authentication", "curated")
	insertResolvePage(t, s, "compiled/elsewhere", "compiled", "elsewhere", "draft")

	// The range predicate must seek idx_pages_name_nocase instead of
	// evaluating LIKE per row.
	plan := explainQueryPlan(t, s, listKnowledgePagesFoldCaseSQL)
	assertIndexSearch(t, plan, "idx_pages_name_nocase")

	pages, err := s.ListKnowledgePagesFoldCase()
	if err != nil {
		t.Fatalf("ListKnowledgePagesFoldCase: %v", err)
	}
	if len(pages) != 1 || pages[0].Name != "knowledge/team/authentication" {
		t.Errorf("pages = %+v, want knowledge/team/authentication only", pages)
	}
}

func TestGetPageBySourceDoc_UsesIndex(t *testing.T) {
	s := newTestStore(t)
	insertResolvePage(t, s, "Article Gateway", "compiled", "gateway", "draft")

	plan := explainQueryPlan(t, s, getPageBySourceDocSQL, "compiled", "gateway")
	assertIndexSearch(t, plan, "idx_pages_source_doc_nocase")

	// Mixed-case arguments must resolve through the NOCASE index.
	pages, err := s.GetPageBySourceDoc("COMPILED", "Gateway")
	if err != nil {
		t.Fatalf("GetPageBySourceDoc: %v", err)
	}
	if len(pages) != 1 || pages[0].Name != "Article Gateway" {
		t.Errorf("pages = %+v, want Article Gateway", pages)
	}
}

func TestListDraftCandidatePages_UsesIndex(t *testing.T) {
	s := newTestStore(t)
	insertResolvePage(t, s, "learning/alpha-20260101T000000-x", "learning", "alpha", "draft")
	insertResolvePage(t, s, "compiled/beta", "compiled", "beta", "draft")
	insertResolvePage(t, s, "knowledge/team/gamma", "knowledge-team", "gamma", "draft")
	// Admitted case-insensitively, like the NOCASE range predicate.
	insertResolvePage(t, s, "Learning/Caps", "learning", "caps", "draft")
	// Excluded: not draft, and outside the three candidate namespaces.
	insertResolvePage(t, s, "compiled/validated", "compiled", "validated", "validated")
	insertResolvePage(t, s, "notes/elsewhere", "notes", "elsewhere", "draft")

	// The tier predicate must drive an index SEARCH, not a full-table scan.
	plan := explainQueryPlan(t, s, listDraftCandidatePagesSQL)
	assertIndexSearch(t, plan, "idx_pages_tier")

	pages, err := s.ListDraftCandidatePages()
	if err != nil {
		t.Fatalf("ListDraftCandidatePages: %v", err)
	}
	var names []string
	for _, p := range pages {
		names = append(names, p.Name)
	}
	// ORDER BY name is the BINARY collation of the primary key.
	want := []string{
		"Learning/Caps",
		"compiled/beta",
		"knowledge/team/gamma",
		"learning/alpha-20260101T000000-x",
	}
	got := "[" + strings.Join(names, " ") + "]"
	wantJoined := "[" + strings.Join(want, " ") + "]"
	if got != wantJoined {
		t.Errorf("ListDraftCandidatePages names = %s, want %s", got, wantJoined)
	}
}

// TestDeletePagesBySourceExcludingTier verifies the compiled-page cleanup
// helper (dewey-distill): it deletes only pages from the given source whose
// tier differs from the excluded tier, returns the deleted page names so
// callers can remove their files, and leaves validated pages and pages from
// other sources untouched.
func TestDeletePagesBySourceExcludingTier(t *testing.T) {
	s := newTestStore(t)
	insertResolvePage(t, s, "compiled/a", "compiled", "a", "draft")
	insertResolvePage(t, s, "compiled/b", "compiled", "b", "validated")
	insertResolvePage(t, s, "learning/c", "learning", "c", "draft")

	deleted, err := s.DeletePagesBySourceExcludingTier("compiled", "validated")
	if err != nil {
		t.Fatalf("DeletePagesBySourceExcludingTier: %v", err)
	}
	if len(deleted) != 1 || deleted[0] != "compiled/a" {
		t.Errorf("deleted = %v, want [compiled/a]", deleted)
	}
	if p, _ := s.GetPage("compiled/a"); p != nil {
		t.Errorf("compiled/a = %+v, want deleted", p)
	}
	b, _ := s.GetPage("compiled/b")
	if b == nil || b.Tier != "validated" {
		t.Errorf("compiled/b = %+v, want retained validated page", b)
	}
	if p, _ := s.GetPage("learning/c"); p == nil {
		t.Error("learning/c was deleted; tier exclusion must be scoped to the source")
	}
}

// TestUpdatePage_PreservesValidatedTier verifies the UpdatePage tier guard:
// a write carrying a draft tier (for example a racing compile that rebuilt
// the article) must not demote a human-validated page — the SQL CASE clause
// keeps the validated value.
func TestUpdatePage_PreservesValidatedTier(t *testing.T) {
	s := newTestStore(t)
	insertResolvePage(t, s, "compiled/gateway", "compiled", "gateway", "validated")

	page, err := s.GetPage("compiled/gateway")
	if err != nil || page == nil {
		t.Fatalf("GetPage: %v (page=%v)", err, page)
	}
	page.Tier = "draft"
	if err := s.UpdatePage(page); err != nil {
		t.Fatalf("UpdatePage: %v", err)
	}
	after, _ := s.GetPage("compiled/gateway")
	if after.Tier != "validated" {
		t.Errorf("tier = %q, want validated preserved against a draft-tier write", after.Tier)
	}

	// The guard must not block normal draft-to-draft updates.
	insertResolvePage(t, s, "compiled/normal", "compiled", "normal", "draft")
	np, _ := s.GetPage("compiled/normal")
	if np == nil {
		t.Fatal("compiled/normal missing")
	}
	np.Tier = "draft"
	if err := s.UpdatePage(np); err != nil {
		t.Fatalf("UpdatePage(normal): %v", err)
	}
	na, _ := s.GetPage("compiled/normal")
	if na.Tier != "draft" {
		t.Errorf("normal tier = %q, want draft", na.Tier)
	}
}

// --- Content-bound validated preservation (VTB-FR-001) ---

// newCompiledPage builds the Page shape a compiled-article write path
// hands to the store: name compiled/<tag>, source compiled, and explicit
// content-hash and tier fields. The compiled paths construct a fresh page
// per regeneration rather than read-modify-writing the stored row, so the
// content-bound preservation tests use this constructor for both the
// seeded row and the write under test — the two differ only in the fields
// each case varies.
func newCompiledPage(name, contentHash, tier string) *Page {
	return &Page{
		Name:         name,
		OriginalName: name,
		SourceID:     SourceCompiled,
		SourceDocID:  strings.TrimPrefix(name, CompiledPrefix),
		ContentHash:  contentHash,
		Tier:         tier,
	}
}

// insertCompiledPage seeds a store row built by newCompiledPage, failing
// the test on insert errors.
func insertCompiledPage(t *testing.T, s *Store, name, contentHash, tier string) {
	t.Helper()
	if err := s.InsertPage(newCompiledPage(name, contentHash, tier)); err != nil {
		t.Fatalf("InsertPage(%q): %v", name, err)
	}
}

// TestUpdatePage_ChangedContentDemotesValidated verifies the demoting half
// of the content-bound rule (VTB-FR-001): when the incoming content hash
// differs from the stored hash, the caller-supplied tier applies, so a
// regeneration that changed the article bytes demotes a human-validated
// page — the machine never carries the human vouch onto content it has
// not seen.
func TestUpdatePage_ChangedContentDemotesValidated(t *testing.T) {
	s := newTestStore(t)
	insertCompiledPage(t, s, "compiled/auth", "hash-v1", "validated")

	regenerated := newCompiledPage("compiled/auth", "hash-v2", "draft")
	if err := s.UpdatePage(regenerated); err != nil {
		t.Fatalf("UpdatePage: %v", err)
	}

	got, err := s.GetPage("compiled/auth")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if got.Tier != "draft" {
		t.Errorf("tier = %q, want draft — changed content must drop the validated tier", got.Tier)
	}
	// The incoming hash must be persisted: the demoted row now compares
	// against hash-v2, so a later byte-identical regeneration can preserve
	// whatever tier the page holds then.
	if got.ContentHash != "hash-v2" {
		t.Errorf("content_hash = %q, want hash-v2 persisted", got.ContentHash)
	}
}

// TestUpdatePage_IdenticalContentPreservesValidated verifies the
// preserving half of the content-bound rule (VTB-FR-001): a write whose
// content hash equals the stored hash keeps the validated tier even
// though the writer supplied draft — byte-identical regeneration keeps
// the human vouch.
func TestUpdatePage_IdenticalContentPreservesValidated(t *testing.T) {
	s := newTestStore(t)
	insertCompiledPage(t, s, "compiled/auth", "hash-v1", "validated")

	regenerated := newCompiledPage("compiled/auth", "hash-v1", "draft")
	if err := s.UpdatePage(regenerated); err != nil {
		t.Fatalf("UpdatePage: %v", err)
	}

	got, err := s.GetPage("compiled/auth")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if got.Tier != "validated" {
		t.Errorf("tier = %q, want validated preserved on identical content", got.Tier)
	}
}

// TestUpdatePage_HashLessWriterKeepsValidated verifies the empty-hash
// equality (VTB-FR-001): two empty content hashes compare as equal, so a
// writer that does not track content hashes keeps the pre-change
// never-demote behavior for validated pages.
func TestUpdatePage_HashLessWriterKeepsValidated(t *testing.T) {
	s := newTestStore(t)
	insertCompiledPage(t, s, "compiled/auth", "", "validated")

	write := newCompiledPage("compiled/auth", "", "draft")
	if err := s.UpdatePage(write); err != nil {
		t.Fatalf("UpdatePage: %v", err)
	}

	got, err := s.GetPage("compiled/auth")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if got.Tier != "validated" {
		t.Errorf("tier = %q, want validated preserved for a hash-less writer", got.Tier)
	}
}

// TestUpdatePage_PromoteRaceStaysClosed verifies the promotion trust
// boundary across UpdatePage's read-then-write window (VTB-FR-001): a
// caller that reads a draft page, loses a human promotion inside the
// window, and then writes back its stale draft tier with the content hash
// unchanged must not revert the promotion. Promotion remains the only
// path onto validated; this write only preserves it.
func TestUpdatePage_PromoteRaceStaysClosed(t *testing.T) {
	s := newTestStore(t)
	insertCompiledPage(t, s, "compiled/auth", "hash-v1", "draft")

	// The read half of the window: the writer carries away the page's
	// draft tier and its content hash.
	stale, err := s.GetPage("compiled/auth")
	if err != nil || stale == nil {
		t.Fatalf("GetPage: %v (page=%v)", err, stale)
	}

	// The human promotion gate lands inside the window — the only path
	// that may elevate the page.
	if err := s.PromotePagesTiers([]string{"compiled/auth"}); err != nil {
		t.Fatalf("PromotePagesTiers: %v", err)
	}
	promoted, err := s.GetPage("compiled/auth")
	if err != nil || promoted == nil {
		t.Fatalf("GetPage after promote: %v (page=%v)", err, promoted)
	}
	if promoted.Tier != "validated" {
		t.Fatalf("tier after promote = %q, want validated — the race requires an elevated page", promoted.Tier)
	}

	// The write half lands last: the stale draft tier read before the
	// promotion, with the content hash unchanged.
	if err := s.UpdatePage(stale); err != nil {
		t.Fatalf("UpdatePage: %v", err)
	}
	got, err := s.GetPage("compiled/auth")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if got.Tier != "validated" {
		t.Errorf("tier = %q, want validated — a stale draft-tier write with unchanged content must not revert the promotion", got.Tier)
	}
}

// TestUpdatePage_NonValidatedTiersUnaffected verifies the rule's scope
// (VTB-FR-001): a page whose stored tier is not validated takes the
// caller-supplied tier exactly as before, on hash match and hash change
// alike — the content-hash comparison never widens into tier behavior for
// non-validated pages.
func TestUpdatePage_NonValidatedTiersUnaffected(t *testing.T) {
	tests := []struct {
		name       string
		storedTier string
		storedHash string
		writeHash  string
		writeTier  string
		wantTier   string
	}{
		{name: "draft stored, hash match", storedTier: "draft", storedHash: "hash-v1", writeHash: "hash-v1", writeTier: "curated", wantTier: "curated"},
		{name: "draft stored, hash change", storedTier: "draft", storedHash: "hash-v1", writeHash: "hash-v2", writeTier: "curated", wantTier: "curated"},
		// The everyday compile regeneration on a never-promoted page.
		{name: "draft stored, hash change, draft write", storedTier: "draft", storedHash: "hash-v1", writeHash: "hash-v2", writeTier: "draft", wantTier: "draft"},
		{name: "curated stored, hash match", storedTier: "curated", storedHash: "hash-v1", writeHash: "hash-v1", writeTier: "draft", wantTier: "draft"},
		{name: "curated stored, hash change", storedTier: "curated", storedHash: "hash-v1", writeHash: "hash-v2", writeTier: "draft", wantTier: "draft"},
		{name: "authored stored, hash match", storedTier: "authored", storedHash: "hash-v1", writeHash: "hash-v1", writeTier: "draft", wantTier: "draft"},
		{name: "authored stored, hash change", storedTier: "authored", storedHash: "hash-v1", writeHash: "hash-v2", writeTier: "draft", wantTier: "draft"},
		// Empty hashes compare as equal, but that equality only guards
		// validated rows — a hash-less write to a non-validated page still
		// takes the caller-supplied tier.
		{name: "draft stored, hash-less writer", storedTier: "draft", storedHash: "", writeHash: "", writeTier: "curated", wantTier: "curated"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestStore(t)
			insertCompiledPage(t, s, "compiled/topic", tc.storedHash, tc.storedTier)

			write := newCompiledPage("compiled/topic", tc.writeHash, tc.writeTier)
			if err := s.UpdatePage(write); err != nil {
				t.Fatalf("UpdatePage: %v", err)
			}

			got, err := s.GetPage("compiled/topic")
			if err != nil {
				t.Fatalf("GetPage: %v", err)
			}
			if got.Tier != tc.wantTier {
				t.Errorf("tier = %q, want %q (stored tier %q, write hash %q)", got.Tier, tc.wantTier, tc.storedTier, tc.writeHash)
			}
		})
	}
}

// --- Supersession claim (CAS) operations ---

// insertClaimTarget inserts a page seeded for ClaimSupersession tests and
// fails the test on insert errors. UpdatedAt is seeded to a deliberately old
// timestamp (1) so assertions that a landed claim advances updated_at cannot
// pass merely because the insert and the claim landed in the same
// millisecond — no sleeps needed.
func insertClaimTarget(t *testing.T, s *Store, name, sourceID, properties string) {
	t.Helper()
	if err := s.InsertPage(&Page{
		Name:         name,
		OriginalName: name,
		SourceID:     sourceID,
		SourceDocID:  name,
		Properties:   properties,
		ContentHash:  "claim-hash",
		Tier:         "draft",
		Category:     "gotcha",
		CreatedAt:    1,
		UpdatedAt:    1,
	}); err != nil {
		t.Fatalf("InsertPage(%q): %v", name, err)
	}
}

// TestClaimSupersession_ClaimLands verifies the happy path of the
// compare-and-swap: an unclaimed learning's slot is taken, the claimed
// properties JSON replaces the column verbatim, updated_at advances in Unix
// milliseconds, and every other column keeps its seeded value — the claim
// writes only the properties and updated_at columns, never the whole row.
func TestClaimSupersession_ClaimLands(t *testing.T) {
	s := newTestStore(t)
	const target = "learning/auth-20260502T143022-alice"
	insertClaimTarget(t, s, target, SourceLearning, `{"validity":"current"}`)
	claimProps := `{"validity":"superseded","superseded_by":"auth-20260601T120000-bob"}`

	before := time.Now().UnixMilli()
	claimed, winner, err := s.ClaimSupersession(target, claimProps)
	after := time.Now().UnixMilli()
	if err != nil {
		t.Fatalf("ClaimSupersession: %v", err)
	}
	if !claimed {
		t.Fatal("claimed = false, want true for an unclaimed slot")
	}
	if winner != "" {
		t.Errorf("winner = %q, want empty on a landed claim", winner)
	}

	got, err := s.GetPage(target)
	if err != nil || got == nil {
		t.Fatalf("GetPage after claim: %v (page=%v)", err, got)
	}
	if got.Properties != claimProps {
		t.Errorf("properties = %q, want the claimed %q verbatim", got.Properties, claimProps)
	}

	// updated_at must advance in Unix milliseconds: strictly past the seeded
	// 1, and bracketed by wall-clock UnixMilli readings around the claim —
	// a seconds-precision write would sit three orders of magnitude below
	// the bracket.
	if got.UpdatedAt <= 1 {
		t.Errorf("updated_at = %d, want it advanced past the seeded 1", got.UpdatedAt)
	}
	if got.UpdatedAt < before || got.UpdatedAt > after {
		t.Errorf("updated_at = %d, want Unix milliseconds within [%d, %d]", got.UpdatedAt, before, after)
	}

	// The claim writes only properties and updated_at; every other column
	// keeps its seeded value.
	if got.Name != target || got.OriginalName != target {
		t.Errorf("name/original_name = %q/%q, want %q untouched", got.Name, got.OriginalName, target)
	}
	if got.SourceID != SourceLearning {
		t.Errorf("source_id = %q, want %q untouched", got.SourceID, SourceLearning)
	}
	if got.SourceDocID != target {
		t.Errorf("source_doc_id = %q, want %q untouched", got.SourceDocID, target)
	}
	if got.ContentHash != "claim-hash" {
		t.Errorf("content_hash = %q, want the seeded claim-hash untouched", got.ContentHash)
	}
	if got.Tier != "draft" {
		t.Errorf("tier = %q, want the seeded draft untouched", got.Tier)
	}
	if got.Category != "gotcha" {
		t.Errorf("category = %q, want the seeded gotcha untouched", got.Category)
	}
	if got.CreatedAt != 1 {
		t.Errorf("created_at = %d, want the seeded 1 untouched", got.CreatedAt)
	}
	if got.IsJournal {
		t.Error("is_journal = true, want false untouched")
	}
}

// TestClaimSupersession_SecondClaimRefusesWithWinner verifies the lost-claim
// outcome: a second claim on a taken slot refuses without error and reports
// the recorded successor identity as the winner, leaving the first claim's
// write — properties and updated_at — intact.
func TestClaimSupersession_SecondClaimRefusesWithWinner(t *testing.T) {
	s := newTestStore(t)
	const target = "learning/auth-20260502T143022-alice"
	insertClaimTarget(t, s, target, SourceLearning, `{"validity":"current"}`)

	first := `{"validity":"superseded","superseded_by":"auth-20260601T120000-bob"}`
	claimed, winner, err := s.ClaimSupersession(target, first)
	if err != nil {
		t.Fatalf("first ClaimSupersession: %v", err)
	}
	if !claimed || winner != "" {
		t.Fatalf("first claim: claimed=%v winner=%q, want true/empty", claimed, winner)
	}
	afterFirst, err := s.GetPage(target)
	if err != nil || afterFirst == nil {
		t.Fatalf("GetPage after first claim: %v (page=%v)", err, afterFirst)
	}
	if afterFirst.UpdatedAt == 1 {
		t.Fatal("first claim did not advance updated_at")
	}

	second := `{"validity":"superseded","superseded_by":"auth-20260602T130000-carol"}`
	claimed, winner, err = s.ClaimSupersession(target, second)
	if err != nil {
		t.Fatalf("second ClaimSupersession: %v (a refused claim with a recorded winner is not an error)", err)
	}
	if claimed {
		t.Error("claimed = true, want false — the slot is already taken")
	}
	if want := "auth-20260601T120000-bob"; winner != want {
		t.Errorf("winner = %q, want the first claim's successor %q", winner, want)
	}

	// The refused claim must leave the first claim's write intact.
	got, err := s.GetPage(target)
	if err != nil || got == nil {
		t.Fatalf("GetPage after second claim: %v (page=%v)", err, got)
	}
	if got.Properties != first {
		t.Errorf("properties = %q, want the first claim's %q untouched", got.Properties, first)
	}
	if got.UpdatedAt != afterFirst.UpdatedAt {
		t.Errorf("updated_at = %d, want %d from the landed claim (a refused claim must not bump it)", got.UpdatedAt, afterFirst.UpdatedAt)
	}
}

// TestClaimSupersession_UnclaimedPropertiesPassGuard verifies every
// unclaimed properties form passes the SQL guard and takes the slot: SQL
// NULL, empty string, a JSON null document, and a missing key, plus the
// boundary forms where the key exists but holds an unclaimed value. The
// empty-string row exercises the dedicated disjunct for an empty-string
// column — json_extract raises a malformed-JSON error on an empty-string
// document rather than returning NULL (the GetPagesWithProperty precedent),
// so COALESCE alone cannot absorb it.
func TestClaimSupersession_UnclaimedPropertiesPassGuard(t *testing.T) {
	tests := []struct {
		name       string
		properties string
		// sqlNull rewrites the column to SQL NULL after insert; InsertPage
		// binds Go "" as SQL '', not NULL.
		sqlNull bool
	}{
		{name: "sql null", sqlNull: true},
		{name: "empty string", properties: ""},
		{name: "json null document", properties: "null"},
		{name: "empty object", properties: "{}"},
		{name: "superseded_by key absent", properties: `{"validity":"current","author":"alice"}`},
		// json_extract yields SQL NULL for a JSON null value and '' for an
		// empty string — both unclaimed, matching decodeLearningProperties.
		{name: "superseded_by json null", properties: `{"superseded_by": null}`},
		{name: "superseded_by empty string", properties: `{"superseded_by": ""}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestStore(t)
			const target = "learning/auth-20260502T143022-alice"
			insertClaimTarget(t, s, target, SourceLearning, tc.properties)
			if tc.sqlNull {
				if _, err := s.db.Exec(`UPDATE pages SET properties = NULL WHERE name = ?`, target); err != nil {
					t.Fatalf("set properties to NULL: %v", err)
				}
			}

			claimProps := `{"validity":"superseded","superseded_by":"auth-20260601T120000-bob"}`
			claimed, winner, err := s.ClaimSupersession(target, claimProps)
			if err != nil {
				t.Fatalf("ClaimSupersession: %v", err)
			}
			if !claimed {
				t.Fatal("claimed = false, want true — unclaimed properties must pass the guard")
			}
			if winner != "" {
				t.Errorf("winner = %q, want empty on a landed claim", winner)
			}

			got, err := s.GetPage(target)
			if err != nil || got == nil {
				t.Fatalf("GetPage: %v (page=%v)", err, got)
			}
			if got.Properties != claimProps {
				t.Errorf("properties = %q, want the claimed %q — the row must actually update", got.Properties, claimProps)
			}
			if got.UpdatedAt <= 1 {
				t.Errorf("updated_at = %d, want it advanced past the seeded 1", got.UpdatedAt)
			}
		})
	}
}

// TestClaimSupersession_RefusalFailsClosed verifies every zero-row refusal
// that is not a lost race surfaces as a hard error rather than an empty
// winner, and that the refused or failed claim leaves the target row
// untouched.
func TestClaimSupersession_RefusalFailsClosed(t *testing.T) {
	tests := []struct {
		name            string
		properties      string
		sourceID        string
		missing         bool
		wantErrContains string
	}{
		{name: "missing page", missing: true, wantErrContains: "not found for supersession claim"},
		// The recorded successor on a non-learning row must error, not
		// surface as the winner — the source_id guard re-asserts row
		// identity the way PromotePagesTiers' tier predicate does.
		{
			name:            "non-learning source_id",
			properties:      `{"superseded_by":"auth-20260601T120000-bob"}`,
			sourceID:        "disk-local",
			wantErrContains: "is no longer a learning",
		},
		// json_extract returns the raw JSON value for each non-string
		// shape; the re-read's comma-ok assertion must hard-error instead
		// of yielding an empty winner.
		{name: "non-string superseded_by number", properties: `{"superseded_by": 42}`, sourceID: SourceLearning, wantErrContains: "non-string superseded_by"},
		{name: "non-string superseded_by boolean", properties: `{"superseded_by": true}`, sourceID: SourceLearning, wantErrContains: "non-string superseded_by"},
		{name: "non-string superseded_by object", properties: `{"superseded_by": {}}`, sourceID: SourceLearning, wantErrContains: "non-string superseded_by"},
		{name: "non-string superseded_by array", properties: `{"superseded_by": []}`, sourceID: SourceLearning, wantErrContains: "non-string superseded_by"},
		// SQLite's JSON parser accepts \x hex escapes (a JSON5 extension)
		// that Go's encoding/json rejects, so this document is parseable
		// by the SQL guard — json_extract reads superseded_by = "winner"
		// and refuses the claim — while the disambiguating re-read fails
		// json.Unmarshal: the fail-closed decode path.
		{
			name:            "undecodable properties on re-read",
			properties:      `{"superseded_by": "winner", "note": "a\x01b"}`,
			sourceID:        SourceLearning,
			wantErrContains: "decode properties of",
		},
		// Malformed non-empty JSON makes json_extract itself error, so the
		// UPDATE fails closed before any row is written.
		{name: "malformed properties", properties: `{not json`, sourceID: SourceLearning, wantErrContains: "claim supersession for"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestStore(t)
			const target = "learning/auth-20260502T143022-alice"
			if !tc.missing {
				insertClaimTarget(t, s, target, tc.sourceID, tc.properties)
			}

			claimed, winner, err := s.ClaimSupersession(target, `{"validity":"superseded","superseded_by":"auth-20260602T130000-carol"}`)
			if err == nil {
				t.Fatalf("ClaimSupersession error = nil, want a failure containing %q", tc.wantErrContains)
			}
			if !strings.Contains(err.Error(), tc.wantErrContains) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tc.wantErrContains)
			}
			if claimed {
				t.Error("claimed = true, want false")
			}
			if winner != "" {
				t.Errorf("winner = %q, want empty — the refusal must error rather than yield a winner", winner)
			}
			if tc.missing {
				return
			}

			// A refused or failed claim must leave the target row
			// untouched.
			got, err := s.GetPage(target)
			if err != nil || got == nil {
				t.Fatalf("GetPage after refused claim: %v (page=%v)", err, got)
			}
			if got.Properties != tc.properties {
				t.Errorf("properties = %q, want the seeded %q untouched", got.Properties, tc.properties)
			}
			if got.UpdatedAt != 1 {
				t.Errorf("updated_at = %d, want the seeded 1 (a refused claim must not bump it)", got.UpdatedAt)
			}
			if got.Tier != "draft" || got.ContentHash != "claim-hash" {
				t.Errorf("tier = %q, content_hash = %q, want the seeded draft/claim-hash untouched", got.Tier, got.ContentHash)
			}
		})
	}
}
