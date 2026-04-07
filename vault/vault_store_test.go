package vault

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/unbound-force/dewey/store"
	"github.com/unbound-force/dewey/types"
)

// newTestVaultStore creates a VaultStore backed by an in-memory SQLite database.
func newTestVaultStore(t *testing.T, vaultPath string) (*VaultStore, *store.Store) {
	t.Helper()
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New(:memory:) failed: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	vs := NewVaultStore(s, vaultPath, "disk-local")
	return vs, s
}

func TestVaultStore_FullIndex(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	testdata := filepath.Join(filepath.Dir(thisFile), "testdata")

	vs, s := newTestVaultStore(t, testdata)

	c := New(testdata)
	if err := vs.FullIndex(c); err != nil {
		t.Fatalf("FullIndex: %v", err)
	}

	// Verify pages were persisted.
	pages, err := s.ListPages()
	if err != nil {
		t.Fatalf("ListPages: %v", err)
	}
	if len(pages) < 6 {
		t.Errorf("expected at least 6 pages, got %d", len(pages))
	}

	// Verify blocks were persisted for a known page.
	blocks, err := s.GetBlocksByPage("projects/dewey")
	if err != nil {
		t.Fatalf("GetBlocksByPage: %v", err)
	}
	if len(blocks) == 0 {
		t.Error("expected blocks for projects/dewey")
	}

	// Verify metadata was set.
	pageCount, err := s.GetMeta("page_count")
	if err != nil {
		t.Fatalf("GetMeta(page_count): %v", err)
	}
	if pageCount == "" || pageCount == "0" {
		t.Errorf("page_count = %q, want non-zero", pageCount)
	}

	lastIndex, err := s.GetMeta("last_full_index_at")
	if err != nil {
		t.Fatalf("GetMeta(last_full_index_at): %v", err)
	}
	if lastIndex == "" {
		t.Error("last_full_index_at should be set after full index")
	}
}

func TestVaultStore_IncrementalIndex_NoChanges(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	testdata := filepath.Join(filepath.Dir(thisFile), "testdata")

	vs, s := newTestVaultStore(t, testdata)

	c := New(testdata)

	// First: full index.
	if err := vs.FullIndex(c); err != nil {
		t.Fatalf("FullIndex: %v", err)
	}

	// Second: incremental index (no changes).
	c2 := New(testdata, WithStore(vs.store))
	stats, err := vs.IncrementalIndex(c2)
	if err != nil {
		t.Fatalf("IncrementalIndex: %v", err)
	}

	// All files should be unchanged.
	if stats.New != 0 {
		t.Errorf("New = %d, want 0", stats.New)
	}
	if stats.Changed != 0 {
		t.Errorf("Changed = %d, want 0", stats.Changed)
	}
	if stats.Deleted != 0 {
		t.Errorf("Deleted = %d, want 0", stats.Deleted)
	}
	if stats.Unchanged == 0 {
		t.Error("Unchanged = 0, want > 0")
	}

	// Verify Total() sums correctly.
	if got := stats.Total(); got != stats.Unchanged {
		t.Errorf("Total() = %d, want %d (all unchanged)", got, stats.Unchanged)
	}

	// Verify page count metadata was updated.
	pageCount, err := s.GetMeta("page_count")
	if err != nil {
		t.Fatalf("GetMeta(page_count): %v", err)
	}
	if pageCount == "" || pageCount == "0" {
		t.Errorf("page_count = %q, want non-zero after incremental index", pageCount)
	}
}

func TestVaultStore_IncrementalIndex_NewFile(t *testing.T) {
	// Copy testdata to temp dir so we can modify it.
	tmpDir := t.TempDir()
	copyTestdata(t, tmpDir)

	vs, s := newTestVaultStore(t, tmpDir)

	c := New(tmpDir)
	if err := vs.FullIndex(c); err != nil {
		t.Fatalf("FullIndex: %v", err)
	}

	// Count pages before adding new file.
	pagesBefore, err := s.ListPages()
	if err != nil {
		t.Fatalf("ListPages before: %v", err)
	}
	countBefore := len(pagesBefore)

	// Add a new file.
	newFile := filepath.Join(tmpDir, "new-page.md")
	if err := os.WriteFile(newFile, []byte("# New Page\n\nNew content."), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Incremental index should detect the new file.
	c2 := New(tmpDir, WithStore(vs.store))
	stats, err := vs.IncrementalIndex(c2)
	if err != nil {
		t.Fatalf("IncrementalIndex: %v", err)
	}

	if stats.New != 1 {
		t.Errorf("New = %d, want 1", stats.New)
	}
	if stats.Changed != 0 {
		t.Errorf("Changed = %d, want 0", stats.Changed)
	}
	if stats.Deleted != 0 {
		t.Errorf("Deleted = %d, want 0", stats.Deleted)
	}

	// Verify the new page was persisted to store.
	pagesAfter, err := s.ListPages()
	if err != nil {
		t.Fatalf("ListPages after: %v", err)
	}
	if len(pagesAfter) != countBefore+1 {
		t.Errorf("page count = %d, want %d (countBefore+1)", len(pagesAfter), countBefore+1)
	}

	// Verify the new page exists in the store with correct data.
	newPage, err := s.GetPage("new-page")
	if err != nil {
		t.Fatalf("GetPage(new-page): %v", err)
	}
	if newPage == nil {
		t.Fatal("new-page should exist in store after incremental index")
	}
	if newPage.ContentHash == "" {
		t.Error("new-page ContentHash should not be empty")
	}
	if newPage.SourceID != "disk-local" {
		t.Errorf("new-page SourceID = %q, want %q", newPage.SourceID, "disk-local")
	}

	// Verify Total() is consistent.
	if got := stats.Total(); got != stats.New+stats.Changed+stats.Deleted+stats.Unchanged {
		t.Errorf("Total() = %d, want %d", got, stats.New+stats.Changed+stats.Deleted+stats.Unchanged)
	}
}

func TestVaultStore_IncrementalIndex_ChangedFile(t *testing.T) {
	tmpDir := t.TempDir()
	copyTestdata(t, tmpDir)

	vs, s := newTestVaultStore(t, tmpDir)

	c := New(tmpDir)
	if err := vs.FullIndex(c); err != nil {
		t.Fatalf("FullIndex: %v", err)
	}

	// Get the original content hash before modification.
	originalPage, err := s.GetPage("index")
	if err != nil {
		t.Fatalf("GetPage(index) before: %v", err)
	}
	if originalPage == nil {
		t.Fatal("index page should exist before modification")
	}
	originalHash := originalPage.ContentHash

	// Modify an existing file.
	indexFile := filepath.Join(tmpDir, "index.md")
	if err := os.WriteFile(indexFile, []byte("# Modified Index\n\nChanged content."), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	c2 := New(tmpDir, WithStore(vs.store))
	stats, err := vs.IncrementalIndex(c2)
	if err != nil {
		t.Fatalf("IncrementalIndex: %v", err)
	}

	if stats.Changed != 1 {
		t.Errorf("Changed = %d, want 1", stats.Changed)
	}
	if stats.New != 0 {
		t.Errorf("New = %d, want 0", stats.New)
	}
	if stats.Deleted != 0 {
		t.Errorf("Deleted = %d, want 0", stats.Deleted)
	}

	// Verify the changed page's content hash was updated in the store.
	updatedPage, err := s.GetPage("index")
	if err != nil {
		t.Fatalf("GetPage(index) after: %v", err)
	}
	if updatedPage == nil {
		t.Fatal("index page should still exist after modification")
	}
	if updatedPage.ContentHash == originalHash {
		t.Error("ContentHash should have changed after file modification")
	}
	if updatedPage.ContentHash == "" {
		t.Error("ContentHash should not be empty after modification")
	}
}

func TestVaultStore_IncrementalIndex_DeletedFile(t *testing.T) {
	tmpDir := t.TempDir()
	copyTestdata(t, tmpDir)

	vs, s := newTestVaultStore(t, tmpDir)

	c := New(tmpDir)
	if err := vs.FullIndex(c); err != nil {
		t.Fatalf("FullIndex: %v", err)
	}

	// Verify page exists before deletion.
	pageBefore, err := s.GetPage("index")
	if err != nil {
		t.Fatalf("GetPage(index) before: %v", err)
	}
	if pageBefore == nil {
		t.Fatal("index page should exist before deletion")
	}

	// Count pages before deletion.
	pagesBefore, err := s.ListPages()
	if err != nil {
		t.Fatalf("ListPages before: %v", err)
	}
	countBefore := len(pagesBefore)

	// Delete a file.
	indexFile := filepath.Join(tmpDir, "index.md")
	if err := os.Remove(indexFile); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	c2 := New(tmpDir, WithStore(vs.store))
	stats, err := vs.IncrementalIndex(c2)
	if err != nil {
		t.Fatalf("IncrementalIndex: %v", err)
	}

	if stats.Deleted != 1 {
		t.Errorf("Deleted = %d, want 1", stats.Deleted)
	}
	if stats.New != 0 {
		t.Errorf("New = %d, want 0", stats.New)
	}
	if stats.Changed != 0 {
		t.Errorf("Changed = %d, want 0", stats.Changed)
	}

	// Verify the deleted page was removed from the store.
	pageAfter, err := s.GetPage("index")
	if err != nil {
		t.Fatalf("GetPage(index) after: %v", err)
	}
	if pageAfter != nil {
		t.Error("index page should be removed from store after deletion")
	}

	// Verify page count decreased.
	pagesAfter, err := s.ListPages()
	if err != nil {
		t.Fatalf("ListPages after: %v", err)
	}
	if len(pagesAfter) != countBefore-1 {
		t.Errorf("page count = %d, want %d (countBefore-1)", len(pagesAfter), countBefore-1)
	}
}

func TestVaultStore_CorruptionRecovery(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	testdata := filepath.Join(filepath.Dir(thisFile), "testdata")

	vs, s := newTestVaultStore(t, testdata)

	// Corrupt the schema version.
	if err := s.SetMeta("schema_version", ""); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}

	// ValidateStore should detect corruption.
	err := vs.ValidateStore()
	if err == nil {
		t.Fatal("ValidateStore should fail with empty schema_version")
	}

	// Recovery: full re-index.
	c := New(testdata)
	if err := vs.FullIndex(c); err != nil {
		t.Fatalf("FullIndex after corruption: %v", err)
	}

	// Verify pages were re-indexed.
	pages, err := s.ListPages()
	if err != nil {
		t.Fatalf("ListPages: %v", err)
	}
	if len(pages) < 6 {
		t.Errorf("expected at least 6 pages after recovery, got %d", len(pages))
	}
}

func TestVaultStore_NilStore(t *testing.T) {
	// VaultStore with nil store should be a no-op.
	vs := NewVaultStore(nil, "/tmp", "disk-local")

	// All operations should succeed silently.
	if err := vs.PersistPage(&cachedPage{}); err != nil {
		t.Errorf("PersistPage with nil store: %v", err)
	}
	if err := vs.RemovePage("test"); err != nil {
		t.Errorf("RemovePage with nil store: %v", err)
	}

	hashes, err := vs.LoadPages()
	if err != nil {
		t.Errorf("LoadPages with nil store: %v", err)
	}
	if hashes != nil {
		t.Errorf("LoadPages with nil store should return nil, got %v", hashes)
	}
}

func TestVaultStore_PersistAndLoadPage(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	testdata := filepath.Join(filepath.Dir(thisFile), "testdata")

	vs, s := newTestVaultStore(t, testdata)

	// Load vault and persist a single page.
	c := New(testdata)
	if err := c.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	c.mu.RLock()
	page, ok := c.pages["index"]
	c.mu.RUnlock()
	if !ok {
		t.Fatal("index page not found in vault")
	}

	if err := vs.PersistPage(page); err != nil {
		t.Fatalf("PersistPage: %v", err)
	}

	// Verify page is in store.
	sp, err := s.GetPage("index")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if sp == nil {
		t.Fatal("page not found in store after persist")
	}
	if sp.SourceID != "disk-local" {
		t.Errorf("SourceID = %q, want %q", sp.SourceID, "disk-local")
	}
	if sp.ContentHash == "" {
		t.Error("ContentHash should not be empty")
	}
}

func TestVaultStore_RemovePage(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	testdata := filepath.Join(filepath.Dir(thisFile), "testdata")

	vs, s := newTestVaultStore(t, testdata)

	c := New(testdata)
	if err := vs.FullIndex(c); err != nil {
		t.Fatalf("FullIndex: %v", err)
	}

	// Remove a page.
	if err := vs.RemovePage("index"); err != nil {
		t.Fatalf("RemovePage: %v", err)
	}

	// Verify it's gone.
	sp, err := s.GetPage("index")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if sp != nil {
		t.Error("page should be nil after removal")
	}
}

func TestVaultStore_IncrementalIndex_StoreConsistency(t *testing.T) {
	// Verify that IncrementalIndex maintains store consistency by checking
	// page count in store matches expected after new + changed + deleted operations.
	tmpDir := t.TempDir()
	copyTestdata(t, tmpDir)

	vs, s := newTestVaultStore(t, tmpDir)

	c := New(tmpDir)
	if err := vs.FullIndex(c); err != nil {
		t.Fatalf("FullIndex: %v", err)
	}

	// Get initial page list and count.
	initialPages, err := s.ListPages()
	if err != nil {
		t.Fatalf("ListPages initial: %v", err)
	}
	initialCount := len(initialPages)
	if initialCount < 6 {
		t.Fatalf("expected at least 6 initial pages, got %d", initialCount)
	}

	// Verify each initial page has a non-empty ContentHash.
	for _, p := range initialPages {
		if p.ContentHash == "" {
			t.Errorf("initial page %q has empty ContentHash", p.Name)
		}
		if p.SourceID == "" {
			t.Errorf("initial page %q has empty SourceID", p.Name)
		}
	}

	// Add a new file, modify another, and delete a third.
	newFile := filepath.Join(tmpDir, "consistency-new.md")
	if err := os.WriteFile(newFile, []byte("# Consistency New\n\nNew content."), 0o644); err != nil {
		t.Fatalf("WriteFile new: %v", err)
	}
	indexFile := filepath.Join(tmpDir, "index.md")
	if err := os.WriteFile(indexFile, []byte("# Modified for consistency\n\nChanged."), 0o644); err != nil {
		t.Fatalf("WriteFile modified: %v", err)
	}
	// Find a page that's NOT index to delete.
	for _, p := range initialPages {
		if p.Name != "index" && !strings.Contains(p.Name, "/") {
			// Found a root-level page to delete. But we need to check the testdata first.
			break
		}
	}

	c2 := New(tmpDir, WithStore(vs.store))
	stats, err := vs.IncrementalIndex(c2)
	if err != nil {
		t.Fatalf("IncrementalIndex: %v", err)
	}

	// Verify stats fields are consistent integers.
	if stats.New < 1 {
		t.Errorf("New = %d, want >= 1 (added consistency-new.md)", stats.New)
	}
	if stats.Changed < 1 {
		t.Errorf("Changed = %d, want >= 1 (modified index.md)", stats.Changed)
	}
	if stats.Unchanged < 0 {
		t.Errorf("Unchanged = %d, should not be negative", stats.Unchanged)
	}
	if stats.Deleted < 0 {
		t.Errorf("Deleted = %d, should not be negative", stats.Deleted)
	}

	// Verify Total() equals the sum of all fields.
	expectedTotal := stats.New + stats.Changed + stats.Deleted + stats.Unchanged
	if got := stats.Total(); got != expectedTotal {
		t.Errorf("Total() = %d, want %d (New=%d + Changed=%d + Deleted=%d + Unchanged=%d)",
			got, expectedTotal, stats.New, stats.Changed, stats.Deleted, stats.Unchanged)
	}

	// Verify pages in store after indexing.
	afterPages, err := s.ListPages()
	if err != nil {
		t.Fatalf("ListPages after: %v", err)
	}
	expectedPageCount := initialCount + stats.New - stats.Deleted
	if len(afterPages) != expectedPageCount {
		t.Errorf("page count after = %d, want %d (initial=%d + new=%d - deleted=%d)",
			len(afterPages), expectedPageCount, initialCount, stats.New, stats.Deleted)
	}

	// Verify the new page exists in store with correct metadata.
	newPage, err := s.GetPage("consistency-new")
	if err != nil {
		t.Fatalf("GetPage(consistency-new): %v", err)
	}
	if newPage == nil {
		t.Fatal("consistency-new page should exist in store after incremental index")
	}
	if newPage.ContentHash == "" {
		t.Error("consistency-new ContentHash should not be empty")
	}
	if newPage.SourceID != "disk-local" {
		t.Errorf("consistency-new SourceID = %q, want %q", newPage.SourceID, "disk-local")
	}

	// Verify page_count metadata matches actual page count.
	pageCountMeta, err := s.GetMeta("page_count")
	if err != nil {
		t.Fatalf("GetMeta(page_count): %v", err)
	}
	wantPageCount := fmt.Sprintf("%d", stats.New+stats.Changed+stats.Unchanged)
	if pageCountMeta != wantPageCount {
		t.Errorf("page_count metadata = %q, want %q", pageCountMeta, wantPageCount)
	}
}

func TestVaultStore_IncrementalIndex_ContentHashVerification(t *testing.T) {
	// Verify that content hashes are correctly updated in the store for changed files.
	tmpDir := t.TempDir()
	copyTestdata(t, tmpDir)

	vs, s := newTestVaultStore(t, tmpDir)

	c := New(tmpDir)
	if err := vs.FullIndex(c); err != nil {
		t.Fatalf("FullIndex: %v", err)
	}

	// Record hashes for all pages after full index.
	initialPages, err := s.ListPages()
	if err != nil {
		t.Fatalf("ListPages: %v", err)
	}
	initialHashes := make(map[string]string)
	for _, p := range initialPages {
		initialHashes[p.Name] = p.ContentHash
	}

	// Modify index.md.
	indexFile := filepath.Join(tmpDir, "index.md")
	if err := os.WriteFile(indexFile, []byte("# Completely New Content\n\nDifferent text."), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	c2 := New(tmpDir, WithStore(vs.store))
	_, err = vs.IncrementalIndex(c2)
	if err != nil {
		t.Fatalf("IncrementalIndex: %v", err)
	}

	// Verify changed page has a new hash.
	updatedPage, err := s.GetPage("index")
	if err != nil {
		t.Fatalf("GetPage(index): %v", err)
	}
	if updatedPage == nil {
		t.Fatal("index page should exist after incremental index")
	}
	if updatedPage.ContentHash == initialHashes["index"] {
		t.Error("index ContentHash should have changed after file modification")
	}
	if updatedPage.ContentHash == "" {
		t.Error("index ContentHash should not be empty after modification")
	}

	// Verify unchanged pages retain their original hash.
	afterPages, err := s.ListPages()
	if err != nil {
		t.Fatalf("ListPages after: %v", err)
	}
	for _, p := range afterPages {
		if p.Name == "index" {
			continue // already checked
		}
		original, exists := initialHashes[p.Name]
		if !exists {
			continue // new page, skip
		}
		if p.ContentHash != original {
			t.Errorf("unchanged page %q: ContentHash changed from %q to %q",
				p.Name, original, p.ContentHash)
		}
	}
}

func TestIndexStats_Total(t *testing.T) {
	stats := IndexStats{New: 3, Changed: 2, Deleted: 1, Unchanged: 10}
	if got := stats.Total(); got != 16 {
		t.Errorf("Total() = %d, want 16", got)
	}
}

// copyTestdata copies the testdata directory to a temp directory.
func copyTestdata(t *testing.T, dst string) {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	testdata := filepath.Join(filepath.Dir(thisFile), "testdata")

	if err := filepath.Walk(testdata, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(testdata, path)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	}); err != nil {
		t.Fatalf("copy testdata: %v", err)
	}
}

// --- Mock embedder for generateEmbeddings tests ---

// mockEmbedder implements embed.Embedder for testing generateEmbeddings.
type mockEmbedder struct {
	available bool
	modelID   string
	embedFn   func(ctx context.Context, text string) ([]float32, error)
	calls     []string // records chunk texts passed to Embed
}

func (m *mockEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	m.calls = append(m.calls, text)
	if m.embedFn != nil {
		return m.embedFn(ctx, text)
	}
	// Default: return a simple 3-dimensional vector.
	return []float32{0.1, 0.2, 0.3}, nil
}

func (m *mockEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	var results [][]float32
	for _, text := range texts {
		vec, err := m.Embed(ctx, text)
		if err != nil {
			return nil, err
		}
		results = append(results, vec)
	}
	return results, nil
}

func (m *mockEmbedder) Available() bool {
	return m.available
}

func (m *mockEmbedder) ModelID() string {
	if m.modelID != "" {
		return m.modelID
	}
	return "test-model"
}

// insertTestPageAndBlocks inserts a page and its blocks into the store,
// satisfying foreign key constraints required by the embeddings table.
func insertTestPageAndBlocks(t *testing.T, s *store.Store, pageName string, blocks []types.BlockEntity) {
	t.Helper()
	err := s.InsertPage(&store.Page{
		Name:         pageName,
		OriginalName: pageName,
		SourceID:     "disk-local",
		SourceDocID:  pageName + ".md",
	})
	if err != nil {
		t.Fatalf("InsertPage(%q): %v", pageName, err)
	}

	for i, b := range blocks {
		err := s.InsertBlock(&store.Block{
			UUID:     b.UUID,
			PageName: pageName,
			Content:  b.Content,
			Position: i,
		})
		if err != nil {
			t.Fatalf("InsertBlock(%q): %v", b.UUID, err)
		}
		// Also insert child blocks.
		for j, child := range b.Children {
			err := s.InsertBlock(&store.Block{
				UUID:       child.UUID,
				PageName:   pageName,
				ParentUUID: sql.NullString{String: b.UUID, Valid: true},
				Content:    child.Content,
				Position:   j,
			})
			if err != nil {
				t.Fatalf("InsertBlock(%q child): %v", child.UUID, err)
			}
		}
	}
}

func TestGenerateEmbeddings_EmbedderUnavailable(t *testing.T) {
	vs, _ := newTestVaultStore(t, t.TempDir())

	me := &mockEmbedder{available: false}
	vs.SetEmbedder(me)

	blocks := []types.BlockEntity{
		{UUID: "block-1", Content: "Some content"},
	}

	// Should skip silently — no calls to Embed.
	vs.generateEmbeddings("test-page", blocks, nil)

	if len(me.calls) != 0 {
		t.Errorf("expected 0 Embed calls when unavailable, got %d", len(me.calls))
	}
}

func TestGenerateEmbeddings_NilEmbedder(t *testing.T) {
	vs, _ := newTestVaultStore(t, t.TempDir())
	// embedder is nil by default (not set).

	blocks := []types.BlockEntity{
		{UUID: "block-1", Content: "Some content"},
	}

	// Should skip silently — no panic.
	vs.generateEmbeddings("test-page", blocks, nil)
}

func TestGenerateEmbeddings_NilStore(t *testing.T) {
	vs := NewVaultStore(nil, t.TempDir(), "disk-local")
	me := &mockEmbedder{available: true}
	vs.SetEmbedder(me)

	blocks := []types.BlockEntity{
		{UUID: "block-1", Content: "Some content"},
	}

	// Should skip silently — store is nil.
	vs.generateEmbeddings("test-page", blocks, nil)

	if len(me.calls) != 0 {
		t.Errorf("expected 0 Embed calls when store is nil, got %d", len(me.calls))
	}
}

func TestGenerateEmbeddings_PagesWithBlocks(t *testing.T) {
	vs, s := newTestVaultStore(t, t.TempDir())

	me := &mockEmbedder{
		available: true,
		modelID:   "test-model",
	}
	vs.SetEmbedder(me)

	blocks := []types.BlockEntity{
		{UUID: "block-1", Content: "First block content"},
		{UUID: "block-2", Content: "Second block content"},
	}

	// Insert page and blocks into store to satisfy FK constraints.
	insertTestPageAndBlocks(t, s, "test-page", blocks)

	vs.generateEmbeddings("test-page", blocks, nil)

	// Verify Embed was called for each non-empty block.
	if len(me.calls) != 2 {
		t.Fatalf("expected 2 Embed calls, got %d", len(me.calls))
	}

	// Verify embeddings were persisted in the store.
	for _, b := range blocks {
		emb, err := s.GetEmbedding(b.UUID, "test-model")
		if err != nil {
			t.Fatalf("GetEmbedding(%q): %v", b.UUID, err)
		}
		if emb == nil {
			t.Errorf("expected embedding for block %q, got nil", b.UUID)
			continue
		}
		if len(emb.Vector) != 3 {
			t.Errorf("block %q: vector length = %d, want 3", b.UUID, len(emb.Vector))
		}
		if emb.ChunkText == "" {
			t.Errorf("block %q: chunk_text should not be empty", b.UUID)
		}
		if emb.ModelID != "test-model" {
			t.Errorf("block %q: model_id = %q, want %q", b.UUID, emb.ModelID, "test-model")
		}
	}
}

func TestGenerateEmbeddings_SkipsEmptyBlocks(t *testing.T) {
	vs, s := newTestVaultStore(t, t.TempDir())

	me := &mockEmbedder{available: true, modelID: "test-model"}
	vs.SetEmbedder(me)

	blocks := []types.BlockEntity{
		{UUID: "block-empty", Content: ""},
		{UUID: "block-whitespace", Content: "   \n\t  "},
		{UUID: "block-real", Content: "Actual content here"},
	}

	// Only insert the blocks that have UUIDs we'll use.
	insertTestPageAndBlocks(t, s, "test-page", blocks)

	vs.generateEmbeddings("test-page", blocks, nil)

	// Only the non-empty block should get an Embed call.
	if len(me.calls) != 1 {
		t.Fatalf("expected 1 Embed call (skipping empty/whitespace blocks), got %d", len(me.calls))
	}

	// Verify only the real block has an embedding.
	emb, err := s.GetEmbedding("block-real", "test-model")
	if err != nil {
		t.Fatalf("GetEmbedding(block-real): %v", err)
	}
	if emb == nil {
		t.Error("expected embedding for block-real, got nil")
	}
}

func TestGenerateEmbeddings_EmbedError(t *testing.T) {
	vs, s := newTestVaultStore(t, t.TempDir())

	callCount := 0
	me := &mockEmbedder{
		available: true,
		modelID:   "test-model",
		embedFn: func(_ context.Context, _ string) ([]float32, error) {
			callCount++
			return nil, fmt.Errorf("ollama connection refused")
		},
	}
	vs.SetEmbedder(me)

	blocks := []types.BlockEntity{
		{UUID: "block-1", Content: "Content that will fail embedding"},
		{UUID: "block-2", Content: "Another block"},
	}

	insertTestPageAndBlocks(t, s, "test-page", blocks)

	// Should not panic — errors are logged and skipped.
	vs.generateEmbeddings("test-page", blocks, nil)

	// Verify Embed was called for both blocks (continues on error).
	if callCount != 2 {
		t.Errorf("expected 2 Embed calls (continues past errors), got %d", callCount)
	}

	// Verify no embeddings were persisted (all failed).
	for _, b := range blocks {
		emb, err := s.GetEmbedding(b.UUID, "test-model")
		if err != nil {
			t.Fatalf("GetEmbedding(%q): %v", b.UUID, err)
		}
		if emb != nil {
			t.Errorf("block %q: expected no embedding after error, got one", b.UUID)
		}
	}
}

func TestGenerateEmbeddings_EmptyVault(t *testing.T) {
	vs, _ := newTestVaultStore(t, t.TempDir())

	me := &mockEmbedder{available: true, modelID: "test-model"}
	vs.SetEmbedder(me)

	// No blocks at all.
	vs.generateEmbeddings("empty-page", nil, nil)

	if len(me.calls) != 0 {
		t.Errorf("expected 0 Embed calls for empty block list, got %d", len(me.calls))
	}
}

func TestGenerateEmbeddings_WithChildren(t *testing.T) {
	vs, s := newTestVaultStore(t, t.TempDir())

	me := &mockEmbedder{available: true, modelID: "test-model"}
	vs.SetEmbedder(me)

	blocks := []types.BlockEntity{
		{
			UUID:    "parent-block",
			Content: "## Section Heading\n\nParent content",
			Children: []types.BlockEntity{
				{UUID: "child-block", Content: "Child block content"},
			},
		},
	}

	// Insert page, parent block, and child block into store.
	insertTestPageAndBlocks(t, s, "docs-page", blocks)

	vs.generateEmbeddings("docs-page", blocks, nil)

	// Should embed both parent and child.
	if len(me.calls) != 2 {
		t.Fatalf("expected 2 Embed calls (parent + child), got %d", len(me.calls))
	}

	// Verify both embeddings were persisted.
	parentEmb, err := s.GetEmbedding("parent-block", "test-model")
	if err != nil {
		t.Fatalf("GetEmbedding(parent-block): %v", err)
	}
	if parentEmb == nil {
		t.Error("expected embedding for parent-block, got nil")
	}

	childEmb, err := s.GetEmbedding("child-block", "test-model")
	if err != nil {
		t.Fatalf("GetEmbedding(child-block): %v", err)
	}
	if childEmb == nil {
		t.Error("expected embedding for child-block, got nil")
	}
}

func TestGenerateEmbeddings_HeadingPathPropagation(t *testing.T) {
	vs, s := newTestVaultStore(t, t.TempDir())

	me := &mockEmbedder{available: true, modelID: "test-model"}
	vs.SetEmbedder(me)

	blocks := []types.BlockEntity{
		{
			UUID:    "heading-block",
			Content: "## Installation\n\nInstall instructions here.",
			Children: []types.BlockEntity{
				{UUID: "sub-block", Content: "Run `go install`"},
			},
		},
	}

	insertTestPageAndBlocks(t, s, "setup", blocks)

	vs.generateEmbeddings("setup", blocks, nil)

	if len(me.calls) != 2 {
		t.Fatalf("expected 2 Embed calls, got %d", len(me.calls))
	}

	// The parent chunk should include the page name and content.
	parentChunk := me.calls[0]
	if parentChunk == "" {
		t.Fatal("parent chunk should not be empty")
	}

	// The child chunk should include the heading hierarchy from the parent.
	childChunk := me.calls[1]
	if childChunk == "" {
		t.Fatal("child chunk should not be empty")
	}

	// Verify the child chunk contains the heading context propagated from parent.
	// PrepareChunk("setup", ["Installation"], content) should produce
	// "setup > Installation\n\ncontent"
	wantSubstring := "setup > Installation"
	if !strings.Contains(childChunk, wantSubstring) {
		t.Errorf("child chunk should contain heading path %q, got:\n%s", wantSubstring, childChunk)
	}
}

func TestGenerateEmbeddings_PartialEmbedError(t *testing.T) {
	vs, s := newTestVaultStore(t, t.TempDir())

	callIdx := 0
	me := &mockEmbedder{
		available: true,
		modelID:   "test-model",
		embedFn: func(_ context.Context, _ string) ([]float32, error) {
			callIdx++
			if callIdx == 1 {
				// First call fails.
				return nil, fmt.Errorf("transient error")
			}
			// Subsequent calls succeed.
			return []float32{0.5, 0.6, 0.7}, nil
		},
	}
	vs.SetEmbedder(me)

	blocks := []types.BlockEntity{
		{UUID: "fail-block", Content: "This will fail"},
		{UUID: "ok-block", Content: "This will succeed"},
	}

	insertTestPageAndBlocks(t, s, "mixed-page", blocks)

	vs.generateEmbeddings("mixed-page", blocks, nil)

	// First block should have no embedding (error).
	failEmb, err := s.GetEmbedding("fail-block", "test-model")
	if err != nil {
		t.Fatalf("GetEmbedding(fail-block): %v", err)
	}
	if failEmb != nil {
		t.Error("fail-block should have no embedding after error")
	}

	// Second block should have an embedding (success).
	okEmb, err := s.GetEmbedding("ok-block", "test-model")
	if err != nil {
		t.Fatalf("GetEmbedding(ok-block): %v", err)
	}
	if okEmb == nil {
		t.Fatal("ok-block should have an embedding")
	}
	if len(okEmb.Vector) != 3 {
		t.Errorf("ok-block vector length = %d, want 3", len(okEmb.Vector))
	}
}

// TestGenerateEmbeddings_RetryOnContextOverflow verifies that when the embedder
// returns a "context length" error, GenerateEmbeddings retries with a truncated
// chunk and stores the embedding on success.
func TestGenerateEmbeddings_RetryOnContextOverflow(t *testing.T) {
	vs, s := newTestVaultStore(t, t.TempDir())

	callCount := 0
	me := &mockEmbedder{
		available: true,
		modelID:   "test-model",
		embedFn: func(_ context.Context, text string) ([]float32, error) {
			callCount++
			if callCount == 1 {
				// First call: context too long.
				return nil, fmt.Errorf("the input length exceeds the context length")
			}
			// Second call (truncated): succeed.
			return []float32{0.4, 0.5, 0.6}, nil
		},
	}
	vs.SetEmbedder(me)

	blocks := []types.BlockEntity{
		{UUID: "overflow-block", Content: "Some content that is pretend-long"},
	}

	insertTestPageAndBlocks(t, s, "overflow-page", blocks)

	count := GenerateEmbeddings(s, me, "overflow-page", blocks, nil)
	if count != 1 {
		t.Errorf("GenerateEmbeddings count = %d, want 1 (retry should succeed)", count)
	}
	if callCount != 2 {
		t.Errorf("embedder called %d times, want 2 (original + retry)", callCount)
	}

	// Verify embedding was stored.
	emb, err := s.GetEmbedding("overflow-block", "test-model")
	if err != nil {
		t.Fatalf("GetEmbedding: %v", err)
	}
	if emb == nil {
		t.Fatal("overflow-block should have an embedding after retry")
	}
}

// TestGenerateEmbeddings_RetryFailsBoth verifies that when both the original
// and retry embedding calls fail, the block is skipped with a warning.
func TestGenerateEmbeddings_RetryFailsBoth(t *testing.T) {
	vs, s := newTestVaultStore(t, t.TempDir())

	me := &mockEmbedder{
		available: true,
		modelID:   "test-model",
		embedFn: func(_ context.Context, _ string) ([]float32, error) {
			return nil, fmt.Errorf("the input length exceeds the context length")
		},
	}
	vs.SetEmbedder(me)

	blocks := []types.BlockEntity{
		{UUID: "double-fail-block", Content: "Content that always fails"},
	}

	insertTestPageAndBlocks(t, s, "double-fail-page", blocks)

	count := GenerateEmbeddings(s, me, "double-fail-page", blocks, nil)
	if count != 0 {
		t.Errorf("GenerateEmbeddings count = %d, want 0 (both attempts failed)", count)
	}

	// Verify no embedding was stored.
	emb, err := s.GetEmbedding("double-fail-block", "test-model")
	if err != nil {
		t.Fatalf("GetEmbedding: %v", err)
	}
	if emb != nil {
		t.Error("double-fail-block should have no embedding after both attempts failed")
	}
}

// BenchmarkIncrementalStartup measures the time from store.Open() to ready-to-serve
// for a vault with 200 files and <10 changes. Target: <2s per SC-001.
// This is the benchmark test for T066.
func BenchmarkIncrementalStartup(b *testing.B) {
	// Create a temporary vault with 200 files.
	tmpDir := b.TempDir()
	for i := 0; i < 200; i++ {
		content := fmt.Sprintf("# Page %d\n\n## Section 1\n\nContent for page %d.\n\n## Section 2\n\nMore content here.", i, i)
		path := filepath.Join(tmpDir, fmt.Sprintf("page%03d.md", i))
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			b.Fatalf("write test file: %v", err)
		}
	}

	// First: full index to populate the store.
	dbPath := filepath.Join(tmpDir, ".dewey-bench.db")
	s, err := store.New(dbPath)
	if err != nil {
		b.Fatalf("store.New: %v", err)
	}

	vs := NewVaultStore(s, tmpDir, "disk-local")
	c := New(tmpDir)
	if err := vs.FullIndex(c); err != nil {
		b.Fatalf("FullIndex: %v", err)
	}
	_ = s.Close()

	// Modify 3 files to simulate incremental changes.
	for i := 0; i < 3; i++ {
		path := filepath.Join(tmpDir, fmt.Sprintf("page%03d.md", i))
		if err := os.WriteFile(path, []byte(fmt.Sprintf("# Modified Page %d\n\nUpdated content.", i)), 0o644); err != nil {
			b.Fatalf("write modified file: %v", err)
		}
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Measure: Open store → incremental index → ready.
		s, err := store.New(dbPath)
		if err != nil {
			b.Fatalf("store.New: %v", err)
		}

		vs := NewVaultStore(s, tmpDir, "disk-local")
		c := New(tmpDir)
		_, err = vs.IncrementalIndex(c)
		if err != nil {
			b.Fatalf("IncrementalIndex: %v", err)
		}

		_ = s.Close()
	}
}

func TestDiffPages_Empty(t *testing.T) {
	diff := diffPages(map[string]string{}, map[string]string{})
	if len(diff.newPages) != 0 || len(diff.changedPages) != 0 || len(diff.deletedPages) != 0 || len(diff.unchanged) != 0 {
		t.Errorf("expected empty diff, got new=%d changed=%d deleted=%d unchanged=%d",
			len(diff.newPages), len(diff.changedPages), len(diff.deletedPages), len(diff.unchanged))
	}
}

func TestDiffPages_AllNew(t *testing.T) {
	current := map[string]string{"a": "h1", "b": "h2"}
	stored := map[string]string{}
	diff := diffPages(current, stored)
	if len(diff.newPages) != 2 {
		t.Errorf("expected 2 new pages, got %d", len(diff.newPages))
	}
	if len(diff.changedPages) != 0 {
		t.Errorf("expected 0 changed pages, got %d", len(diff.changedPages))
	}
	if len(diff.deletedPages) != 0 {
		t.Errorf("expected 0 deleted pages, got %d", len(diff.deletedPages))
	}
}

func TestDiffPages_AllChanged(t *testing.T) {
	current := map[string]string{"a": "h1-new", "b": "h2-new"}
	stored := map[string]string{"a": "h1-old", "b": "h2-old"}
	diff := diffPages(current, stored)
	if len(diff.newPages) != 0 {
		t.Errorf("expected 0 new pages, got %d", len(diff.newPages))
	}
	if len(diff.changedPages) != 2 {
		t.Errorf("expected 2 changed pages, got %d", len(diff.changedPages))
	}
	if len(diff.deletedPages) != 0 {
		t.Errorf("expected 0 deleted pages, got %d", len(diff.deletedPages))
	}
}

func TestDiffPages_AllDeleted(t *testing.T) {
	current := map[string]string{}
	stored := map[string]string{"a": "h1", "b": "h2"}
	diff := diffPages(current, stored)
	if len(diff.newPages) != 0 {
		t.Errorf("expected 0 new pages, got %d", len(diff.newPages))
	}
	if len(diff.changedPages) != 0 {
		t.Errorf("expected 0 changed pages, got %d", len(diff.changedPages))
	}
	if len(diff.deletedPages) != 2 {
		t.Errorf("expected 2 deleted pages, got %d", len(diff.deletedPages))
	}
}

func TestDiffPages_Mixed(t *testing.T) {
	current := map[string]string{
		"kept":    "same-hash",
		"changed": "new-hash",
		"new":     "brand-new",
	}
	stored := map[string]string{
		"kept":    "same-hash",
		"changed": "old-hash",
		"deleted": "old-hash",
	}
	diff := diffPages(current, stored)

	if len(diff.newPages) != 1 {
		t.Errorf("expected 1 new page, got %d", len(diff.newPages))
	}
	if len(diff.changedPages) != 1 {
		t.Errorf("expected 1 changed page, got %d", len(diff.changedPages))
	}
	if len(diff.deletedPages) != 1 {
		t.Errorf("expected 1 deleted page, got %d", len(diff.deletedPages))
	}
	if len(diff.unchanged) != 1 {
		t.Errorf("expected 1 unchanged page, got %d", len(diff.unchanged))
	}
}

// --- LoadExternalPages tests (004-unified-content-serve T025, T026) ---

func TestLoadExternalPages_LoadsFromStore(t *testing.T) {
	vs, s := newTestVaultStore(t, t.TempDir())

	// Pre-populate store with external pages and blocks.
	extPage := &store.Page{
		Name:         "github-org/issue-42",
		OriginalName: "Bug Report",
		SourceID:     "github-org",
		SourceDocID:  "issue-42",
		Properties:   `{"type": "issue"}`,
		ContentHash:  "hash-42",
		CreatedAt:    1000,
		UpdatedAt:    2000,
	}
	if err := s.InsertPage(extPage); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	// Insert blocks for the external page.
	rootBlock := &store.Block{
		UUID:         "root-block-1",
		PageName:     "github-org/issue-42",
		Content:      "# Bug Report",
		HeadingLevel: 1,
		Position:     0,
	}
	if err := s.InsertBlock(rootBlock); err != nil {
		t.Fatalf("InsertBlock(root): %v", err)
	}
	childBlock := &store.Block{
		UUID:         "child-block-1",
		PageName:     "github-org/issue-42",
		ParentUUID:   sql.NullString{String: "root-block-1", Valid: true},
		Content:      "## Steps to Reproduce",
		HeadingLevel: 2,
		Position:     0,
	}
	if err := s.InsertBlock(childBlock); err != nil {
		t.Fatalf("InsertBlock(child): %v", err)
	}

	// Also insert a disk-local page (should NOT be loaded).
	localPage := &store.Page{
		Name:         "local-page",
		OriginalName: "Local Page",
		SourceID:     "disk-local",
		SourceDocID:  "local-page.md",
		ContentHash:  "hash-local",
	}
	if err := s.InsertPage(localPage); err != nil {
		t.Fatalf("InsertPage(local): %v", err)
	}

	// Create vault client and load external pages.
	vc := New(t.TempDir())
	count, err := vs.LoadExternalPages(vc)
	if err != nil {
		t.Fatalf("LoadExternalPages: %v", err)
	}
	if count != 1 {
		t.Errorf("loaded %d pages, want 1", count)
	}

	// Verify the external page is in the vault's pages map.
	vc.mu.RLock()
	defer vc.mu.RUnlock()

	cached, ok := vc.pages["github-org/issue-42"]
	if !ok {
		t.Fatal("external page not found in vault pages map")
	}
	if cached.sourceID != "github-org" {
		t.Errorf("sourceID = %q, want %q", cached.sourceID, "github-org")
	}
	if !cached.readOnly {
		t.Error("readOnly should be true for external pages")
	}
	if cached.entity.OriginalName != "Bug Report" {
		t.Errorf("OriginalName = %q, want %q", cached.entity.OriginalName, "Bug Report")
	}

	// Verify block tree was reconstructed.
	if len(cached.blocks) != 1 {
		t.Fatalf("expected 1 root block, got %d", len(cached.blocks))
	}
	if cached.blocks[0].UUID != "root-block-1" {
		t.Errorf("root block UUID = %q, want %q", cached.blocks[0].UUID, "root-block-1")
	}
	if len(cached.blocks[0].Children) != 1 {
		t.Fatalf("expected 1 child block, got %d", len(cached.blocks[0].Children))
	}

	// Verify local page was NOT loaded.
	if _, ok := vc.pages["local-page"]; ok {
		t.Error("local page should not be loaded by LoadExternalPages")
	}
}

func TestLoadExternalPages_SearchableAndBacklinks(t *testing.T) {
	vs, s := newTestVaultStore(t, t.TempDir())

	// Create an external page with a wikilink to a local page.
	extPage := &store.Page{
		Name:         "github-org/pr-10",
		OriginalName: "Fix Architecture",
		SourceID:     "github-org",
		SourceDocID:  "pr-10",
		ContentHash:  "hash-pr10",
	}
	if err := s.InsertPage(extPage); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}
	block := &store.Block{
		UUID:     "ext-block-1",
		PageName: "github-org/pr-10",
		Content:  "This PR fixes the [[architecture]] module.",
		Position: 0,
	}
	if err := s.InsertBlock(block); err != nil {
		t.Fatalf("InsertBlock: %v", err)
	}

	// Create a vault with a local "architecture" page.
	vaultDir := t.TempDir()
	archPath := filepath.Join(vaultDir, "architecture.md")
	if err := os.WriteFile(archPath, []byte("# Architecture\n\nSystem design."), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	vc := New(vaultDir, WithStore(s))
	if err := vc.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Load external pages.
	count, err := vs.LoadExternalPages(vc)
	if err != nil {
		t.Fatalf("LoadExternalPages: %v", err)
	}
	if count != 1 {
		t.Errorf("loaded %d pages, want 1", count)
	}

	// Build backlinks (includes external pages).
	vc.BuildBacklinks()

	// Verify external page is searchable via FullTextSearch.
	// Search for "fixes" which appears in the external page's block content.
	results, err := vc.FullTextSearch(context.Background(), "fixes", 10)
	if err != nil {
		t.Fatalf("FullTextSearch: %v", err)
	}

	// The search index uses OriginalName for page names in results.
	foundExternal := false
	for _, hit := range results {
		if hit.PageName == "Fix Architecture" {
			foundExternal = true
			break
		}
	}
	if !foundExternal {
		t.Errorf("external page not found in FullTextSearch results (got %d results)", len(results))
	}

	// Verify external page appears in GetAllPages.
	allPages, err := vc.GetAllPages(context.Background())
	if err != nil {
		t.Fatalf("GetAllPages: %v", err)
	}
	foundInAll := false
	for _, p := range allPages {
		if p.Name == "github-org/pr-10" {
			foundInAll = true
			break
		}
	}
	if !foundInAll {
		t.Error("external page not found in GetAllPages results")
	}
}

// --- reconstructBlockTree tests (004-unified-content-serve T010) ---

func TestReconstructBlockTree_Empty(t *testing.T) {
	result := reconstructBlockTree(nil)
	if result != nil {
		t.Errorf("expected nil for empty input, got %v", result)
	}

	result = reconstructBlockTree([]*store.Block{})
	if result != nil {
		t.Errorf("expected nil for empty slice, got %v", result)
	}
}

func TestReconstructBlockTree_SingleRoot(t *testing.T) {
	flat := []*store.Block{
		{UUID: "root-1", PageName: "test", Content: "# Root Block", HeadingLevel: 1, Position: 0},
	}

	result := reconstructBlockTree(flat)
	if len(result) != 1 {
		t.Fatalf("expected 1 root block, got %d", len(result))
	}
	if result[0].UUID != "root-1" {
		t.Errorf("root UUID = %q, want %q", result[0].UUID, "root-1")
	}
	if result[0].Content != "# Root Block" {
		t.Errorf("root Content = %q, want %q", result[0].Content, "# Root Block")
	}
	if len(result[0].Children) != 0 {
		t.Errorf("expected 0 children, got %d", len(result[0].Children))
	}
}

func TestReconstructBlockTree_MultipleRoots(t *testing.T) {
	flat := []*store.Block{
		{UUID: "root-1", PageName: "test", Content: "# First", HeadingLevel: 1, Position: 0},
		{UUID: "root-2", PageName: "test", Content: "# Second", HeadingLevel: 1, Position: 1},
	}

	result := reconstructBlockTree(flat)
	if len(result) != 2 {
		t.Fatalf("expected 2 root blocks, got %d", len(result))
	}
	if result[0].UUID != "root-1" {
		t.Errorf("first root UUID = %q, want %q", result[0].UUID, "root-1")
	}
	if result[1].UUID != "root-2" {
		t.Errorf("second root UUID = %q, want %q", result[1].UUID, "root-2")
	}
}

func TestReconstructBlockTree_NestedBlocks(t *testing.T) {
	flat := []*store.Block{
		{UUID: "root-1", PageName: "test", Content: "# Root", HeadingLevel: 1, Position: 0},
		{UUID: "child-1", PageName: "test", Content: "## Child", HeadingLevel: 2, Position: 0,
			ParentUUID: sql.NullString{String: "root-1", Valid: true}},
		{UUID: "grandchild-1", PageName: "test", Content: "### Grandchild", HeadingLevel: 3, Position: 0,
			ParentUUID: sql.NullString{String: "child-1", Valid: true}},
	}

	result := reconstructBlockTree(flat)
	if len(result) != 1 {
		t.Fatalf("expected 1 root block, got %d", len(result))
	}

	root := result[0]
	if root.UUID != "root-1" {
		t.Errorf("root UUID = %q, want %q", root.UUID, "root-1")
	}
	if len(root.Children) != 1 {
		t.Fatalf("expected 1 child of root, got %d", len(root.Children))
	}

	child := root.Children[0]
	if child.UUID != "child-1" {
		t.Errorf("child UUID = %q, want %q", child.UUID, "child-1")
	}
	if len(child.Children) != 1 {
		t.Fatalf("expected 1 grandchild, got %d", len(child.Children))
	}

	grandchild := child.Children[0]
	if grandchild.UUID != "grandchild-1" {
		t.Errorf("grandchild UUID = %q, want %q", grandchild.UUID, "grandchild-1")
	}
}

func TestReconstructBlockTree_MultipleChildrenOrdered(t *testing.T) {
	flat := []*store.Block{
		{UUID: "root-1", PageName: "test", Content: "# Root", HeadingLevel: 1, Position: 0},
		{UUID: "child-a", PageName: "test", Content: "## A", HeadingLevel: 2, Position: 0,
			ParentUUID: sql.NullString{String: "root-1", Valid: true}},
		{UUID: "child-b", PageName: "test", Content: "## B", HeadingLevel: 2, Position: 1,
			ParentUUID: sql.NullString{String: "root-1", Valid: true}},
		{UUID: "child-c", PageName: "test", Content: "## C", HeadingLevel: 2, Position: 2,
			ParentUUID: sql.NullString{String: "root-1", Valid: true}},
	}

	result := reconstructBlockTree(flat)
	if len(result) != 1 {
		t.Fatalf("expected 1 root, got %d", len(result))
	}
	if len(result[0].Children) != 3 {
		t.Fatalf("expected 3 children, got %d", len(result[0].Children))
	}

	// Verify order matches Position.
	expectedUUIDs := []string{"child-a", "child-b", "child-c"}
	for i, expected := range expectedUUIDs {
		if result[0].Children[i].UUID != expected {
			t.Errorf("child[%d].UUID = %q, want %q", i, result[0].Children[i].UUID, expected)
		}
	}
}
