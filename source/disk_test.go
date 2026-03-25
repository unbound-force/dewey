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
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
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

	// Verify all changes have ChangeType Added when no stored hashes.
	for i, c := range changes {
		if c.Type != ChangeAdded {
			t.Errorf("changes[%d].Type = %q, want %q", i, c.Type, ChangeAdded)
		}
	}

	// Verify each change has a non-empty document ID.
	for i, c := range changes {
		if c.ID == "" {
			t.Errorf("changes[%d].ID should not be empty", i)
		}
	}

	// Verify each added change has a non-nil Document with content.
	for i, c := range changes {
		if c.Document == nil {
			t.Errorf("changes[%d].Document should not be nil for added changes", i)
			continue
		}
		if c.Document.Content == "" {
			t.Errorf("changes[%d].Document.Content should not be empty", i)
		}
		if c.Document.ContentHash == "" {
			t.Errorf("changes[%d].Document.ContentHash should not be empty", i)
		}
		if c.Document.SourceID != "disk-local" {
			t.Errorf("changes[%d].Document.SourceID = %q, want %q", i, c.Document.SourceID, "disk-local")
		}
	}
}

func TestDiskSource_Diff_ModifiedFile(t *testing.T) {
	dir := createTestVault(t)
	ds := NewDiskSource("disk-local", "local", dir)

	// First, list to get current hashes.
	docs, err := ds.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	hashes := make(map[string]string)
	var originalHash string
	for _, doc := range docs {
		hashes[doc.ID] = doc.ContentHash
		if doc.ID == "page1.md" {
			originalHash = doc.ContentHash
		}
	}
	ds.SetStoredHashes(hashes)

	// Modify a file.
	newContent := "# Modified\nNew content."
	if err := os.WriteFile(filepath.Join(dir, "page1.md"), []byte(newContent), 0o644); err != nil {
		t.Fatalf("write modified file: %v", err)
	}

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

	// Find the modified change and verify its properties.
	var modChange *Change
	for i := range changes {
		if changes[i].Type == ChangeModified && changes[i].ID == "page1.md" {
			modChange = &changes[i]
			break
		}
	}
	if modChange == nil {
		t.Fatal("expected a modified change for page1.md")
	}

	// Verify the modified change has a Document with updated content.
	if modChange.Document == nil {
		t.Fatal("modified change Document should not be nil")
	}
	if modChange.Document.Content != newContent {
		t.Errorf("modified Document.Content = %q, want %q", modChange.Document.Content, newContent)
	}

	// Verify the content hash changed from the original.
	if modChange.Document.ContentHash == originalHash {
		t.Error("modified Document.ContentHash should differ from original")
	}
	if modChange.Document.ContentHash == "" {
		t.Error("modified Document.ContentHash should not be empty")
	}

	// Verify no other change types appeared (only modified).
	for i, c := range changes {
		if c.Type != ChangeModified {
			t.Errorf("changes[%d].Type = %q, expected only %q changes", i, c.Type, ChangeModified)
		}
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
	var deletedChange *Change
	for i, c := range changes {
		if c.Type == ChangeDeleted && c.ID == "deleted.md" {
			deletedCount++
			deletedChange = &changes[i]
		}
	}
	if deletedCount != 1 {
		t.Errorf("expected 1 deleted change for deleted.md, got %d", deletedCount)
	}

	// Verify the deleted change has the correct ChangeType.
	if deletedChange != nil {
		if deletedChange.Type != ChangeDeleted {
			t.Errorf("deleted change Type = %q, want %q", deletedChange.Type, ChangeDeleted)
		}
		// Deleted changes should have a nil Document (nothing to fetch).
		if deletedChange.Document != nil {
			t.Error("deleted change Document should be nil")
		}
		// But ID should always be set.
		if deletedChange.ID != "deleted.md" {
			t.Errorf("deleted change ID = %q, want %q", deletedChange.ID, "deleted.md")
		}
	}

	// Verify page1.md is unchanged (hash matches) — should NOT appear in changes.
	for _, c := range changes {
		if c.ID == "page1.md" && c.Type != ChangeDeleted {
			t.Errorf("page1.md should not appear as %q (hash matches stored)", c.Type)
		}
	}

	// Verify new files (page2.md, subdir/nested.md) appear as Added since they
	// are not in storedHashes.
	addedIDs := make(map[string]bool)
	for _, c := range changes {
		if c.Type == ChangeAdded {
			addedIDs[c.ID] = true
		}
	}
	if !addedIDs["page2.md"] {
		t.Error("expected page2.md to appear as Added (not in stored hashes)")
	}
	if !addedIDs["subdir/nested.md"] {
		t.Error("expected subdir/nested.md to appear as Added (not in stored hashes)")
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
