package store

import (
	"database/sql"
	"fmt"
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
	t.Cleanup(func() { s.Close() })
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
	defer s.Close()

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
