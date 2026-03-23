package source

import (
	"os"
	"path/filepath"
	"testing"
)

func createTestVault(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Create test .md files.
	files := map[string]string{
		"page1.md":          "# Page 1\nSome content here.",
		"page2.md":          "# Page 2\nMore content.",
		"subdir/nested.md":  "# Nested\nNested content.",
		".hidden/secret.md": "# Secret\nShould be skipped.",
		"not-markdown.txt":  "This is not markdown.",
	}

	for name, content := range files {
		path := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write test file %s: %v", name, err)
		}
	}

	return dir
}

func TestDiskSource_List(t *testing.T) {
	dir := createTestVault(t)
	ds := NewDiskSource("disk-local", "local", dir)

	docs, err := ds.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	// Should find page1.md, page2.md, subdir/nested.md.
	// Should NOT find .hidden/secret.md or not-markdown.txt.
	if len(docs) != 3 {
		t.Fatalf("expected 3 documents, got %d", len(docs))
	}

	// Verify content hashes are set.
	for _, doc := range docs {
		if doc.ContentHash == "" {
			t.Errorf("document %q has empty content hash", doc.ID)
		}
		if doc.SourceID != "disk-local" {
			t.Errorf("document %q source_id = %q, want %q", doc.ID, doc.SourceID, "disk-local")
		}
	}
}

func TestDiskSource_Fetch(t *testing.T) {
	dir := createTestVault(t)
	ds := NewDiskSource("disk-local", "local", dir)

	doc, err := ds.Fetch("page1.md")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if doc.Title != "page1" {
		t.Errorf("title = %q, want %q", doc.Title, "page1")
	}
	if doc.Content == "" {
		t.Error("content should not be empty")
	}
}

func TestDiskSource_Fetch_NotFound(t *testing.T) {
	dir := createTestVault(t)
	ds := NewDiskSource("disk-local", "local", dir)

	_, err := ds.Fetch("nonexistent.md")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestDiskSource_Diff_NewFiles(t *testing.T) {
	dir := createTestVault(t)
	ds := NewDiskSource("disk-local", "local", dir)

	// No stored hashes — all files should be "added".
	changes, err := ds.Diff()
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}

	addedCount := 0
	for _, c := range changes {
		if c.Type == ChangeAdded {
			addedCount++
		}
	}
	if addedCount != 3 {
		t.Errorf("expected 3 added changes, got %d", addedCount)
	}
}

func TestDiskSource_Diff_ModifiedFile(t *testing.T) {
	dir := createTestVault(t)
	ds := NewDiskSource("disk-local", "local", dir)

	// First, list to get current hashes.
	docs, _ := ds.List()
	hashes := make(map[string]string)
	for _, doc := range docs {
		hashes[doc.ID] = doc.ContentHash
	}
	ds.SetStoredHashes(hashes)

	// Modify a file.
	os.WriteFile(filepath.Join(dir, "page1.md"), []byte("# Modified\nNew content."), 0o644)

	changes, err := ds.Diff()
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}

	modifiedCount := 0
	for _, c := range changes {
		if c.Type == ChangeModified {
			modifiedCount++
		}
	}
	if modifiedCount != 1 {
		t.Errorf("expected 1 modified change, got %d", modifiedCount)
	}
}

func TestDiskSource_Diff_DeletedFile(t *testing.T) {
	dir := createTestVault(t)
	ds := NewDiskSource("disk-local", "local", dir)

	// Set stored hashes including a file that will be deleted.
	hashes := map[string]string{
		"page1.md":   computeHash("# Page 1\nSome content here."),
		"deleted.md": computeHash("# Deleted\nThis file was deleted."),
	}
	ds.SetStoredHashes(hashes)

	changes, err := ds.Diff()
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}

	deletedCount := 0
	for _, c := range changes {
		if c.Type == ChangeDeleted && c.ID == "deleted.md" {
			deletedCount++
		}
	}
	if deletedCount != 1 {
		t.Errorf("expected 1 deleted change for deleted.md, got %d", deletedCount)
	}
}

func TestDiskSource_Meta(t *testing.T) {
	ds := NewDiskSource("disk-local", "local", "/tmp/test")
	meta := ds.Meta()

	if meta.ID != "disk-local" {
		t.Errorf("id = %q, want %q", meta.ID, "disk-local")
	}
	if meta.Type != "disk" {
		t.Errorf("type = %q, want %q", meta.Type, "disk")
	}
	if meta.Status != "active" {
		t.Errorf("status = %q, want %q", meta.Status, "active")
	}
}
