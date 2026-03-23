package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/unbound-force/dewey/store"
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

	vs, _ := newTestVaultStore(t, testdata)

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
}

func TestVaultStore_IncrementalIndex_NewFile(t *testing.T) {
	// Copy testdata to temp dir so we can modify it.
	tmpDir := t.TempDir()
	copyTestdata(t, tmpDir)

	vs, _ := newTestVaultStore(t, tmpDir)

	c := New(tmpDir)
	if err := vs.FullIndex(c); err != nil {
		t.Fatalf("FullIndex: %v", err)
	}

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
}

func TestVaultStore_IncrementalIndex_ChangedFile(t *testing.T) {
	tmpDir := t.TempDir()
	copyTestdata(t, tmpDir)

	vs, _ := newTestVaultStore(t, tmpDir)

	c := New(tmpDir)
	if err := vs.FullIndex(c); err != nil {
		t.Fatalf("FullIndex: %v", err)
	}

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
}

func TestVaultStore_IncrementalIndex_DeletedFile(t *testing.T) {
	tmpDir := t.TempDir()
	copyTestdata(t, tmpDir)

	vs, _ := newTestVaultStore(t, tmpDir)

	c := New(tmpDir)
	if err := vs.FullIndex(c); err != nil {
		t.Fatalf("FullIndex: %v", err)
	}

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
