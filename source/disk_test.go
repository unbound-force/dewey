package source

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unbound-force/dewey/ignore"
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

func TestDiskSource_Diff_DocumentFields(t *testing.T) {
	dir := createTestVault(t)
	ds := NewDiskSource("disk-local", "local", dir)

	// With no stored hashes, all files are "added" — verify Document fields thoroughly.
	changes, err := ds.Diff()
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}

	if len(changes) == 0 {
		t.Fatal("expected at least 1 change, got 0")
	}

	for i, c := range changes {
		// Every change must have a non-empty ID.
		if c.ID == "" {
			t.Errorf("changes[%d].ID is empty", i)
		}

		// Type must be a valid ChangeType.
		switch c.Type {
		case ChangeAdded, ChangeModified, ChangeDeleted:
			// ok
		default:
			t.Errorf("changes[%d].Type = %q, not a valid ChangeType", i, c.Type)
		}

		// For Added changes, verify all Document fields.
		if c.Type == ChangeAdded {
			if c.Document == nil {
				t.Errorf("changes[%d].Document should not be nil for Added change", i)
				continue
			}

			// ID should match the Change.ID.
			if c.Document.ID != c.ID {
				t.Errorf("changes[%d]: Document.ID = %q, Change.ID = %q — should match",
					i, c.Document.ID, c.ID)
			}

			// Content should not be empty.
			if c.Document.Content == "" {
				t.Errorf("changes[%d].Document.Content should not be empty", i)
			}

			// ContentHash should be a SHA-256 hex digest (64 chars).
			if len(c.Document.ContentHash) != 64 {
				t.Errorf("changes[%d].Document.ContentHash length = %d, want 64 (SHA-256 hex)",
					i, len(c.Document.ContentHash))
			}

			// ContentHash should match the actual content hash.
			expectedHash := computeHash(c.Document.Content)
			if c.Document.ContentHash != expectedHash {
				t.Errorf("changes[%d].Document.ContentHash = %q, want %q (computed from Content)",
					i, c.Document.ContentHash, expectedHash)
			}

			// SourceID should be set.
			if c.Document.SourceID != "disk-local" {
				t.Errorf("changes[%d].Document.SourceID = %q, want %q",
					i, c.Document.SourceID, "disk-local")
			}

			// Title should be the filename without .md extension.
			if c.Document.Title == "" {
				t.Errorf("changes[%d].Document.Title should not be empty", i)
			}

			// FetchedAt should be recent (not zero time).
			if c.Document.FetchedAt.IsZero() {
				t.Errorf("changes[%d].Document.FetchedAt should not be zero", i)
			}
		}
	}
}

func TestDiskSource_Diff_ModifiedDocumentFields(t *testing.T) {
	dir := createTestVault(t)
	ds := NewDiskSource("disk-local", "local", dir)

	// Set up stored hashes to detect modifications.
	docs, err := ds.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	hashes := make(map[string]string)
	for _, doc := range docs {
		hashes[doc.ID] = doc.ContentHash
	}
	ds.SetStoredHashes(hashes)

	// Modify page1.md with distinct content.
	modifiedContent := "# Modified Page\n\nCompletely new content for testing."
	if err := os.WriteFile(filepath.Join(dir, "page1.md"), []byte(modifiedContent), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	changes, err := ds.Diff()
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}

	// Should have exactly 1 change (only page1.md was modified).
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}

	c := changes[0]

	// Verify the change is a modification.
	if c.Type != ChangeModified {
		t.Errorf("Type = %q, want %q", c.Type, ChangeModified)
	}
	if c.ID != "page1.md" {
		t.Errorf("ID = %q, want %q", c.ID, "page1.md")
	}

	// Verify the Document has updated content.
	if c.Document == nil {
		t.Fatal("Document should not be nil for modified change")
	}
	if c.Document.Content != modifiedContent {
		t.Errorf("Document.Content = %q, want %q", c.Document.Content, modifiedContent)
	}

	// Verify content hash matches the new content.
	expectedHash := computeHash(modifiedContent)
	if c.Document.ContentHash != expectedHash {
		t.Errorf("Document.ContentHash = %q, want %q", c.Document.ContentHash, expectedHash)
	}

	// Verify content hash differs from original.
	originalHash := hashes["page1.md"]
	if c.Document.ContentHash == originalHash {
		t.Error("Document.ContentHash should differ from original after modification")
	}

	// Verify source metadata.
	if c.Document.SourceID != "disk-local" {
		t.Errorf("Document.SourceID = %q, want %q", c.Document.SourceID, "disk-local")
	}
	if c.Document.ID != "page1.md" {
		t.Errorf("Document.ID = %q, want %q", c.Document.ID, "page1.md")
	}
}

func TestDiskSource_Diff_UnchangedFilesExcluded(t *testing.T) {
	dir := createTestVault(t)
	ds := NewDiskSource("disk-local", "local", dir)

	// Set stored hashes to match current files exactly.
	docs, err := ds.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	hashes := make(map[string]string)
	for _, doc := range docs {
		hashes[doc.ID] = doc.ContentHash
	}
	ds.SetStoredHashes(hashes)

	// No modifications — Diff should return no changes.
	changes, err := ds.Diff()
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("expected 0 changes when nothing modified, got %d", len(changes))
		for i, c := range changes {
			t.Logf("  changes[%d]: Type=%q ID=%q", i, c.Type, c.ID)
		}
	}
}

func TestWalkDiskFiles_FiltersCorrectly(t *testing.T) {
	dir := t.TempDir()

	// Create two .md files, one .txt file, and a hidden directory with an .md file.
	layout := map[string]string{
		"alpha.md":          "# Alpha\nContent A.",
		"beta.md":           "# Beta\nContent B.",
		"readme.txt":        "Not markdown — should be excluded.",
		".hidden/secret.md": "# Secret\nInside hidden dir — should be excluded.",
	}
	for name, content := range layout {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	// Build a default matcher (no .gitignore, no extra patterns) to match
	// the previous behavior of walkDiskFiles which only skipped hidden dirs.
	matcher, mErr := ignore.NewMatcher(filepath.Join(dir, ".gitignore"), nil)
	if mErr != nil {
		t.Fatalf("NewMatcher: %v", mErr)
	}

	files, err := walkDiskFiles(dir, matcher, true)
	if err != nil {
		t.Fatalf("walkDiskFiles: %v", err)
	}

	// Should contain exactly the two .md files.
	if len(files) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(files), files)
	}

	for _, expected := range []string{"alpha.md", "beta.md"} {
		hash, ok := files[expected]
		if !ok {
			t.Errorf("missing expected file %q", expected)
			continue
		}
		if len(hash) != 64 {
			t.Errorf("hash for %q has length %d, want 64 (SHA-256 hex)", expected, len(hash))
		}
	}

	// Hidden dir contents must not appear.
	if _, ok := files[".hidden/secret.md"]; ok {
		t.Error("hidden directory file .hidden/secret.md should have been skipped")
	}

	// Non-.md file must not appear.
	if _, ok := files["readme.txt"]; ok {
		t.Error("non-.md file readme.txt should have been skipped")
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

// TestDiskSource_IgnorePatterns verifies that extra ignore patterns from
// sources.yaml configuration are applied during List, excluding matched
// directories and their contents.
func TestDiskSource_IgnorePatterns(t *testing.T) {
	dir := t.TempDir()

	// Create a vault with docs/ and drafts/ directories.
	layout := map[string]string{
		"docs/guide.md": "# Guide\nUser guide content.",
		"drafts/wip.md": "# WIP\nWork in progress.",
	}
	for name, content := range layout {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	ds := NewDiskSource("disk-test", "test", dir,
		WithIgnorePatterns([]string{"drafts"}),
	)

	docs, err := ds.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	// Build a set of document IDs for easy lookup.
	docIDs := make(map[string]bool)
	for _, doc := range docs {
		docIDs[doc.ID] = true
	}

	// docs/guide.md should be included.
	if !docIDs["docs/guide.md"] {
		t.Error("expected docs/guide.md to be included, but it was not")
	}

	// drafts/wip.md should be excluded by the ignore pattern.
	if docIDs["drafts/wip.md"] {
		t.Error("expected drafts/wip.md to be excluded by ignore pattern, but it was included")
	}

	// Verify we got exactly 1 document.
	if len(docs) != 1 {
		t.Errorf("expected 1 document, got %d", len(docs))
		for _, doc := range docs {
			t.Logf("  included: %s", doc.ID)
		}
	}
}

// TestDiskSource_RecursiveFalse verifies that WithRecursive(false) limits
// List to only files in the base directory — no files from subdirectories
// are returned.
func TestDiskSource_RecursiveFalse(t *testing.T) {
	dir := t.TempDir()

	// Create a vault with root-level and nested files.
	layout := map[string]string{
		"README.md":        "# README\nRoot-level file.",
		"docs/guide.md":    "# Guide\nSubdirectory file.",
		"sub/deep/file.md": "# Deep\nNested subdirectory file.",
	}
	for name, content := range layout {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	ds := NewDiskSource("disk-test", "test", dir,
		WithRecursive(false),
	)

	docs, err := ds.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	// Build a set of document IDs for easy lookup.
	docIDs := make(map[string]bool)
	for _, doc := range docs {
		docIDs[doc.ID] = true
	}

	// Only README.md (root level) should be returned.
	if !docIDs["README.md"] {
		t.Error("expected README.md to be included (root level), but it was not")
	}

	// docs/guide.md should NOT be returned (subdirectory).
	if docIDs["docs/guide.md"] {
		t.Error("expected docs/guide.md to be excluded (subdirectory), but it was included")
	}

	// sub/deep/file.md should NOT be returned (nested subdirectory).
	if docIDs["sub/deep/file.md"] {
		t.Error("expected sub/deep/file.md to be excluded (nested subdirectory), but it was included")
	}

	// Verify we got exactly 1 document.
	if len(docs) != 1 {
		t.Errorf("expected 1 document, got %d", len(docs))
		for _, doc := range docs {
			t.Logf("  included: %s", doc.ID)
		}
	}
}

// TestDiskSource_RecursiveDefault verifies that the default behavior (no
// WithRecursive option) traverses all subdirectories and returns all .md
// files at every depth level.
func TestDiskSource_RecursiveDefault(t *testing.T) {
	dir := t.TempDir()

	// Create a vault with root-level and nested files.
	layout := map[string]string{
		"README.md":        "# README\nRoot-level file.",
		"docs/guide.md":    "# Guide\nSubdirectory file.",
		"sub/deep/file.md": "# Deep\nNested subdirectory file.",
	}
	for name, content := range layout {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	// No WithRecursive option — default behavior should be recursive=true.
	ds := NewDiskSource("disk-test", "test", dir)

	docs, err := ds.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	// Build a set of document IDs for easy lookup.
	docIDs := make(map[string]bool)
	for _, doc := range docs {
		docIDs[doc.ID] = true
	}

	// All three files should be returned.
	for _, expected := range []string{"README.md", "docs/guide.md", "sub/deep/file.md"} {
		if !docIDs[expected] {
			t.Errorf("expected %s to be included, but it was not", expected)
		}
	}

	// Verify we got exactly 3 documents.
	if len(docs) != 3 {
		t.Errorf("expected 3 documents, got %d", len(docs))
		for _, doc := range docs {
			t.Logf("  included: %s", doc.ID)
		}
	}
}

// TestDiskSource_UnionMerge verifies that .gitignore patterns and extra
// ignore patterns from sources.yaml are merged (union semantics). Both
// sets of patterns should apply simultaneously.
func TestDiskSource_UnionMerge(t *testing.T) {
	dir := t.TempDir()

	// Create a .gitignore that excludes node_modules/.
	gitignoreContent := "node_modules/\n"
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(gitignoreContent), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	// Create directories and files.
	layout := map[string]string{
		"node_modules/pkg/README.md": "# Pkg\nPackage readme.",
		"drafts/wip.md":              "# WIP\nWork in progress.",
		"docs/guide.md":              "# Guide\nUser guide content.",
	}
	for name, content := range layout {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	// Extra patterns exclude "drafts"; .gitignore excludes "node_modules/".
	// Both should apply (union merge).
	ds := NewDiskSource("disk-test", "test", dir,
		WithIgnorePatterns([]string{"drafts"}),
	)

	docs, err := ds.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	// Build a set of document IDs for easy lookup.
	docIDs := make(map[string]bool)
	for _, doc := range docs {
		docIDs[doc.ID] = true
	}

	// docs/guide.md should be included — not matched by any pattern.
	if !docIDs["docs/guide.md"] {
		t.Error("expected docs/guide.md to be included, but it was not")
	}

	// node_modules/pkg/README.md should be excluded by .gitignore pattern.
	if docIDs["node_modules/pkg/README.md"] {
		t.Error("expected node_modules/pkg/README.md to be excluded by .gitignore, but it was included")
	}

	// drafts/wip.md should be excluded by extra ignore pattern.
	if docIDs["drafts/wip.md"] {
		t.Error("expected drafts/wip.md to be excluded by extra ignore pattern, but it was included")
	}

	// Verify we got exactly 1 document.
	if len(docs) != 1 {
		t.Errorf("expected 1 document, got %d", len(docs))
		for _, doc := range docs {
			t.Logf("  included: %s", doc.ID)
		}
	}
}
