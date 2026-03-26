package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	if version != "1" {
		t.Errorf("schema_version = %q, want %q", version, "1")
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
	if version != "1" {
		t.Errorf("schema_version = %q, want %q", version, "1")
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
