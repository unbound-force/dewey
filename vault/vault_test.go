package vault

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/unbound-force/dewey/backend"
	"github.com/unbound-force/dewey/types"
)

// Compile-time check: *Client satisfies backend.Backend.
var _ backend.Backend = (*Client)(nil)

func testVault(t *testing.T) *Client {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	testdata := filepath.Join(filepath.Dir(thisFile), "testdata")

	c := New(testdata)
	if err := c.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	c.BuildBacklinks()
	return c
}

func TestLoad(t *testing.T) {
	c := testVault(t)

	// Should have pages for: index, projects/dewey, projects/openchaos,
	// people/Hanna, daily notes/2026-01-31, daily notes/2026-02-01.
	// Plus the alias "dewey-mcp" pointing to projects/dewey.
	pages, err := c.GetAllPages(context.Background())
	if err != nil {
		t.Fatalf("GetAllPages: %v", err)
	}

	// Should NOT include .obsidian directory contents.
	for _, p := range pages {
		if strings.Contains(p.Name, ".obsidian") {
			t.Errorf("hidden directory leaked: %s", p.Name)
		}
	}

	if len(pages) < 6 {
		t.Errorf("expected at least 6 pages, got %d", len(pages))
		for _, p := range pages {
			t.Logf("  page: %s", p.Name)
		}
	}
}

func TestGetPage(t *testing.T) {
	c := testVault(t)
	ctx := context.Background()

	t.Run("exact name", func(t *testing.T) {
		page, err := c.GetPage(ctx, "projects/dewey")
		if err != nil {
			t.Fatalf("GetPage: %v", err)
		}
		if page == nil {
			t.Fatal("page not found")
		}
		if page.Properties == nil {
			t.Fatal("expected properties")
		}
		if page.Properties["type"] != "project" {
			t.Errorf("type = %v, want project", page.Properties["type"])
		}
	})

	t.Run("case insensitive", func(t *testing.T) {
		page, err := c.GetPage(ctx, "Projects/Dewey")
		if err != nil {
			t.Fatalf("GetPage: %v", err)
		}
		if page == nil {
			t.Fatal("page not found with different case")
		}
	})

	t.Run("alias lookup", func(t *testing.T) {
		page, err := c.GetPage(ctx, "dewey-mcp")
		if err != nil {
			t.Fatalf("GetPage: %v", err)
		}
		if page == nil {
			t.Fatal("alias lookup failed")
		}
		if page.Name != "projects/dewey" {
			t.Errorf("alias resolved to %q, want projects/dewey", page.Name)
		}
	})

	t.Run("nonexistent", func(t *testing.T) {
		page, err := c.GetPage(ctx, "nonexistent")
		if err != nil {
			t.Fatalf("GetPage: %v", err)
		}
		if page != nil {
			t.Error("expected nil for nonexistent page")
		}
	})
}

func TestGetPageBlocksTree(t *testing.T) {
	c := testVault(t)
	ctx := context.Background()

	blocks, err := c.GetPageBlocksTree(ctx, "projects/dewey")
	if err != nil {
		t.Fatalf("GetPageBlocksTree: %v", err)
	}
	if len(blocks) == 0 {
		t.Fatal("expected blocks")
	}

	// First block should be the H1 "# dewey".
	if !strings.Contains(blocks[0].Content, "dewey") {
		t.Errorf("first block = %q, expected to contain dewey", blocks[0].Content)
	}

	// Should have children (## Architecture, ## Features).
	if len(blocks[0].Children) < 2 {
		t.Errorf("expected at least 2 children, got %d", len(blocks[0].Children))
	}

	// All blocks should have UUIDs.
	for _, b := range blocks {
		if b.UUID == "" {
			t.Error("block has empty UUID")
		}
	}
}

func TestGetBlock(t *testing.T) {
	c := testVault(t)
	ctx := context.Background()

	// Get a page's blocks first to find a UUID.
	blocks, _ := c.GetPageBlocksTree(ctx, "projects/dewey")
	if len(blocks) == 0 {
		t.Fatal("no blocks to test")
	}

	uuid := blocks[0].UUID
	block, err := c.GetBlock(ctx, uuid)
	if err != nil {
		t.Fatalf("GetBlock: %v", err)
	}
	if block == nil {
		t.Fatal("block not found")
	}
	if block.UUID != uuid {
		t.Errorf("UUID = %q, want %q", block.UUID, uuid)
	}
	if block.Page == nil || block.Page.Name == "" {
		t.Error("expected page reference on block")
	}
}

func TestBacklinks(t *testing.T) {
	c := testVault(t)
	ctx := context.Background()

	// projects/dewey is linked from: projects/openchaos, people/Hanna, daily notes, index.
	raw, err := c.GetPageLinkedReferences(ctx, "projects/dewey")
	if err != nil {
		t.Fatalf("GetPageLinkedReferences: %v", err)
	}

	var refs []any
	if err := json.Unmarshal(raw, &refs); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(refs) < 2 {
		t.Errorf("expected at least 2 pages linking to dewey, got %d", len(refs))
	}
}

func TestJournalPages(t *testing.T) {
	c := testVault(t)
	ctx := context.Background()

	pages, _ := c.GetAllPages(ctx)
	journalCount := 0
	for _, p := range pages {
		if p.Journal {
			journalCount++
		}
	}
	if journalCount != 2 {
		t.Errorf("expected 2 journal pages, got %d", journalCount)
	}
}

func TestPing(t *testing.T) {
	c := testVault(t)
	if err := c.Ping(context.Background()); err != nil {
		t.Errorf("Ping: %v", err)
	}

	bad := New("/nonexistent/path")
	if err := bad.Ping(context.Background()); err == nil {
		t.Error("expected error for nonexistent path")
	}
}

// testWritableVault copies testdata to a temp dir and returns a writable vault.
func testWritableVault(t *testing.T) *Client {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	testdata := filepath.Join(filepath.Dir(thisFile), "testdata")

	tmpDir := t.TempDir()
	// Copy testdata to temp.
	if err := filepath.WalkDir(testdata, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(testdata, path)
		target := filepath.Join(tmpDir, rel)
		if d.IsDir() {
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

	c := New(tmpDir)
	if err := c.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	c.BuildBacklinks()
	return c
}

func TestCreatePage(t *testing.T) {
	c := testWritableVault(t)
	ctx := context.Background()

	page, err := c.CreatePage(ctx, "test-new-page", map[string]any{"type": "test"}, nil)
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}
	if page == nil {
		t.Fatal("CreatePage returned nil page")
	}
	if page.Name != "test-new-page" {
		t.Errorf("Name = %q, want %q", page.Name, "test-new-page")
	}

	// Should be retrievable.
	got, err := c.GetPage(ctx, "test-new-page")
	if err != nil || got == nil {
		t.Fatalf("GetPage after create: %v, got=%v", err, got)
	}

	// Duplicate should fail.
	_, err = c.CreatePage(ctx, "test-new-page", nil, nil)
	if err == nil {
		t.Error("expected error for duplicate page")
	}
}

func TestCreatePageSubdirectory(t *testing.T) {
	c := testWritableVault(t)
	ctx := context.Background()

	page, err := c.CreatePage(ctx, "deep/nested/page", nil, nil)
	if err != nil {
		t.Fatalf("CreatePage with subdirectory: %v", err)
	}
	if page == nil {
		t.Fatal("CreatePage returned nil")
	}

	got, err := c.GetPage(ctx, "deep/nested/page")
	if err != nil || got == nil {
		t.Fatalf("GetPage after create: %v", err)
	}
}

func TestAppendBlockInPage(t *testing.T) {
	c := testWritableVault(t)
	ctx := context.Background()

	// Append to existing page.
	block, err := c.AppendBlockInPage(ctx, "index", "New appended content")
	if err != nil {
		t.Fatalf("AppendBlockInPage: %v", err)
	}
	if block == nil {
		t.Fatal("returned nil block")
	}

	// Verify the content is in the page now.
	blocks, _ := c.GetPageBlocksTree(ctx, "index")
	found := false
	var walk func([]types.BlockEntity)
	walk = func(bs []types.BlockEntity) {
		for _, b := range bs {
			if strings.Contains(b.Content, "New appended content") {
				found = true
			}
			walk(b.Children)
		}
	}
	walk(blocks)
	if !found {
		t.Error("appended content not found in page blocks")
	}
}

func TestAppendBlockAutoCreate(t *testing.T) {
	c := testWritableVault(t)
	ctx := context.Background()

	// Append to non-existent page should auto-create it.
	block, err := c.AppendBlockInPage(ctx, "auto-created", "First block")
	if err != nil {
		t.Fatalf("AppendBlockInPage auto-create: %v", err)
	}
	if block == nil {
		t.Fatal("returned nil block")
	}

	page, _ := c.GetPage(ctx, "auto-created")
	if page == nil {
		t.Error("auto-created page not found")
	}
}

func TestUpdateBlock(t *testing.T) {
	c := testWritableVault(t)
	ctx := context.Background()

	// Get a block UUID from an existing page.
	blocks, _ := c.GetPageBlocksTree(ctx, "index")
	if len(blocks) == 0 {
		t.Fatal("no blocks in index page")
	}
	uuid := blocks[0].UUID
	oldContent := blocks[0].Content

	err := c.UpdateBlock(ctx, uuid, "Updated content here")
	if err != nil {
		t.Fatalf("UpdateBlock: %v", err)
	}

	// Re-read and verify.
	blocks, _ = c.GetPageBlocksTree(ctx, "index")
	found := false
	walk := func(bs []types.BlockEntity) {
		for _, b := range bs {
			if strings.Contains(b.Content, "Updated content here") {
				found = true
			}
			if b.Content == oldContent {
				t.Error("old content still present after update")
			}
		}
	}
	walk(blocks)
	if !found {
		t.Error("updated content not found")
	}
}

func TestRemoveBlock(t *testing.T) {
	c := testWritableVault(t)
	ctx := context.Background()

	blocks, _ := c.GetPageBlocksTree(ctx, "index")
	if len(blocks) == 0 {
		t.Fatal("no blocks in index page")
	}
	uuid := blocks[0].UUID
	oldContent := blocks[0].Content

	err := c.RemoveBlock(ctx, uuid)
	if err != nil {
		t.Fatalf("RemoveBlock: %v", err)
	}

	// Verify content is gone.
	absPath := filepath.Join(c.vaultPath, c.pages["index"].filePath)
	data, _ := os.ReadFile(absPath)
	if strings.Contains(string(data), oldContent) {
		t.Error("removed block content still in file")
	}
}

func TestDeletePage(t *testing.T) {
	c := testWritableVault(t)
	ctx := context.Background()

	// Verify page exists.
	page, _ := c.GetPage(ctx, "index")
	if page == nil {
		t.Fatal("index page should exist")
	}

	err := c.DeletePage(ctx, "index")
	if err != nil {
		t.Fatalf("DeletePage: %v", err)
	}

	// Should be gone.
	page, _ = c.GetPage(ctx, "index")
	if page != nil {
		t.Error("deleted page still found")
	}

	// Delete non-existent should fail.
	err = c.DeletePage(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for non-existent page")
	}
}

func TestRenamePage(t *testing.T) {
	c := testWritableVault(t)
	ctx := context.Background()

	err := c.RenamePage(ctx, "index", "renamed-index")
	if err != nil {
		t.Fatalf("RenamePage: %v", err)
	}

	// Old name should be gone.
	old, _ := c.GetPage(ctx, "index")
	if old != nil {
		t.Error("old page name still found")
	}

	// New name should exist.
	new_, _ := c.GetPage(ctx, "renamed-index")
	if new_ == nil {
		t.Error("renamed page not found")
	}

	// Rename to existing name should fail.
	err = c.RenamePage(ctx, "renamed-index", "projects/dewey")
	if err == nil {
		t.Error("expected error when renaming to existing page")
	}
}

func TestRenamePageUpdatesLinks(t *testing.T) {
	c := testWritableVault(t)
	ctx := context.Background()

	// Create a page that links to another.
	_, _ = c.CreatePage(ctx, "linker", nil, nil)
	_, _ = c.AppendBlockInPage(ctx, "linker", "See [[projects/dewey]] for details")

	// Rename the target.
	err := c.RenamePage(ctx, "projects/dewey", "tools/dewey")
	if err != nil {
		t.Fatalf("RenamePage: %v", err)
	}

	// The linker page should now reference the new name.
	blocks, _ := c.GetPageBlocksTree(ctx, "linker")
	found := false
	for _, b := range blocks {
		if strings.Contains(b.Content, "[[tools/dewey]]") {
			found = true
		}
		if strings.Contains(b.Content, "[[projects/dewey]]") {
			t.Error("old link still present after rename")
		}
	}
	if !found {
		t.Error("updated link not found in linker page")
	}
}

func TestPrependBlockInPage(t *testing.T) {
	c := testWritableVault(t)
	ctx := context.Background()

	block, err := c.PrependBlockInPage(ctx, "index", "Prepended content")
	if err != nil {
		t.Fatalf("PrependBlockInPage: %v", err)
	}
	if block == nil {
		t.Fatal("returned nil block")
	}

	// First block should be the prepended content.
	blocks, _ := c.GetPageBlocksTree(ctx, "index")
	if len(blocks) == 0 {
		t.Fatal("no blocks after prepend")
	}
	if !strings.Contains(blocks[0].Content, "Prepended content") {
		t.Errorf("first block = %q, expected prepended content", blocks[0].Content)
	}
}

func TestDatascriptQueryNotSupported(t *testing.T) {
	c := testVault(t)
	_, err := c.DatascriptQuery(context.Background(), "[:find ...]")
	if err != ErrNotSupported {
		t.Errorf("DatascriptQuery: %v, want ErrNotSupported", err)
	}
}

func TestFindBlocksByTag(t *testing.T) {
	c := testVault(t)
	ctx := context.Background()

	results, err := c.FindBlocksByTag(ctx, "decision", false)
	if err != nil {
		t.Fatalf("FindBlocksByTag: %v", err)
	}

	// The daily notes/2026-01-31 page has a block with #decision.
	found := false
	for _, r := range results {
		if strings.Contains(r.Page, "2026-01-31") {
			found = true
		}
	}
	if !found {
		t.Error("expected to find #decision in daily notes/2026-01-31")
	}
}

func TestFindByProperty(t *testing.T) {
	c := testVault(t)
	ctx := context.Background()

	results, err := c.FindByProperty(ctx, "type", "project", "eq")
	if err != nil {
		t.Fatalf("FindByProperty: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 project pages, got %d", len(results))
		for _, r := range results {
			t.Logf("  %s", r.Name)
		}
	}
}

func TestSearchJournals(t *testing.T) {
	c := testVault(t)
	ctx := context.Background()

	results, err := c.SearchJournals(ctx, "backend", "", "")
	if err != nil {
		t.Fatalf("SearchJournals: %v", err)
	}

	found := false
	for _, r := range results {
		if strings.Contains(r.Date, "2026-02-01") {
			found = true
		}
	}
	if !found {
		t.Error("expected to find 'backend' in 2026-02-01 journal")
	}
}

func TestGraphBuildFromVault(t *testing.T) {
	c := testVault(t)
	ctx := context.Background()

	// Test that graph.Build works with the vault client.
	// This is the key integration test: the vault client satisfies the
	// interface that graph.Build needs.
	pages, err := c.GetAllPages(ctx)
	if err != nil {
		t.Fatalf("GetAllPages: %v", err)
	}

	// Verify we can get blocks for every page (graph.Build does this).
	for _, p := range pages {
		blocks, err := c.GetPageBlocksTree(ctx, p.Name)
		if err != nil {
			t.Errorf("GetPageBlocksTree(%s): %v", p.Name, err)
		}
		// blocks can be nil for empty pages, that's fine.
		_ = blocks
	}
}

func TestReload(t *testing.T) {
	c := testWritableVault(t)
	ctx := context.Background()

	// Get initial page count.
	pages, err := c.GetAllPages(ctx)
	if err != nil {
		t.Fatalf("GetAllPages: %v", err)
	}
	initialCount := len(pages)

	// Create a new page directly on disk (simulating external change).
	newPagePath := filepath.Join(c.vaultPath, "external-change.md")
	if err := os.WriteFile(newPagePath, []byte("# External Change\n\nThis was added externally."), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Reload the vault.
	if err := c.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	// Should now see the new page.
	pages, err = c.GetAllPages(ctx)
	if err != nil {
		t.Fatalf("GetAllPages after reload: %v", err)
	}

	if len(pages) != initialCount+1 {
		t.Errorf("expected %d pages after reload, got %d", initialCount+1, len(pages))
	}

	// Verify the new page is retrievable.
	page, err := c.GetPage(ctx, "external-change")
	if err != nil || page == nil {
		t.Fatalf("new page not found after reload: err=%v, page=%v", err, page)
	}
}

func TestWatchFileCreate(t *testing.T) {
	c := testWritableVault(t)
	ctx := context.Background()

	// Start watching.
	if err := c.Watch(); err != nil {
		t.Fatalf("Watch: %v", err)
	}
	defer func() { _ = c.Close() }()

	// Create a new file.
	newPagePath := filepath.Join(c.vaultPath, "watched-create.md")
	if err := os.WriteFile(newPagePath, []byte("# Watched Create\n\nCreated while watching."), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Give the watcher time to process.
	time.Sleep(100 * time.Millisecond)

	// Should be indexed.
	page, err := c.GetPage(ctx, "watched-create")
	if err != nil {
		t.Fatalf("GetPage after create: %v", err)
	}
	if page == nil {
		t.Fatal("created page not found after watch event")
	}
}

func TestWatchFileModify(t *testing.T) {
	c := testWritableVault(t)
	ctx := context.Background()

	// Create a file first.
	testPagePath := filepath.Join(c.vaultPath, "watched-modify.md")
	if err := os.WriteFile(testPagePath, []byte("# Original Content"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Reload to index it.
	if err := c.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	// Start watching.
	if err := c.Watch(); err != nil {
		t.Fatalf("Watch: %v", err)
	}
	defer func() { _ = c.Close() }()

	// Modify the file.
	if err := os.WriteFile(testPagePath, []byte("# Modified Content\n\nThis was changed."), 0o644); err != nil {
		t.Fatalf("WriteFile modify: %v", err)
	}

	// Give the watcher time to process.
	time.Sleep(100 * time.Millisecond)

	// Should have updated content.
	blocks, err := c.GetPageBlocksTree(ctx, "watched-modify")
	if err != nil {
		t.Fatalf("GetPageBlocksTree: %v", err)
	}
	if len(blocks) == 0 {
		t.Fatal("no blocks after modify")
	}
	if !strings.Contains(blocks[0].Content, "Modified Content") {
		t.Errorf("expected modified content, got: %s", blocks[0].Content)
	}
}

func TestWatchFileDelete(t *testing.T) {
	c := testWritableVault(t)
	ctx := context.Background()

	// Create a file first.
	testPagePath := filepath.Join(c.vaultPath, "watched-delete.md")
	if err := os.WriteFile(testPagePath, []byte("# To Be Deleted"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Reload to index it.
	if err := c.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	// Verify it exists.
	page, _ := c.GetPage(ctx, "watched-delete")
	if page == nil {
		t.Fatal("page should exist before delete")
	}

	// Start watching.
	if err := c.Watch(); err != nil {
		t.Fatalf("Watch: %v", err)
	}
	defer func() { _ = c.Close() }()

	// Delete the file.
	if err := os.Remove(testPagePath); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Give the watcher time to process.
	time.Sleep(100 * time.Millisecond)

	// Should be gone from index.
	page, err := c.GetPage(ctx, "watched-delete")
	if err != nil {
		t.Fatalf("GetPage after delete: %v", err)
	}
	if page != nil {
		t.Error("deleted page still found in index")
	}
}

func TestConcurrentAccess(t *testing.T) {
	c := testWritableVault(t)
	ctx := context.Background()

	// Start watching.
	if err := c.Watch(); err != nil {
		t.Fatalf("Watch: %v", err)
	}
	defer func() { _ = c.Close() }()

	// Spawn multiple goroutines that read and write concurrently.
	const numReaders = 5
	const numWriters = 3

	done := make(chan bool)

	// Readers: continuously read pages.
	for i := 0; i < numReaders; i++ {
		go func(id int) {
			for j := 0; j < 10; j++ {
				_, _ = c.GetAllPages(ctx)
				_, _ = c.GetPage(ctx, "index")
				time.Sleep(10 * time.Millisecond)
			}
			done <- true
		}(i)
	}

	// Writers: create new files.
	for i := 0; i < numWriters; i++ {
		go func(id int) {
			for j := 0; j < 5; j++ {
				pageName := fmt.Sprintf("concurrent-%d-%d.md", id, j)
				pagePath := filepath.Join(c.vaultPath, pageName)
				_ = os.WriteFile(pagePath, []byte(fmt.Sprintf("# Page %d-%d", id, j)), 0o644)
				time.Sleep(20 * time.Millisecond)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to finish.
	for i := 0; i < numReaders+numWriters; i++ {
		<-done
	}

	// Give watcher time to catch up.
	time.Sleep(200 * time.Millisecond)

	// Verify no race conditions or crashes occurred.
	pages, err := c.GetAllPages(ctx)
	if err != nil {
		t.Fatalf("GetAllPages after concurrent access: %v", err)
	}
	if len(pages) == 0 {
		t.Error("expected some pages after concurrent access")
	}
}

// --- UUID Persistence Tests ---

func TestEmbeddedUUIDParsing(t *testing.T) {
	c := testWritableVault(t)
	ctx := context.Background()

	// Create a file with embedded UUIDs
	testUUID1 := "12345678-1234-1234-1234-123456789abc"
	testUUID2 := "87654321-4321-4321-4321-cba987654321"

	content := "---\ntype: test\n---\n\n" +
		"# Heading 1 <!-- id: " + testUUID1 + " -->\n\n" +
		"Some content\n\n" +
		"## Heading 2 <!-- id: " + testUUID2 + " -->\n\n" +
		"More content"

	_, err := c.CreatePage(ctx, "uuid-test", map[string]any{"type": "test"}, nil)
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}

	// Write the file with embedded UUIDs directly
	absPath := filepath.Join(c.vaultPath, "uuid-test.md")
	if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Reload the vault
	c = New(c.vaultPath)
	if err := c.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	c.BuildBacklinks()

	// Check that the UUIDs are preserved
	blocks, err := c.GetPageBlocksTree(ctx, "uuid-test")
	if err != nil {
		t.Fatalf("GetPageBlocksTree: %v", err)
	}

	if len(blocks) == 0 {
		t.Fatal("expected blocks")
	}

	// First block should have testUUID1
	if blocks[0].UUID != testUUID1 {
		t.Errorf("block 0 UUID = %q, want %q", blocks[0].UUID, testUUID1)
	}

	// Content should not contain the UUID comment
	if strings.Contains(blocks[0].Content, "<!-- id:") {
		t.Errorf("block content should not contain UUID comment, got: %q", blocks[0].Content)
	}

	// Second block (child) should have testUUID2
	if len(blocks[0].Children) == 0 {
		t.Fatal("expected child blocks")
	}
	if blocks[0].Children[0].UUID != testUUID2 {
		t.Errorf("block 0 child 0 UUID = %q, want %q", blocks[0].Children[0].UUID, testUUID2)
	}
}

func TestFileWithoutEmbeddedUUIDs(t *testing.T) {
	c := testWritableVault(t)
	ctx := context.Background()

	// Create a file without embedded UUIDs (old format)
	content := "---\ntype: test\n---\n\n# Old Format\n\nNo UUIDs here"

	_, err := c.CreatePage(ctx, "old-format", map[string]any{"type": "test"}, nil)
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}

	absPath := filepath.Join(c.vaultPath, "old-format.md")
	if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Reload
	c = New(c.vaultPath)
	if err := c.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	c.BuildBacklinks()

	// Should get deterministic UUIDs
	blocks, err := c.GetPageBlocksTree(ctx, "old-format")
	if err != nil {
		t.Fatalf("GetPageBlocksTree: %v", err)
	}

	if len(blocks) == 0 {
		t.Fatal("expected blocks")
	}

	uuid1 := blocks[0].UUID
	if uuid1 == "" {
		t.Error("expected non-empty UUID")
	}

	// Re-parse same file, should get same UUIDs (deterministic)
	c = New(c.vaultPath)
	if err := c.Load(); err != nil {
		t.Fatalf("Load second time: %v", err)
	}
	c.BuildBacklinks()

	blocks2, _ := c.GetPageBlocksTree(ctx, "old-format")
	if len(blocks2) == 0 {
		t.Fatal("expected blocks on second parse")
	}

	if blocks2[0].UUID != uuid1 {
		t.Errorf("UUID changed between parses: %q != %q", blocks2[0].UUID, uuid1)
	}
}

func TestWriteThenReparse_UUIDsStable(t *testing.T) {
	c := testWritableVault(t)
	ctx := context.Background()

	// Create page and append blocks
	_, err := c.CreatePage(ctx, "stability-test", nil, nil)
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}

	block1, err := c.AppendBlockInPage(ctx, "stability-test", "# First Heading")
	if err != nil {
		t.Fatalf("AppendBlockInPage 1: %v", err)
	}
	uuid1 := block1.UUID

	block2, err := c.AppendBlockInPage(ctx, "stability-test", "## Second Heading")
	if err != nil {
		t.Fatalf("AppendBlockInPage 2: %v", err)
	}
	uuid2 := block2.UUID

	// Re-parse the vault
	c = New(c.vaultPath)
	if err := c.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	c.BuildBacklinks()

	// UUIDs should be stable
	blocks, err := c.GetPageBlocksTree(ctx, "stability-test")
	if err != nil {
		t.Fatalf("GetPageBlocksTree: %v", err)
	}

	if len(blocks) != 1 {
		t.Fatalf("expected 1 root block, got %d", len(blocks))
	}

	if blocks[0].UUID != uuid1 {
		t.Errorf("block 0 UUID changed: %q != %q", blocks[0].UUID, uuid1)
	}

	if len(blocks[0].Children) != 1 {
		t.Fatalf("expected 1 child block, got %d", len(blocks[0].Children))
	}

	if blocks[0].Children[0].UUID != uuid2 {
		t.Errorf("child block UUID changed: %q != %q", blocks[0].Children[0].UUID, uuid2)
	}

	// Verify UUIDs are embedded in the file
	absPath := filepath.Join(c.vaultPath, "stability-test.md")
	fileContent, err := os.ReadFile(absPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	fileStr := string(fileContent)
	if !strings.Contains(fileStr, "<!-- id: "+uuid1+" -->") {
		t.Error("file should contain UUID comment for first block")
	}
	if !strings.Contains(fileStr, "<!-- id: "+uuid2+" -->") {
		t.Error("file should contain UUID comment for second block")
	}
}

func TestUpdateBlockPreservesUUID(t *testing.T) {
	c := testWritableVault(t)
	ctx := context.Background()

	_, err := c.CreatePage(ctx, "update-test", nil, nil)
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}

	block, err := c.AppendBlockInPage(ctx, "update-test", "# Original Content")
	if err != nil {
		t.Fatalf("AppendBlockInPage: %v", err)
	}
	originalUUID := block.UUID

	// Update the block
	err = c.UpdateBlock(ctx, originalUUID, "# Updated Content")
	if err != nil {
		t.Fatalf("UpdateBlock: %v", err)
	}

	// Re-parse and verify UUID is preserved
	c = New(c.vaultPath)
	if err := c.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	c.BuildBacklinks()

	blocks, _ := c.GetPageBlocksTree(ctx, "update-test")
	if len(blocks) == 0 {
		t.Fatal("expected blocks after update")
	}

	if blocks[0].UUID != originalUUID {
		t.Errorf("UUID changed after update: %q != %q", blocks[0].UUID, originalUUID)
	}

	if !strings.Contains(blocks[0].Content, "Updated Content") {
		t.Errorf("content not updated: %q", blocks[0].Content)
	}
}

func TestPrependBlockEmbedsUUID(t *testing.T) {
	c := testWritableVault(t)
	ctx := context.Background()

	_, err := c.CreatePage(ctx, "prepend-test", nil, nil)
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}

	// Append a block first
	_, err = c.AppendBlockInPage(ctx, "prepend-test", "# Second Block")
	if err != nil {
		t.Fatalf("AppendBlockInPage: %v", err)
	}

	// Prepend a block
	block, err := c.PrependBlockInPage(ctx, "prepend-test", "# First Block")
	if err != nil {
		t.Fatalf("PrependBlockInPage: %v", err)
	}
	prependedUUID := block.UUID

	// Re-parse
	c = New(c.vaultPath)
	if err := c.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	c.BuildBacklinks()

	blocks, _ := c.GetPageBlocksTree(ctx, "prepend-test")
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}

	// First block should have the prepended UUID
	if blocks[0].UUID != prependedUUID {
		t.Errorf("first block UUID = %q, want %q", blocks[0].UUID, prependedUUID)
	}

	// Verify file contains UUID
	absPath := filepath.Join(c.vaultPath, "prepend-test.md")
	fileContent, _ := os.ReadFile(absPath)
	if !strings.Contains(string(fileContent), "<!-- id: "+prependedUUID+" -->") {
		t.Error("file should contain UUID comment for prepended block")
	}
}

func TestInsertBlockEmbedsUUID(t *testing.T) {
	c := testWritableVault(t)
	ctx := context.Background()

	_, err := c.CreatePage(ctx, "insert-test", nil, nil)
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}

	parent, err := c.AppendBlockInPage(ctx, "insert-test", "# Parent")
	if err != nil {
		t.Fatalf("AppendBlockInPage: %v", err)
	}

	child, err := c.InsertBlock(ctx, parent.UUID, "Child content", nil)
	if err != nil {
		t.Fatalf("InsertBlock: %v", err)
	}
	childUUID := child.UUID

	// Re-parse
	c = New(c.vaultPath)
	if err := c.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	c.BuildBacklinks()

	blocks, _ := c.GetPageBlocksTree(ctx, "insert-test")
	if len(blocks) == 0 || len(blocks[0].Children) == 0 {
		t.Fatal("expected parent with child")
	}

	if blocks[0].Children[0].UUID != childUUID {
		t.Errorf("child UUID = %q, want %q", blocks[0].Children[0].UUID, childUUID)
	}
}

func TestSafePath_Traversal(t *testing.T) {
	dir := t.TempDir()
	vc := New(dir)

	// Normal path should work.
	_, err := vc.safePath("some/page.md")
	if err != nil {
		t.Errorf("normal path should be safe: %v", err)
	}

	// Path traversal should fail.
	_, err = vc.safePath("../../etc/passwd")
	if err == nil {
		t.Error("path traversal should be rejected")
	}

	// Another traversal attempt.
	_, err = vc.safePath("../outside.md")
	if err == nil {
		t.Error("path traversal via .. should be rejected")
	}
}

// --- Tests for decomposed MoveBlock helpers ---

func TestParseMoveOptions_NilOpts(t *testing.T) {
	got := parseMoveOptions(nil)
	if got {
		t.Error("parseMoveOptions(nil) = true, want false")
	}
}

func TestParseMoveOptions_EmptyMap(t *testing.T) {
	got := parseMoveOptions(map[string]any{})
	if got {
		t.Error("parseMoveOptions({}) = true, want false")
	}
}

func TestParseMoveOptions_BeforeTrue(t *testing.T) {
	got := parseMoveOptions(map[string]any{"before": true})
	if !got {
		t.Error("parseMoveOptions({before: true}) = false, want true")
	}
}

func TestParseMoveOptions_BeforeFalse(t *testing.T) {
	got := parseMoveOptions(map[string]any{"before": false})
	if got {
		t.Error("parseMoveOptions({before: false}) = true, want false")
	}
}

func TestParseMoveOptions_BeforeNonBool(t *testing.T) {
	tests := []struct {
		name string
		val  any
	}{
		{"string", "true"},
		{"int", 1},
		{"nil", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseMoveOptions(map[string]any{"before": tc.val})
			if got {
				t.Errorf("parseMoveOptions({before: %v}) = true, want false (non-bool type assertion yields zero value)", tc.val)
			}
		})
	}
}

func TestParseMoveOptions_OtherKeys(t *testing.T) {
	got := parseMoveOptions(map[string]any{"after": true, "something": "else"})
	if got {
		t.Error("parseMoveOptions without 'before' key = true, want false")
	}
}

func TestInsertContentRelative_InsertAfter(t *testing.T) {
	fileStr := "line1\nline2\nline3\n"
	got := insertContentRelative(fileStr, "inserted", "line2", false)
	want := "line1\nline2\ninserted\nline3\n"
	if got != want {
		t.Errorf("insertContentRelative(after) =\n%q\nwant\n%q", got, want)
	}
}

func TestInsertContentRelative_InsertBefore(t *testing.T) {
	fileStr := "line1\nline2\nline3\n"
	got := insertContentRelative(fileStr, "inserted", "line2", true)
	want := "line1\ninserted\nline2\nline3\n"
	if got != want {
		t.Errorf("insertContentRelative(before) =\n%q\nwant\n%q", got, want)
	}
}

func TestInsertContentRelative_TargetNotFound(t *testing.T) {
	fileStr := "line1\nline2\nline3\n"
	got := insertContentRelative(fileStr, "inserted", "nonexistent", false)
	// strings.Replace with missing target returns the original string unchanged.
	if got != fileStr {
		t.Errorf("insertContentRelative(target not found) modified the string:\ngot:  %q\nwant: %q", got, fileStr)
	}
}

func TestInsertContentRelative_FirstOccurrence(t *testing.T) {
	fileStr := "target\nother\ntarget\n"
	got := insertContentRelative(fileStr, "inserted", "target", false)
	// Only the first occurrence should be replaced.
	want := "target\ninserted\nother\ntarget\n"
	if got != want {
		t.Errorf("insertContentRelative(first occurrence) =\n%q\nwant\n%q", got, want)
	}
}

func TestInsertContentRelative_MultilineContent(t *testing.T) {
	fileStr := "# Heading\n\nSome paragraph\n\n## Subheading\n"
	srcContent := "Inserted paragraph"
	tgtContent := "Some paragraph"

	t.Run("after", func(t *testing.T) {
		got := insertContentRelative(fileStr, srcContent, tgtContent, false)
		want := "# Heading\n\nSome paragraph\nInserted paragraph\n\n## Subheading\n"
		if got != want {
			t.Errorf("after =\n%q\nwant\n%q", got, want)
		}
	})

	t.Run("before", func(t *testing.T) {
		got := insertContentRelative(fileStr, srcContent, tgtContent, true)
		want := "# Heading\n\nInserted paragraph\nSome paragraph\n\n## Subheading\n"
		if got != want {
			t.Errorf("before =\n%q\nwant\n%q", got, want)
		}
	})
}

func TestInsertContentRelative_EmptyFileStr(t *testing.T) {
	got := insertContentRelative("", "inserted", "target", false)
	// Empty string has no target to match, so returns unchanged.
	if got != "" {
		t.Errorf("insertContentRelative(empty) = %q, want %q", got, "")
	}
}

// headingLineOrder returns the order of heading lines in fileStr,
// filtered to those containing any of the given substrings.
// Returns a slice of the matched substrings in the order they appear.
func headingLineOrder(fileStr string, markers []string) []string {
	var order []string
	for _, line := range strings.Split(fileStr, "\n") {
		if !strings.HasPrefix(line, "#") {
			continue
		}
		for _, m := range markers {
			if strings.Contains(line, m) {
				order = append(order, m)
				break
			}
		}
	}
	return order
}

func TestMoveBlockSamePage_Direct(t *testing.T) {
	// Test moveBlockSamePage directly with a manually constructed vault
	// to avoid UUID embedding complications from AppendBlockInPage.
	dir := t.TempDir()
	c := New(dir)

	// Create file with known content (no embedded UUIDs).
	content := "# Block A\n# Block B\n# Block C\n"
	filePath := filepath.Join(dir, "movepage.md")
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := c.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	c.BuildBacklinks()

	// Acquire lock and call moveBlockSamePage to move A after C.
	c.mu.Lock()
	err := c.moveBlockSamePage("movepage", "# Block A", "# Block C", false)
	c.mu.Unlock()
	if err != nil {
		t.Fatalf("moveBlockSamePage(A after C): %v", err)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	order := headingLineOrder(string(data), []string{"Block A", "Block B", "Block C"})
	wantOrder := []string{"Block B", "Block C", "Block A"}
	if fmt.Sprintf("%v", order) != fmt.Sprintf("%v", wantOrder) {
		t.Errorf("after move A after C: heading order = %v, want %v\nfile:\n%s", order, wantOrder, string(data))
	}

	// Move A before B (restore original order).
	c.mu.Lock()
	err = c.moveBlockSamePage("movepage", "# Block A", "# Block B", true)
	c.mu.Unlock()
	if err != nil {
		t.Fatalf("moveBlockSamePage(A before B): %v", err)
	}

	data, err = os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	order = headingLineOrder(string(data), []string{"Block A", "Block B", "Block C"})
	wantOrder = []string{"Block A", "Block B", "Block C"}
	if fmt.Sprintf("%v", order) != fmt.Sprintf("%v", wantOrder) {
		t.Errorf("after move A before B: heading order = %v, want %v\nfile:\n%s", order, wantOrder, string(data))
	}
}

func TestMoveBlockSamePage_PageNotFound(t *testing.T) {
	dir := t.TempDir()
	c := New(dir)

	c.mu.Lock()
	err := c.moveBlockSamePage("nonexistent", "src", "tgt", false)
	c.mu.Unlock()
	if err == nil {
		t.Error("moveBlockSamePage with nonexistent page should return error")
	}
	if !strings.Contains(err.Error(), "page not found") {
		t.Errorf("error = %q, want to contain 'page not found'", err.Error())
	}
}

func TestMoveBlockCrossPage_Direct(t *testing.T) {
	dir := t.TempDir()
	c := New(dir)

	// Create source file with two blocks.
	srcContent := "# Source Block\n# Remaining Block\n"
	srcPath := filepath.Join(dir, "move-src.md")
	if err := os.WriteFile(srcPath, []byte(srcContent), 0o644); err != nil {
		t.Fatalf("WriteFile src: %v", err)
	}

	// Create target file with one block.
	tgtContent := "# Target Block\n"
	tgtPath := filepath.Join(dir, "move-tgt.md")
	if err := os.WriteFile(tgtPath, []byte(tgtContent), 0o644); err != nil {
		t.Fatalf("WriteFile tgt: %v", err)
	}

	if err := c.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	c.BuildBacklinks()

	// Move Source Block after Target Block.
	c.mu.Lock()
	err := c.moveBlockCrossPage("move-src", "move-tgt", "# Source Block", "# Target Block", false)
	c.mu.Unlock()
	if err != nil {
		t.Fatalf("moveBlockCrossPage(after): %v", err)
	}

	// Source page should no longer contain "Source Block".
	srcData, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("ReadFile src: %v", err)
	}
	if strings.Contains(string(srcData), "Source Block") {
		t.Error("source block content still in source file after cross-page move")
	}
	if !strings.Contains(string(srcData), "Remaining Block") {
		t.Error("remaining block content missing from source file")
	}

	// Target page should contain both blocks in order: Target, Source.
	tgtData, err := os.ReadFile(tgtPath)
	if err != nil {
		t.Fatalf("ReadFile tgt: %v", err)
	}
	order := headingLineOrder(string(tgtData), []string{"Target Block", "Source Block"})
	wantOrder := []string{"Target Block", "Source Block"}
	if fmt.Sprintf("%v", order) != fmt.Sprintf("%v", wantOrder) {
		t.Errorf("cross-page move after: heading order = %v, want %v\nfile:\n%s", order, wantOrder, string(tgtData))
	}
}

func TestMoveBlockCrossPage_Before(t *testing.T) {
	dir := t.TempDir()
	c := New(dir)

	srcPath := filepath.Join(dir, "xmove-src.md")
	if err := os.WriteFile(srcPath, []byte("# Moved Block\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	tgtPath := filepath.Join(dir, "xmove-tgt.md")
	if err := os.WriteFile(tgtPath, []byte("# Target Block\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := c.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	c.BuildBacklinks()

	c.mu.Lock()
	err := c.moveBlockCrossPage("xmove-src", "xmove-tgt", "# Moved Block", "# Target Block", true)
	c.mu.Unlock()
	if err != nil {
		t.Fatalf("moveBlockCrossPage(before): %v", err)
	}

	tgtData, err := os.ReadFile(tgtPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	order := headingLineOrder(string(tgtData), []string{"Moved Block", "Target Block"})
	wantOrder := []string{"Moved Block", "Target Block"}
	if fmt.Sprintf("%v", order) != fmt.Sprintf("%v", wantOrder) {
		t.Errorf("cross-page move before: heading order = %v, want %v\nfile:\n%s", order, wantOrder, string(tgtData))
	}
}

func TestMoveBlockCrossPage_PageNotFound(t *testing.T) {
	dir := t.TempDir()
	c := New(dir)

	// Create only a source file.
	srcPath := filepath.Join(dir, "exists.md")
	if err := os.WriteFile(srcPath, []byte("# Content\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := c.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	c.mu.Lock()
	err := c.moveBlockCrossPage("nonexistent", "exists", "src", "tgt", false)
	c.mu.Unlock()
	if err == nil {
		t.Error("expected error for nonexistent source page")
	}
	if !strings.Contains(err.Error(), "source page not found") {
		t.Errorf("error = %q, want to contain 'source page not found'", err.Error())
	}

	c.mu.Lock()
	err = c.moveBlockCrossPage("exists", "nonexistent", "src", "tgt", false)
	c.mu.Unlock()
	if err == nil {
		t.Error("expected error for nonexistent target page")
	}
	if !strings.Contains(err.Error(), "target page not found") {
		t.Errorf("error = %q, want to contain 'target page not found'", err.Error())
	}
}

func TestMoveBlock_SourceNotFound(t *testing.T) {
	c := testWritableVault(t)
	ctx := context.Background()

	// Create a target block.
	_, err := c.CreatePage(ctx, "move-err", nil, nil)
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}
	tgtBlock, err := c.AppendBlockInPage(ctx, "move-err", "# Target")
	if err != nil {
		t.Fatalf("AppendBlockInPage: %v", err)
	}

	err = c.MoveBlock(ctx, "nonexistent-uuid", tgtBlock.UUID, nil)
	if err == nil {
		t.Error("MoveBlock with nonexistent source should return error")
	}
	if !strings.Contains(err.Error(), "source block not found") {
		t.Errorf("error = %q, want to contain 'source block not found'", err.Error())
	}
}

func TestMoveBlock_TargetNotFound(t *testing.T) {
	c := testWritableVault(t)
	ctx := context.Background()

	_, err := c.CreatePage(ctx, "move-err2", nil, nil)
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}
	srcBlock, err := c.AppendBlockInPage(ctx, "move-err2", "# Source")
	if err != nil {
		t.Fatalf("AppendBlockInPage: %v", err)
	}

	err = c.MoveBlock(ctx, srcBlock.UUID, "nonexistent-uuid", nil)
	if err == nil {
		t.Error("MoveBlock with nonexistent target should return error")
	}
	if !strings.Contains(err.Error(), "target block not found") {
		t.Errorf("error = %q, want to contain 'target block not found'", err.Error())
	}
}

func TestRemoveContentFromFile(t *testing.T) {
	dir := t.TempDir()
	c := New(dir)

	// Create a file manually.
	filePath := filepath.Join(dir, "remove-test.md")
	original := "# First\n# Second\n# Third\n"
	if err := os.WriteFile(filePath, []byte(original), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cached := &cachedPage{filePath: "remove-test.md"}

	t.Run("removes content with trailing newline", func(t *testing.T) {
		// Reset file.
		if err := os.WriteFile(filePath, []byte(original), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		got, err := c.removeContentFromFile(cached, "# Second")
		if err != nil {
			t.Fatalf("removeContentFromFile: %v", err)
		}
		if strings.Contains(got, "# Second") {
			t.Errorf("returned string still contains removed content: %q", got)
		}
		if !strings.Contains(got, "# First") || !strings.Contains(got, "# Third") {
			t.Errorf("other content missing: %q", got)
		}

		// Verify file was written.
		data, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		if string(data) != got {
			t.Errorf("file content does not match returned string:\nfile: %q\nreturned: %q", string(data), got)
		}
	})

	t.Run("removes content without trailing newline", func(t *testing.T) {
		// File with content at the very end (no trailing newline after it).
		noTrail := "# First\n# Last"
		if err := os.WriteFile(filePath, []byte(noTrail), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		got, err := c.removeContentFromFile(cached, "# Last")
		if err != nil {
			t.Fatalf("removeContentFromFile: %v", err)
		}
		if strings.Contains(got, "# Last") {
			t.Errorf("returned string still contains removed content: %q", got)
		}
		if !strings.Contains(got, "# First") {
			t.Errorf("other content missing: %q", got)
		}
	})
}

func TestPrependBlockInPage_AutoCreate(t *testing.T) {
	c := testWritableVault(t)
	ctx := context.Background()

	// Prepend to a non-existent page — should auto-create it.
	block, err := c.PrependBlockInPage(ctx, "prepend-auto", "Prepended to new page")
	if err != nil {
		t.Fatalf("PrependBlockInPage auto-create: %v", err)
	}
	if block == nil {
		t.Fatal("returned nil block")
	}
	if block.UUID == "" {
		t.Error("block UUID should not be empty")
	}
	if block.Content == "" {
		t.Error("block Content should not be empty")
	}

	// The page should now exist.
	page, err := c.GetPage(ctx, "prepend-auto")
	if err != nil {
		t.Fatalf("GetPage after auto-create: %v", err)
	}
	if page == nil {
		t.Fatal("auto-created page not found")
	}

	// Verify file was created on disk.
	absPath := filepath.Join(c.vaultPath, "prepend-auto.md")
	data, err := os.ReadFile(absPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "Prepended to new page") {
		t.Errorf("file content = %q, should contain 'Prepended to new page'", string(data))
	}
}

func TestPrependBlockInPage_WithFrontmatter(t *testing.T) {
	c := testWritableVault(t)
	ctx := context.Background()

	// Create a page with frontmatter.
	_, err := c.CreatePage(ctx, "prepend-fm", map[string]any{"type": "test"}, nil)
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}

	// Append an existing block after frontmatter so the page has content.
	_, err = c.AppendBlockInPage(ctx, "prepend-fm", "# Existing Block")
	if err != nil {
		t.Fatalf("AppendBlockInPage: %v", err)
	}

	// Prepend a block — should go after frontmatter but before existing content.
	block, err := c.PrependBlockInPage(ctx, "prepend-fm", "Prepended after frontmatter")
	if err != nil {
		t.Fatalf("PrependBlockInPage: %v", err)
	}
	if block == nil {
		t.Fatal("returned nil block")
	}

	// Read the raw file to verify ordering.
	absPath := filepath.Join(c.vaultPath, "prepend-fm.md")
	data, err := os.ReadFile(absPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	fileStr := string(data)

	// Frontmatter should still be at the beginning.
	if !strings.HasPrefix(fileStr, "---\n") {
		t.Errorf("file should start with frontmatter, got: %q", fileStr[:min(50, len(fileStr))])
	}

	// Prepended content should appear before existing content.
	idxPrepended := strings.Index(fileStr, "Prepended after frontmatter")
	idxExisting := strings.Index(fileStr, "Existing Block")
	if idxPrepended < 0 {
		t.Fatal("prepended content not found in file")
	}
	if idxExisting < 0 {
		t.Fatal("existing content not found in file")
	}
	if idxPrepended >= idxExisting {
		t.Errorf("prepended content (pos %d) should appear before existing content (pos %d)", idxPrepended, idxExisting)
	}
}

func TestPrependBlockInPage_EmptyExistingPage(t *testing.T) {
	c := testWritableVault(t)
	ctx := context.Background()

	// Create an empty page (no content, no frontmatter).
	_, err := c.CreatePage(ctx, "prepend-empty", nil, nil)
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}

	// Prepend to the empty page.
	block, err := c.PrependBlockInPage(ctx, "prepend-empty", "Content into empty page")
	if err != nil {
		t.Fatalf("PrependBlockInPage: %v", err)
	}
	if block == nil {
		t.Fatal("returned nil block")
	}

	// Verify the block is now the first (and only) content.
	blocks, err := c.GetPageBlocksTree(ctx, "prepend-empty")
	if err != nil {
		t.Fatalf("GetPageBlocksTree: %v", err)
	}
	if len(blocks) == 0 {
		t.Fatal("expected at least 1 block after prepend")
	}
	if !strings.Contains(blocks[0].Content, "Content into empty page") {
		t.Errorf("first block = %q, expected prepended content", blocks[0].Content)
	}
}

func TestInsertContentInFile(t *testing.T) {
	dir := t.TempDir()
	c := New(dir)

	filePath := filepath.Join(dir, "insert-test.md")
	original := "# Target\n# Other\n"
	if err := os.WriteFile(filePath, []byte(original), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cached := &cachedPage{filePath: "insert-test.md"}

	t.Run("insert after", func(t *testing.T) {
		if err := os.WriteFile(filePath, []byte(original), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		got, err := c.insertContentInFile(cached, "# Inserted", "# Target", false)
		if err != nil {
			t.Fatalf("insertContentInFile: %v", err)
		}
		idxTarget := strings.Index(got, "# Target")
		idxInserted := strings.Index(got, "# Inserted")
		if idxTarget < 0 || idxInserted < 0 {
			t.Fatalf("content not found in result: %q", got)
		}
		if idxInserted <= idxTarget {
			t.Errorf("inserted content should appear after target, got target@%d inserted@%d", idxTarget, idxInserted)
		}

		// Verify file was written.
		data, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		if string(data) != got {
			t.Errorf("file content does not match returned string")
		}
	})

	t.Run("insert before", func(t *testing.T) {
		if err := os.WriteFile(filePath, []byte(original), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		got, err := c.insertContentInFile(cached, "# Inserted", "# Target", true)
		if err != nil {
			t.Fatalf("insertContentInFile: %v", err)
		}
		idxTarget := strings.Index(got, "# Target")
		idxInserted := strings.Index(got, "# Inserted")
		if idxTarget < 0 || idxInserted < 0 {
			t.Fatalf("content not found in result: %q", got)
		}
		if idxInserted >= idxTarget {
			t.Errorf("inserted content should appear before target, got target@%d inserted@%d", idxTarget, idxInserted)
		}
	})
}
