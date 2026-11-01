package vault

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/unbound-force/dewey/v3/backend"
	"github.com/unbound-force/dewey/v3/store"
	"github.com/unbound-force/dewey/v3/types"
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

func TestFindBlocksByTag_FrontmatterTag(t *testing.T) {
	dir := t.TempDir()
	learning := "---\ntag: scaffold-agents-md-stale-refs\nauthor: alice\n---\n\nReplace /review-council with /uf.review-council.\n"
	if err := os.WriteFile(filepath.Join(dir, "learning-note.md"), []byte(learning), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte("See #decision in the log.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := New(dir)
	if err := c.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	ctx := context.Background()
	results, err := c.FindBlocksByTag(ctx, "scaffold-agents-md-stale-refs", false)
	if err != nil {
		t.Fatalf("FindBlocksByTag: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("frontmatter tag results = %d, want 1", len(results))
	}
	if results[0].Page != "learning-note" {
		t.Errorf("page = %q, want learning-note", results[0].Page)
	}
	if len(results[0].Blocks) == 0 {
		t.Fatal("expected at least one block for the learning")
	}
	if !strings.Contains(results[0].Blocks[0].Content, "review-council") {
		t.Errorf("content = %q, want learning body", results[0].Blocks[0].Content)
	}

	inline, err := c.FindBlocksByTag(ctx, "decision", false)
	if err != nil {
		t.Fatalf("FindBlocksByTag inline: %v", err)
	}
	if len(inline) != 1 || inline[0].Page != "notes" {
		t.Fatalf("inline #decision results = %+v, want notes page", inline)
	}

	none, err := c.FindBlocksByTag(ctx, "missing-tag", false)
	if err != nil {
		t.Fatalf("FindBlocksByTag missing: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("missing tag results = %d, want 0", len(none))
	}
}

func TestFindBlocksByTag_FrontmatterTagsList(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "page.md"), []byte("---\ntags: [go, mcp]\n---\n\nBody without hash tags.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := New(dir)
	if err := c.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	results, err := c.FindBlocksByTag(context.Background(), "mcp", false)
	if err != nil {
		t.Fatalf("FindBlocksByTag: %v", err)
	}
	if len(results) != 1 || results[0].Page != "page" {
		t.Fatalf("tags list results = %+v, want page", results)
	}
}

func TestFindBlocksByTag_StoreLearningPages(t *testing.T) {
	dir := t.TempDir()
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	pageName := "learning/scaffold-agents-md-stale-refs-20260328T120000-alice"
	if err := s.InsertPage(&store.Page{
		Name:         pageName,
		OriginalName: pageName,
		SourceID:     "learning",
		SourceDocID:  "learning-scaffold-agents-md-stale-refs-20260328T120000-alice",
		Properties:   `{"tag":"scaffold-agents-md-stale-refs","author":"alice","created_at":"2026-03-28T12:00:00Z"}`,
		Tier:         "draft",
	}); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}
	if err := s.InsertBlock(&store.Block{
		UUID:     "learn-block-1",
		PageName: pageName,
		Content:  "Replace /review-council with /uf.review-council.",
	}); err != nil {
		t.Fatalf("InsertBlock: %v", err)
	}

	// Dual-written markdown lives under .uf/ and is skipped by Load().
	learnDir := filepath.Join(dir, ".uf", "dewey", "learnings")
	if err := os.MkdirAll(learnDir, 0o755); err != nil {
		t.Fatal(err)
	}
	md := "---\ntag: scaffold-agents-md-stale-refs\nauthor: alice\n---\n\nReplace /review-council with /uf.review-council.\n"
	if err := os.WriteFile(filepath.Join(learnDir, "scaffold-agents-md-stale-refs-20260328T120000-alice.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}

	noStore := New(dir)
	if err := noStore.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	none, err := noStore.FindBlocksByTag(context.Background(), "scaffold-agents-md-stale-refs", false)
	if err != nil {
		t.Fatalf("FindBlocksByTag no store: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("without store, hidden .uf learnings should be skipped, got %+v", none)
	}

	c := New(dir, WithStore(s))
	if err := c.Load(); err != nil {
		t.Fatalf("Load with store: %v", err)
	}
	results, err := c.FindBlocksByTag(context.Background(), "scaffold-agents-md-stale-refs", false)
	if err != nil {
		t.Fatalf("FindBlocksByTag: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("store learning results = %d, want 1: %+v", len(results), results)
	}
	if results[0].Page != pageName {
		t.Errorf("page = %q, want %q", results[0].Page, pageName)
	}
	if len(results[0].Blocks) == 0 || !strings.Contains(results[0].Blocks[0].Content, "review-council") {
		t.Errorf("blocks = %+v, want learning body", results[0].Blocks)
	}

	miss, err := c.FindBlocksByTag(context.Background(), "other-tag", false)
	if err != nil {
		t.Fatalf("FindBlocksByTag miss: %v", err)
	}
	if len(miss) != 0 {
		t.Errorf("other-tag results = %d, want 0", len(miss))
	}
}

func TestFindBlocksByTag_StoreListError(t *testing.T) {
	dir := t.TempDir()
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	c := New(dir, WithStore(s))
	if err := c.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	_, err = c.FindBlocksByTag(context.Background(), "any", false)
	if err == nil {
		t.Fatal("expected store query error, got nil")
	}
}

// --- FindBlocksByTag store merge tests (mcp-internal-store-access task 2.6) ---

func TestFindBlocksByTag_StoreBlockContentTag(t *testing.T) {
	// A store-only learning page written after the client was created, with
	// no frontmatter tag: only the live store merge — scanning block content
	// for #tags via parser.Parse, same as the in-memory scan — can surface it
	// without a restart (spec: Unified Tag Search, block-content match kind).
	c, s := newStoreBackedClient(t, map[string]string{
		"readme.md": "# Vault\n\nLocal page.\n",
	})

	pageName := "learning/deploy-rollback-20260329T090000-bob"
	insertStorePageWithBlocks(t, s,
		&store.Page{
			Name:         pageName,
			OriginalName: pageName,
			SourceID:     "learning",
			SourceDocID:  "deploy-rollback-20260329T090000-bob",
			// No tag/tags frontmatter: only the block content carries the tag.
			Properties: `{"author":"bob","created_at":"2026-03-29T09:00:00Z"}`,
			Tier:       "draft",
		},
		&store.Block{UUID: "rollback-intro", PageName: pageName, Content: "Intro line with no tag.", Position: 0},
		&store.Block{UUID: "rollback-body", PageName: pageName, Content: "Prefer deploy #rollback over manual patching.", Position: 1},
	)

	results, err := c.FindBlocksByTag(context.Background(), "rollback", false)
	if err != nil {
		t.Fatalf("FindBlocksByTag: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1: %+v", len(results), results)
	}
	if results[0].Page != pageName {
		t.Errorf("page = %q, want %q", results[0].Page, pageName)
	}
	// Block-content matching is per-block: only the block carrying #rollback
	// is returned, not the whole page body (contrast: metadata tag matches).
	if len(results[0].Blocks) != 1 {
		t.Fatalf("blocks = %d, want 1 (only the tagged block): %+v", len(results[0].Blocks), results[0].Blocks)
	}
	if !strings.Contains(results[0].Blocks[0].Content, "#rollback") {
		t.Errorf("block content = %q, want the #rollback block", results[0].Blocks[0].Content)
	}
}

func TestFindBlocksByTag_StoreCompiledTopicProperty(t *testing.T) {
	// The topic property of a compiled-source page is tag-equivalent (design
	// D5 default): a metadata match surfaces the whole page body. The control
	// page proves the topic rule is source-scoped — the same topic on a
	// learning-source page must NOT match.
	c, s := newStoreBackedClient(t, map[string]string{
		"readme.md": "# Vault\n\nLocal page.\n",
	})

	insertStorePageWithBlocks(t, s,
		&store.Page{
			Name:         "compiled/gateway-routing",
			OriginalName: "gateway-routing",
			SourceID:     "compiled",
			SourceDocID:  "gateway-routing",
			// No tag/tags frontmatter and no #tag in content: only topic matches.
			Properties: `{"topic":"gateway","tier":"draft"}`,
		},
		&store.Block{UUID: "gw-root", PageName: "compiled/gateway-routing", Content: "Gateway routing summary", HeadingLevel: 1, Position: 0},
		&store.Block{UUID: "gw-body", PageName: "compiled/gateway-routing", Content: "Route traffic through the gateway.", Position: 1},
	)
	// Control: topic on a non-compiled source is not tag-equivalent.
	insertStorePageWithBlocks(t, s,
		&store.Page{
			Name:         "learning/gateway-notes-20260329T100000-carol",
			OriginalName: "learning/gateway-notes-20260329T100000-carol",
			SourceID:     "learning",
			SourceDocID:  "gateway-notes-20260329T100000-carol",
			Properties:   `{"topic":"gateway","author":"carol"}`,
			Tier:         "draft",
		},
		&store.Block{UUID: "gw-notes-body", PageName: "learning/gateway-notes-20260329T100000-carol", Content: "Notes about the gateway without any inline tag.", Position: 0},
	)

	results, err := c.FindBlocksByTag(context.Background(), "gateway", false)
	if err != nil {
		t.Fatalf("FindBlocksByTag: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1 (compiled topic match only, learning control must not match): %+v", len(results), results)
	}
	if results[0].Page != "compiled/gateway-routing" {
		t.Errorf("page = %q, want %q", results[0].Page, "compiled/gateway-routing")
	}
	// A topic (metadata) match surfaces the whole body, not a single block.
	if len(results[0].Blocks) != 2 {
		t.Fatalf("blocks = %d, want 2 (metadata match surfaces the whole body): %+v", len(results[0].Blocks), results[0].Blocks)
	}
	if !blockTreeContains(results[0].Blocks, "Route traffic through the gateway") {
		t.Errorf("blocks = %+v, want the compiled article body", results[0].Blocks)
	}
}

func TestFindBlocksByTag_DedupesMirroredPages(t *testing.T) {
	// The vault file notes/tag-mirror.md matches in memory; the store
	// carries a same-named learning page whose own block would also match.
	// The mirrored row must never surface a second entry — vault files win
	// on name collisions (design D3) — and its blocks must not leak into
	// the vault-file result.
	c, s := newStoreBackedClient(t, map[string]string{
		"notes/tag-mirror.md": "# Tag Mirror\n\nvault file content with #mirror-tag\n",
	})

	insertStorePageWithBlocks(t, s,
		&store.Page{
			Name:         "notes/tag-mirror",
			OriginalName: "notes/tag-mirror",
			SourceID:     "learning",
			SourceDocID:  "notes-tag-mirror",
		},
		&store.Block{UUID: "mirror-store-1", PageName: "notes/tag-mirror", Content: "store-side #mirror-tag row", Position: 0},
	)

	results, err := c.FindBlocksByTag(context.Background(), "mirror-tag", false)
	if err != nil {
		t.Fatalf("FindBlocksByTag: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want exactly 1 (mirrored store row deduplicated): %+v", len(results), results)
	}
	if results[0].Page != "notes/tag-mirror" {
		t.Errorf("page = %q, want %q", results[0].Page, "notes/tag-mirror")
	}
	if len(results[0].Blocks) == 0 || !strings.Contains(results[0].Blocks[0].Content, "vault file content") {
		t.Errorf("blocks = %+v, want the vault file block (store rows must not leak)", results[0].Blocks)
	}
	for _, b := range results[0].Blocks {
		if strings.Contains(b.Content, "store-side") {
			t.Errorf("block content = %q, store-side mirror row leaked", b.Content)
		}
	}
}

func TestFrontmatterHasTag(t *testing.T) {
	tests := []struct {
		name  string
		props map[string]any
		tag   string
		want  bool
	}{
		{name: "nil", props: nil, tag: "x", want: false},
		{name: "empty tag", props: map[string]any{"tag": "x"}, tag: "", want: false},
		{name: "singular", props: map[string]any{"tag": "Foo"}, tag: "foo", want: true},
		{name: "list any", props: map[string]any{"tags": []any{"go", "mcp"}}, tag: "mcp", want: true},
		{name: "mismatch", props: map[string]any{"tag": "a"}, tag: "b", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := frontmatterHasTag(tt.props, tt.tag); got != tt.want {
				t.Errorf("frontmatterHasTag(%v, %q) = %v, want %v", tt.props, tt.tag, got, tt.want)
			}
		})
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

// --- Write guard tests (004-unified-content-serve T035) ---

// newVaultWithReadOnlyPage creates a vault client with a read-only page and
// a writable page for write guard testing. Returns the client, the read-only
// page name, and a block UUID from the read-only page.
func newVaultWithReadOnlyPage(t *testing.T) (*Client, string, string) {
	t.Helper()
	vaultDir := t.TempDir()

	// Create a writable local page.
	localPath := filepath.Join(vaultDir, "local-page.md")
	if err := os.WriteFile(localPath, []byte("# Local Page\n\nLocal content."), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	c := New(vaultDir)
	if err := c.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Add a read-only external page directly to the vault's pages map.
	readOnlyBlockUUID := "ext-block-uuid-1"
	extPage := &cachedPage{
		entity: types.PageEntity{
			Name:         "github-org/issue-42",
			OriginalName: "Bug Report",
		},
		lowerName: "github-org/issue-42",
		filePath:  "issue-42",
		blocks: []types.BlockEntity{
			{UUID: readOnlyBlockUUID, Content: "External content"},
		},
		sourceID: "github-org",
		readOnly: true,
	}
	c.mu.Lock()
	c.applyPageIndex(extPage)
	c.mu.Unlock()

	return c, "github-org/issue-42", readOnlyBlockUUID
}

func TestWriteGuard_AppendBlockInPage(t *testing.T) {
	c, roPage, _ := newVaultWithReadOnlyPage(t)
	ctx := context.Background()

	// Should fail on read-only page.
	_, err := c.AppendBlockInPage(ctx, roPage, "new content")
	if err == nil {
		t.Fatal("expected error for AppendBlockInPage on read-only page")
	}
	if !strings.Contains(err.Error(), "read-only") {
		t.Errorf("error = %q, want to contain 'read-only'", err.Error())
	}
	if !strings.Contains(err.Error(), "github-org") {
		t.Errorf("error = %q, want to contain source ID 'github-org'", err.Error())
	}

	// Should succeed on writable page.
	_, err = c.AppendBlockInPage(ctx, "local-page", "appended content")
	if err != nil {
		t.Errorf("AppendBlockInPage on writable page failed: %v", err)
	}
}

func TestWriteGuard_PrependBlockInPage(t *testing.T) {
	c, roPage, _ := newVaultWithReadOnlyPage(t)
	ctx := context.Background()

	_, err := c.PrependBlockInPage(ctx, roPage, "new content")
	if err == nil {
		t.Fatal("expected error for PrependBlockInPage on read-only page")
	}
	if !strings.Contains(err.Error(), "read-only") {
		t.Errorf("error = %q, want to contain 'read-only'", err.Error())
	}
}

func TestWriteGuard_UpdateBlock(t *testing.T) {
	c, _, roBlockUUID := newVaultWithReadOnlyPage(t)
	ctx := context.Background()

	err := c.UpdateBlock(ctx, roBlockUUID, "updated content")
	if err == nil {
		t.Fatal("expected error for UpdateBlock on read-only page")
	}
	if !strings.Contains(err.Error(), "read-only") {
		t.Errorf("error = %q, want to contain 'read-only'", err.Error())
	}
}

func TestWriteGuard_RemoveBlock(t *testing.T) {
	c, _, roBlockUUID := newVaultWithReadOnlyPage(t)
	ctx := context.Background()

	err := c.RemoveBlock(ctx, roBlockUUID)
	if err == nil {
		t.Fatal("expected error for RemoveBlock on read-only page")
	}
	if !strings.Contains(err.Error(), "read-only") {
		t.Errorf("error = %q, want to contain 'read-only'", err.Error())
	}
}

func TestWriteGuard_InsertBlock(t *testing.T) {
	c, _, roBlockUUID := newVaultWithReadOnlyPage(t)
	ctx := context.Background()

	_, err := c.InsertBlock(ctx, roBlockUUID, "child content", nil)
	if err == nil {
		t.Fatal("expected error for InsertBlock on read-only page")
	}
	if !strings.Contains(err.Error(), "read-only") {
		t.Errorf("error = %q, want to contain 'read-only'", err.Error())
	}
}

func TestWriteGuard_DeletePage(t *testing.T) {
	c, roPage, _ := newVaultWithReadOnlyPage(t)
	ctx := context.Background()

	err := c.DeletePage(ctx, roPage)
	if err == nil {
		t.Fatal("expected error for DeletePage on read-only page")
	}
	if !strings.Contains(err.Error(), "read-only") {
		t.Errorf("error = %q, want to contain 'read-only'", err.Error())
	}
}

func TestWriteGuard_RenamePage(t *testing.T) {
	c, roPage, _ := newVaultWithReadOnlyPage(t)
	ctx := context.Background()

	err := c.RenamePage(ctx, roPage, "new-name")
	if err == nil {
		t.Fatal("expected error for RenamePage on read-only page")
	}
	if !strings.Contains(err.Error(), "read-only") {
		t.Errorf("error = %q, want to contain 'read-only'", err.Error())
	}
}

func TestWriteGuard_MoveBlock(t *testing.T) {
	c, _, roBlockUUID := newVaultWithReadOnlyPage(t)
	ctx := context.Background()

	// Get a writable block UUID from the local page.
	c.mu.RLock()
	localPage := c.pages["local-page"]
	c.mu.RUnlock()
	if localPage == nil || len(localPage.blocks) == 0 {
		t.Fatal("local page has no blocks")
	}
	localBlockUUID := localPage.blocks[0].UUID

	// Moving FROM read-only page should fail.
	err := c.MoveBlock(ctx, roBlockUUID, localBlockUUID, nil)
	if err == nil {
		t.Fatal("expected error for MoveBlock from read-only page")
	}
	if !strings.Contains(err.Error(), "read-only") {
		t.Errorf("error = %q, want to contain 'read-only'", err.Error())
	}

	// Moving TO read-only page should also fail.
	err = c.MoveBlock(ctx, localBlockUUID, roBlockUUID, nil)
	if err == nil {
		t.Fatal("expected error for MoveBlock to read-only page")
	}
	if !strings.Contains(err.Error(), "read-only") {
		t.Errorf("error = %q, want to contain 'read-only'", err.Error())
	}
}

// --- Cross-source backlink tests (004-unified-content-serve T036-T038) ---

func TestCrossSourceBacklinks_ExternalToLocal(t *testing.T) {
	vaultDir := t.TempDir()

	// Create a local page "architecture".
	archPath := filepath.Join(vaultDir, "architecture.md")
	if err := os.WriteFile(archPath, []byte("# Architecture\n\nSystem design."), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	c := New(vaultDir)
	if err := c.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Add an external page that references [[architecture]] via wikilink.
	extPage := &cachedPage{
		entity: types.PageEntity{
			Name:         "github-org/issue-42",
			OriginalName: "Bug Report",
		},
		lowerName: "github-org/issue-42",
		filePath:  "issue-42",
		blocks: []types.BlockEntity{
			{UUID: "ext-block-1", Content: "Found a bug in [[architecture]] module."},
		},
		sourceID: "github-org",
		readOnly: true,
	}
	c.mu.Lock()
	c.applyPageIndex(extPage)
	c.mu.Unlock()

	// Build backlinks — should include cross-source references.
	c.BuildBacklinks()

	// Query backlinks for "architecture" — should include the external page.
	ctx := context.Background()
	refs, err := c.GetPageLinkedReferences(ctx, "architecture")
	if err != nil {
		t.Fatalf("GetPageLinkedReferences: %v", err)
	}

	// Parse the JSON response to check for the external page.
	var result []json.RawMessage
	if err := json.Unmarshal(refs, &result); err != nil {
		t.Fatalf("unmarshal refs: %v", err)
	}

	if len(result) == 0 {
		t.Fatal("expected at least one backlink reference, got 0")
	}

	// The backlink should come from the external page.
	foundExternal := false
	for _, entry := range result {
		if strings.Contains(string(entry), "github-org/issue-42") || strings.Contains(string(entry), "Bug Report") {
			foundExternal = true
			break
		}
	}
	if !foundExternal {
		t.Errorf("external page not found in backlinks for 'architecture'. Got: %s", string(refs))
	}
}

func TestCrossSourceBacklinks_ExternalToExternal(t *testing.T) {
	c := New(t.TempDir())

	// Add two external pages that reference each other.
	page1 := &cachedPage{
		entity: types.PageEntity{
			Name:         "github-org/issue-1",
			OriginalName: "Issue 1",
		},
		lowerName: "github-org/issue-1",
		filePath:  "issue-1",
		blocks: []types.BlockEntity{
			{UUID: "block-1", Content: "Related to [[github-org/issue-2]]."},
		},
		sourceID: "github-org",
		readOnly: true,
	}
	page2 := &cachedPage{
		entity: types.PageEntity{
			Name:         "github-org/issue-2",
			OriginalName: "Issue 2",
		},
		lowerName: "github-org/issue-2",
		filePath:  "issue-2",
		blocks: []types.BlockEntity{
			{UUID: "block-2", Content: "See also [[github-org/issue-1]]."},
		},
		sourceID: "github-org",
		readOnly: true,
	}

	c.mu.Lock()
	c.applyPageIndex(page1)
	c.applyPageIndex(page2)
	c.mu.Unlock()

	c.BuildBacklinks()

	ctx := context.Background()

	// Check backlinks for issue-1 — should include issue-2.
	refs1, err := c.GetPageLinkedReferences(ctx, "github-org/issue-1")
	if err != nil {
		t.Fatalf("GetPageLinkedReferences(issue-1): %v", err)
	}
	if !strings.Contains(string(refs1), "github-org/issue-2") && !strings.Contains(string(refs1), "Issue 2") {
		t.Errorf("issue-2 not found in backlinks for issue-1. Got: %s", string(refs1))
	}

	// Check backlinks for issue-2 — should include issue-1.
	refs2, err := c.GetPageLinkedReferences(ctx, "github-org/issue-2")
	if err != nil {
		t.Fatalf("GetPageLinkedReferences(issue-2): %v", err)
	}
	if !strings.Contains(string(refs2), "github-org/issue-1") && !strings.Contains(string(refs2), "Issue 1") {
		t.Errorf("issue-1 not found in backlinks for issue-2. Got: %s", string(refs2))
	}
}

// --- Ignore pattern integration tests (006-unified-ignore Phase 3) ---

func TestLoad_GitignoreRespected(t *testing.T) {
	vaultDir := t.TempDir()

	// Create a .gitignore that excludes node_modules/.
	gitignorePath := filepath.Join(vaultDir, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte("node_modules/\n"), 0o644); err != nil {
		t.Fatalf("WriteFile .gitignore: %v", err)
	}

	// Create node_modules/pkg/README.md (should be ignored).
	nmDir := filepath.Join(vaultDir, "node_modules", "pkg")
	if err := os.MkdirAll(nmDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nmDir, "README.md"), []byte("# Package README"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Create docs/guide.md (should be indexed).
	docsDir := filepath.Join(vaultDir, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, "guide.md"), []byte("# Guide"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	c := New(vaultDir)
	if err := c.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	ctx := context.Background()
	pages, err := c.GetAllPages(ctx)
	if err != nil {
		t.Fatalf("GetAllPages: %v", err)
	}

	foundGuide := false
	for _, p := range pages {
		if strings.Contains(p.Name, "node_modules") {
			t.Errorf("node_modules page should be ignored, found: %s", p.Name)
		}
		if p.Name == "docs/guide" {
			foundGuide = true
		}
	}
	if !foundGuide {
		t.Error("docs/guide should be indexed but was not found")
	}
}

func TestLoad_NoGitignore(t *testing.T) {
	vaultDir := t.TempDir()

	// Create src/main.md.
	srcDir := filepath.Join(vaultDir, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "main.md"), []byte("# Main"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Create docs/guide.md.
	docsDir := filepath.Join(vaultDir, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, "guide.md"), []byte("# Guide"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// No .gitignore file — backward compatibility test.
	c := New(vaultDir)
	if err := c.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	ctx := context.Background()
	pages, err := c.GetAllPages(ctx)
	if err != nil {
		t.Fatalf("GetAllPages: %v", err)
	}

	foundMain := false
	foundGuide := false
	for _, p := range pages {
		if p.Name == "src/main" {
			foundMain = true
		}
		if p.Name == "docs/guide" {
			foundGuide = true
		}
	}
	if !foundMain {
		t.Error("src/main should be indexed but was not found")
	}
	if !foundGuide {
		t.Error("docs/guide should be indexed but was not found")
	}
}

func TestWriteGuard_WritablePagePassesThrough(t *testing.T) {
	c, _, _ := newVaultWithReadOnlyPage(t)
	ctx := context.Background()

	// Verify that writable pages are not blocked by write guards.
	// AppendBlockInPage on a writable page should succeed.
	_, err := c.AppendBlockInPage(ctx, "local-page", "test content")
	if err != nil {
		t.Errorf("AppendBlockInPage on writable page should succeed: %v", err)
	}
}

// --- GetPage / GetPageBlocksTree store fallback tests (mcp-internal-store-access task 2.2) ---

// newStoreBackedClient creates a vault Client over a temp-dir vault with an
// in-memory persistent store attached via WithStore (the production wiring),
// loads the vault files, and returns the client plus the underlying store so
// tests can write rows directly — simulating same-session store_compiled /
// store_learning calls that bypass the vault client.
func newStoreBackedClient(t *testing.T, files map[string]string) (*Client, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		abs := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", abs, err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%s): %v", abs, err)
		}
	}
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New(:memory:): %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	c := New(dir, WithStore(s))
	if err := c.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	return c, s
}

// blockTreeContains reports whether any block in the tree contains substr.
func blockTreeContains(blocks []types.BlockEntity, substr string) bool {
	for _, b := range blocks {
		if strings.Contains(b.Content, substr) {
			return true
		}
		if len(b.Children) > 0 && blockTreeContains(b.Children, substr) {
			return true
		}
	}
	return false
}

func TestGetPage_VaultFilePrecedenceOverStore(t *testing.T) {
	c, s := newStoreBackedClient(t, map[string]string{
		"notes/topic.md": "# Topic\n\nFrom the vault file.\n",
	})

	// Same-named page in the store from a non-vault source
	// (spec: Vault Precedence on Name Collision).
	insertStorePageWithBlocks(t, s,
		&store.Page{
			Name:         "notes/topic",
			OriginalName: "notes/topic",
			SourceID:     "github-org",
			Properties:   `{"origin": "external"}`,
		},
		&store.Block{UUID: "ext-topic-root", PageName: "notes/topic", Content: "# From the store", Position: 0},
	)

	ctx := context.Background()

	page, err := c.GetPage(ctx, "notes/topic")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if page == nil {
		t.Fatal("page = nil, want the vault file page")
	}
	// The vault file has no frontmatter; the store page's properties must not leak.
	if page.Properties != nil {
		t.Errorf("Properties = %v, want nil (vault file has no frontmatter)", page.Properties)
	}

	blocks, err := c.GetPageBlocksTree(ctx, "notes/topic")
	if err != nil {
		t.Fatalf("GetPageBlocksTree: %v", err)
	}
	if len(blocks) == 0 {
		t.Fatal("blocks = empty, want the vault file's blocks")
	}
	if blockTreeContains(blocks, "From the store") {
		t.Error("store page content leaked through; the vault file page must take precedence")
	}
	if !blockTreeContains(blocks, "From the vault file") {
		t.Errorf("blocks = %+v, want vault file content", blocks)
	}
}

func TestGetPage_StoreFallbackOnMemoryMiss(t *testing.T) {
	// No vault file for compiled/gateway — the page lives only in the store,
	// written after the client was created (same-session store_compiled shape).
	c, s := newStoreBackedClient(t, map[string]string{
		"readme.md": "# Vault\n\nLocal page.\n",
	})

	pageName := "compiled/gateway"
	insertStorePageWithBlocks(t, s,
		&store.Page{
			Name:         pageName,
			OriginalName: "gateway",
			SourceID:     "compiled",
			Properties:   `{"topic": "gateway", "tier": "draft"}`,
		},
		&store.Block{UUID: "gw-root", PageName: pageName, Content: "# Gateway", HeadingLevel: 1, Position: 0},
		&store.Block{
			UUID:       "gw-child",
			PageName:   pageName,
			ParentUUID: sql.NullString{String: "gw-root", Valid: true},
			Content:    "Body text.",
			Position:   0,
		},
	)

	ctx := context.Background()

	page, err := c.GetPage(ctx, pageName)
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if page == nil {
		t.Fatal("page = nil, want the store-backed page (same-session visibility)")
	}
	if page.Name != pageName {
		t.Errorf("Name = %q, want %q", page.Name, pageName)
	}
	if page.OriginalName != "gateway" {
		t.Errorf("OriginalName = %q, want %q", page.OriginalName, "gateway")
	}
	if page.Properties["topic"] != "gateway" || page.Properties["tier"] != "draft" {
		t.Errorf("Properties = %v, want topic=gateway tier=draft", page.Properties)
	}

	blocks, err := c.GetPageBlocksTree(ctx, pageName)
	if err != nil {
		t.Fatalf("GetPageBlocksTree: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("root blocks = %d, want 1", len(blocks))
	}
	if blocks[0].UUID != "gw-root" || blocks[0].Content != "# Gateway" {
		t.Errorf("root block = (%q, %q), want (gw-root, # Gateway)", blocks[0].UUID, blocks[0].Content)
	}
	if len(blocks[0].Children) != 1 || blocks[0].Children[0].Content != "Body text." {
		t.Errorf("children = %+v, want one child with body text", blocks[0].Children)
	}

	// The fallback must not cache into c.pages — the in-memory index stays
	// file-only, which is what keeps later re-stores fresh (design D2).
	c.mu.RLock()
	_, cached := c.pages[strings.ToLower(pageName)]
	c.mu.RUnlock()
	if cached {
		t.Error("store page was cached into c.pages; live lookups must not cache (design D2)")
	}
}

func TestGetPage_NoStoreParity(t *testing.T) {
	// No WithStore option: behavior must be identical to the pre-fallback
	// implementation (Constitution II — inherited-tool compatibility).
	c := New(t.TempDir())
	if err := c.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	ctx := context.Background()

	page, err := c.GetPage(ctx, "missing/page")
	if err != nil {
		t.Fatalf("GetPage miss: %v", err)
	}
	if page != nil {
		t.Errorf("GetPage miss = %+v, want nil", page)
	}

	blocks, err := c.GetPageBlocksTree(ctx, "missing/page")
	if err != nil {
		t.Fatalf("GetPageBlocksTree miss: %v", err)
	}
	if blocks != nil {
		t.Errorf("GetPageBlocksTree miss = %+v, want nil", blocks)
	}
}

func TestGetPage_RestoreIsNeverStale(t *testing.T) {
	c, s := newStoreBackedClient(t, nil)

	pageName := "compiled/gateway"

	// storeCompiled mirrors tools/compile.go StoreCompiled's replace
	// semantics: delete the page (CASCADE removes its blocks) and re-insert,
	// so the same page name is fully replaced.
	storeCompiled := func(content string) {
		t.Helper()
		_ = s.DeletePage(pageName) // absent on first call — expected
		if err := s.InsertPage(&store.Page{
			Name:         pageName,
			OriginalName: "gateway",
			SourceID:     "compiled",
			Properties:   `{"topic": "gateway", "tier": "draft"}`,
		}); err != nil {
			t.Fatalf("InsertPage: %v", err)
		}
		if err := s.InsertBlock(&store.Block{UUID: "gw-body", PageName: pageName, Content: content, Position: 0}); err != nil {
			t.Fatalf("InsertBlock: %v", err)
		}
	}

	ctx := context.Background()

	// v1 stored, then read at least once.
	storeCompiled("# Gateway v1")
	blocks, err := c.GetPageBlocksTree(ctx, pageName)
	if err != nil {
		t.Fatalf("GetPageBlocksTree v1: %v", err)
	}
	if len(blocks) != 1 || blocks[0].Content != "# Gateway v1" {
		t.Fatalf("v1 blocks = %+v, want # Gateway v1", blocks)
	}

	// Re-store as v2 in the same session. The live lookup must return v2 —
	// no mid-session cache can serve the stale v1 (spec: Freshness of Store
	// Reads).
	storeCompiled("# Gateway v2")

	blocks, err = c.GetPageBlocksTree(ctx, pageName)
	if err != nil {
		t.Fatalf("GetPageBlocksTree v2: %v", err)
	}
	if len(blocks) != 1 || blocks[0].Content != "# Gateway v2" {
		t.Fatalf("v2 blocks = %+v, want # Gateway v2 (never stale)", blocks)
	}
}

func TestGetPage_StoreErrorSurfaces(t *testing.T) {
	// A store failure must surface as an error, not be masked as a
	// not-found result (design D6 honest-error philosophy).
	c, s := newStoreBackedClient(t, nil)
	if err := s.Close(); err != nil {
		t.Fatalf("Close store: %v", err)
	}

	ctx := context.Background()

	if _, err := c.GetPage(ctx, "compiled/gateway"); err == nil {
		t.Error("GetPage: expected store query error after store close, got nil")
	}
	if _, err := c.GetPageBlocksTree(ctx, "compiled/gateway"); err == nil {
		t.Error("GetPageBlocksTree: expected store query error after store close, got nil")
	}
}

func TestGetPage_StoreFallbackConcurrent(t *testing.T) {
	// Exercise the memory-hit, store-hit, and store-miss paths concurrently
	// to validate the snapshot-then-release locking discipline under -race
	// (design D2 locking discipline).
	c, s := newStoreBackedClient(t, map[string]string{
		"readme.md": "# Vault\n\nLocal page.\n",
	})
	insertStorePageWithBlocks(t, s,
		&store.Page{
			Name:         "compiled/gateway",
			OriginalName: "gateway",
			SourceID:     "compiled",
			Properties:   `{"topic": "gateway"}`,
		},
		&store.Block{UUID: "gw-root", PageName: "compiled/gateway", Content: "# Gateway", Position: 0},
	)

	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 25; j++ {
				name := "compiled/gateway" // store hit
				switch j % 3 {
				case 0:
					name = "readme" // memory hit
				case 1:
					name = fmt.Sprintf("missing-%d-%d", i, j) // miss everywhere
				}
				if _, err := c.GetPage(ctx, name); err != nil {
					t.Errorf("GetPage(%q): %v", name, err)
					return
				}
				if _, err := c.GetPageBlocksTree(ctx, name); err != nil {
					t.Errorf("GetPageBlocksTree(%q): %v", name, err)
					return
				}
			}
		}(i)
	}
	wg.Wait()
}

// --- GetPageBlocksTree store fallback tests (mcp-internal-store-access) ---

func TestGetPageBlocksTree_StoreFallbackOnMemoryMiss(t *testing.T) {
	// No vault file for the store-only page: the block tree must resolve
	// via the live store fallback with its parent/child structure intact
	// (spec: Live Retrieval of Internal Store Pages — full reconstructed
	// block tree).
	c, s := newStoreBackedClient(t, map[string]string{
		"readme.md": "# Vault\n\nLocal page.\n",
	})

	pageName := "compiled/gateway"
	insertStorePageWithBlocks(t, s,
		&store.Page{
			Name:         pageName,
			OriginalName: "gateway",
			SourceID:     "compiled",
			Properties:   `{"topic": "gateway", "tier": "draft"}`,
		},
		&store.Block{UUID: "gw-root", PageName: pageName, Content: "# Gateway", HeadingLevel: 1, Position: 0},
		&store.Block{
			UUID:       "gw-child",
			PageName:   pageName,
			ParentUUID: sql.NullString{String: "gw-root", Valid: true},
			Content:    "Body text.",
			Position:   0,
		},
	)

	blocks, err := c.GetPageBlocksTree(context.Background(), pageName)
	if err != nil {
		t.Fatalf("GetPageBlocksTree: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("root blocks = %d, want 1: %+v", len(blocks), blocks)
	}
	if blocks[0].UUID != "gw-root" || blocks[0].Content != "# Gateway" {
		t.Errorf("root block = (%q, %q), want (gw-root, # Gateway)", blocks[0].UUID, blocks[0].Content)
	}
	if len(blocks[0].Children) != 1 ||
		blocks[0].Children[0].UUID != "gw-child" ||
		blocks[0].Children[0].Content != "Body text." {
		t.Errorf("children = %+v, want one gw-child with body text", blocks[0].Children)
	}

	// The fallback must not cache into c.pages — the in-memory index stays
	// file-only, which is what keeps later re-stores fresh (design D2).
	c.mu.RLock()
	_, cached := c.pages[strings.ToLower(pageName)]
	c.mu.RUnlock()
	if cached {
		t.Error("store page was cached into c.pages; live lookups must not cache (design D2)")
	}
}

func TestGetPageBlocksTree_StoreErrorSurfaces(t *testing.T) {
	// A store failure must surface as an error with no tree, not as a
	// nil-tree not-found (design D6 honest-error philosophy).
	c, s := newStoreBackedClient(t, nil)
	if err := s.Close(); err != nil {
		t.Fatalf("Close store: %v", err)
	}

	blocks, err := c.GetPageBlocksTree(context.Background(), "compiled/gateway")
	if err == nil {
		t.Error("GetPageBlocksTree: expected store query error after store close, got nil")
	}
	if blocks != nil {
		t.Errorf("GetPageBlocksTree blocks = %+v, want nil alongside the error", blocks)
	}
}

// --- GetBlock store fallback tests (mcp-internal-store-access task 2.3) ---

func TestGetBlock_StoreFallbackOnMemoryMiss(t *testing.T) {
	// No vault file for the learning page — the block exists only in the
	// store, written after the client was created (same-session
	// store_learning shape; spec: Block Retrieval for Store Pages).
	c, s := newStoreBackedClient(t, map[string]string{
		"readme.md": "# Vault\n\nLocal page.\n",
	})

	// Mirrors the store_learning write path: a learning/{identity} page
	// whose first block UUID is handed back to agents.
	pageName := "learning/auth-20260101T000000-alice"
	insertStorePageWithBlocks(t, s,
		&store.Page{
			Name:         pageName,
			OriginalName: pageName,
			SourceID:     "learning",
			SourceDocID:  "auth-20260101T000000-alice.md",
			Properties:   `{"tag": "auth"}`,
		},
		&store.Block{UUID: "learning-block-1", PageName: pageName, Content: "Use parameterized SQL.", Position: 0},
	)

	ctx := context.Background()

	got, err := c.GetBlock(ctx, "learning-block-1")
	if err != nil {
		t.Fatalf("GetBlock: %v", err)
	}
	if got == nil {
		t.Fatal("block = nil, want the store-backed block (same-session visibility)")
	}
	if got.UUID != "learning-block-1" {
		t.Errorf("UUID = %q, want %q", got.UUID, "learning-block-1")
	}
	if got.Content != "Use parameterized SQL." {
		t.Errorf("Content = %q, want %q", got.Content, "Use parameterized SQL.")
	}
	if got.Page == nil {
		t.Fatal("Page reference = nil, want page reference to the learning page")
	}
	if got.Page.Name != pageName {
		t.Errorf("Page.Name = %q, want %q", got.Page.Name, pageName)
	}

	// The fallback must not cache into the block index — the in-memory
	// index stays file-only, which is what keeps later re-stores fresh
	// (design D2).
	c.mu.RLock()
	_, cached := c.blockIndex["learning-block-1"]
	c.mu.RUnlock()
	if cached {
		t.Error("store block was cached into c.blockIndex; live lookups must not cache (design D2)")
	}
}

func TestGetBlock_InMemoryPrecedenceOverStore(t *testing.T) {
	c, s := newStoreBackedClient(t, map[string]string{
		"notes/topic.md": "# Topic\n\nFrom the vault file.\n",
	})

	ctx := context.Background()

	// Resolve the vault file block's generated UUID through the public tree
	// API so the store row can collide with it exactly.
	blocks, err := c.GetPageBlocksTree(ctx, "notes/topic")
	if err != nil {
		t.Fatalf("GetPageBlocksTree: %v", err)
	}
	if len(blocks) == 0 {
		t.Fatal("vault file produced no blocks")
	}
	fileUUID, fileContent := blocks[0].UUID, blocks[0].Content

	// Same UUID in the store under a different page and content: the
	// in-memory index must win (vault-file precedence, same rule as GetPage).
	insertStorePageWithBlocks(t, s,
		&store.Page{Name: "compiled/collision", SourceID: "compiled"},
		&store.Block{UUID: fileUUID, PageName: "compiled/collision", Content: "from the store", Position: 0},
	)

	got, err := c.GetBlock(ctx, fileUUID)
	if err != nil {
		t.Fatalf("GetBlock: %v", err)
	}
	if got == nil {
		t.Fatal("block = nil, want the in-memory block")
	}
	if got.Content != fileContent {
		t.Errorf("Content = %q, want in-memory %q (store copy leaked through)", got.Content, fileContent)
	}
	if got.Page == nil || got.Page.Name != "notes/topic" {
		t.Errorf("Page = %+v, want reference to notes/topic", got.Page)
	}
}

func TestGetBlock_NoStoreParity(t *testing.T) {
	// No WithStore option: behavior must be identical to the pre-fallback
	// implementation (Constitution II — inherited-tool compatibility).
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# Vault\n\nLocal page.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	c := New(dir)
	if err := c.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	ctx := context.Background()

	blocks, err := c.GetPageBlocksTree(ctx, "readme")
	if err != nil {
		t.Fatalf("GetPageBlocksTree: %v", err)
	}
	if len(blocks) == 0 {
		t.Fatal("vault file produced no blocks")
	}

	// Memory hit still resolves with its page reference.
	got, err := c.GetBlock(ctx, blocks[0].UUID)
	if err != nil {
		t.Fatalf("GetBlock hit: %v", err)
	}
	if got == nil || got.UUID != blocks[0].UUID {
		t.Errorf("GetBlock hit = %+v, want block %q", got, blocks[0].UUID)
	}
	if got.Page == nil || got.Page.Name != "readme" {
		t.Errorf("GetBlock hit page = %+v, want reference to readme", got.Page)
	}

	// Miss returns (nil, nil) exactly as before — no store to consult and
	// no error.
	miss, err := c.GetBlock(ctx, "missing-uuid")
	if err != nil {
		t.Fatalf("GetBlock miss: %v", err)
	}
	if miss != nil {
		t.Errorf("GetBlock miss = %+v, want nil", miss)
	}
}

func TestGetBlock_RestoreIsNeverStale(t *testing.T) {
	c, s := newStoreBackedClient(t, nil)

	uuid := "gw-body"
	pageName := "compiled/gateway"

	// storeCompiled mirrors tools/compile.go StoreCompiled's replace
	// semantics: delete the page (CASCADE removes its blocks) and re-insert,
	// so the same page name is fully replaced.
	storeCompiled := func(content string) {
		t.Helper()
		_ = s.DeletePage(pageName) // absent on first call — expected
		if err := s.InsertPage(&store.Page{
			Name:         pageName,
			OriginalName: "gateway",
			SourceID:     "compiled",
			Properties:   `{"topic": "gateway", "tier": "draft"}`,
		}); err != nil {
			t.Fatalf("InsertPage: %v", err)
		}
		if err := s.InsertBlock(&store.Block{UUID: uuid, PageName: pageName, Content: content, Position: 0}); err != nil {
			t.Fatalf("InsertBlock: %v", err)
		}
	}

	ctx := context.Background()

	// v1 stored, then read at least once.
	storeCompiled("# Gateway v1")
	got, err := c.GetBlock(ctx, uuid)
	if err != nil {
		t.Fatalf("GetBlock v1: %v", err)
	}
	if got == nil || got.Content != "# Gateway v1" {
		t.Fatalf("v1 block = %+v, want content # Gateway v1", got)
	}

	// Re-store as v2 in the same session. The live lookup must return v2 —
	// no mid-session cache can serve the stale v1 (spec: Freshness of Store
	// Reads).
	storeCompiled("# Gateway v2")

	got, err = c.GetBlock(ctx, uuid)
	if err != nil {
		t.Fatalf("GetBlock v2: %v", err)
	}
	if got == nil || got.Content != "# Gateway v2" {
		t.Fatalf("v2 block = %+v, want content # Gateway v2 (never stale)", got)
	}
}

func TestGetBlock_StoreErrorSurfaces(t *testing.T) {
	// A store failure must surface as an error, not be masked as a
	// not-found result (design D6 honest-error philosophy).
	c, s := newStoreBackedClient(t, nil)
	if err := s.Close(); err != nil {
		t.Fatalf("Close store: %v", err)
	}

	ctx := context.Background()

	if _, err := c.GetBlock(ctx, "learning-block-1"); err == nil {
		t.Error("GetBlock: expected store query error after store close, got nil")
	}
}

// --- GetAllPages store merge tests (mcp-internal-store-access task 2.4) ---

func TestGetAllPages_MergesStoreOnlyPages(t *testing.T) {
	// Vault file + store-only pages (same-session store_learning /
	// store_compiled shapes) + a disk-local mirror row of the vault file:
	// the merged enumeration must contain all of them exactly once
	// (spec: Unified Page Enumeration, list_pages scenario).
	c, s := newStoreBackedClient(t, map[string]string{
		"readme.md": "# Vault\n\nLocal page.\n",
	})

	// The indexing pipeline persists vault files under source "disk-local" —
	// a mirror row that must never surface as a second "readme" entry
	// (spec: No duplication of vault pages persisted to the store).
	insertStorePageWithBlocks(t, s,
		&store.Page{Name: "readme", OriginalName: "readme", SourceID: "disk-local", SourceDocID: "readme.md"},
		&store.Block{UUID: "readme-mirror", PageName: "readme", Content: "# Vault (mirror)", Position: 0},
	)

	learningPage := "learning/auth-20260101T000000-alice"
	insertStorePageWithBlocks(t, s,
		&store.Page{
			Name:         learningPage,
			OriginalName: learningPage,
			SourceID:     "learning",
			Properties:   `{"tag": "auth", "tier": "draft"}`,
			CreatedAt:    1000,
			UpdatedAt:    2000,
		},
		&store.Block{UUID: "learning-block-1", PageName: learningPage, Content: "Use parameterized SQL.", Position: 0},
	)

	compiledPage := "compiled/gateway"
	insertStorePageWithBlocks(t, s,
		&store.Page{
			Name:         compiledPage,
			OriginalName: "gateway",
			SourceID:     "compiled",
			Properties:   `{"topic": "gateway", "tier": "draft"}`,
		},
		&store.Block{UUID: "gw-root", PageName: compiledPage, Content: "# Gateway", Position: 0},
	)

	ctx := context.Background()
	pages, err := c.GetAllPages(ctx)
	if err != nil {
		t.Fatalf("GetAllPages: %v", err)
	}

	counts := make(map[string]int)
	var learningEntity, compiledEntity *types.PageEntity
	for i := range pages {
		counts[pages[i].Name]++
		switch pages[i].Name {
		case learningPage:
			learningEntity = &pages[i]
		case compiledPage:
			compiledEntity = &pages[i]
		}
	}

	if counts["readme"] != 1 {
		t.Errorf("readme appears %d times, want exactly 1 (disk-local mirror row must not duplicate)", counts["readme"])
	}
	if counts[learningPage] != 1 {
		t.Errorf("%s appears %d times, want exactly 1", learningPage, counts[learningPage])
	}
	if counts[compiledPage] != 1 {
		t.Errorf("%s appears %d times, want exactly 1", compiledPage, counts[compiledPage])
	}
	if got := len(pages); got != 3 {
		t.Errorf("total pages = %d, want 3 (readme + learning + compiled): %+v", got, pages)
	}

	if learningEntity == nil {
		t.Fatal("learning page missing from merged enumeration")
	}
	if learningEntity.OriginalName != learningPage {
		t.Errorf("learning OriginalName = %q, want %q", learningEntity.OriginalName, learningPage)
	}
	if learningEntity.Properties["tag"] != "auth" || learningEntity.Properties["tier"] != "draft" {
		t.Errorf("learning Properties = %v, want tag=auth tier=draft", learningEntity.Properties)
	}
	if learningEntity.CreatedAt != 1000 || learningEntity.UpdatedAt != 2000 {
		t.Errorf("learning timestamps = (%d, %d), want (1000, 2000)", learningEntity.CreatedAt, learningEntity.UpdatedAt)
	}

	if compiledEntity == nil {
		t.Fatal("compiled page missing from merged enumeration")
	}
	if compiledEntity.OriginalName != "gateway" {
		t.Errorf("compiled OriginalName = %q, want %q", compiledEntity.OriginalName, "gateway")
	}
	if compiledEntity.Properties["topic"] != "gateway" {
		t.Errorf("compiled Properties = %v, want topic=gateway", compiledEntity.Properties)
	}

	// The merge must not cache into c.pages — the in-memory index stays
	// file-only, which is what keeps later re-stores fresh (design D2).
	c.mu.RLock()
	inMemory := len(c.pages)
	c.mu.RUnlock()
	if inMemory != 1 {
		t.Errorf("len(c.pages) = %d, want 1 (store pages must not be cached into the in-memory index)", inMemory)
	}
}

func TestGetAllPages_NoDuplicatesForVaultMirroredRows(t *testing.T) {
	// A page that exists both as a vault file and as a store row from a
	// non-vault source must appear exactly once, with the vault file's
	// entity winning (spec: Unified Page Enumeration — no duplication;
	// Vault Precedence on Name Collision).
	c, s := newStoreBackedClient(t, map[string]string{
		"notes/topic.md": "# Topic\n\nFrom the vault file.\n",
	})

	insertStorePageWithBlocks(t, s,
		&store.Page{
			Name:         "notes/topic",
			OriginalName: "Notes/Topic From Store",
			SourceID:     "github-org",
			Properties:   `{"origin": "external"}`,
		},
		&store.Block{UUID: "ext-topic-root", PageName: "notes/topic", Content: "# From the store", Position: 0},
	)
	// Case-variant row: the dedupe key is the lowercased name (design D3),
	// so this row must not surface either.
	insertStorePageWithBlocks(t, s,
		&store.Page{Name: "NOTES/TOPIC", OriginalName: "NOTES/TOPIC", SourceID: "web-crawl"},
	)

	ctx := context.Background()
	pages, err := c.GetAllPages(ctx)
	if err != nil {
		t.Fatalf("GetAllPages: %v", err)
	}

	var topic []types.PageEntity
	for _, p := range pages {
		if strings.EqualFold(p.Name, "notes/topic") {
			topic = append(topic, p)
		}
	}
	if len(topic) != 1 {
		t.Fatalf("notes/topic appears %d times, want exactly 1: %+v", len(topic), pages)
	}
	if len(pages) != 1 {
		t.Errorf("total pages = %d, want 1 (vault file only; mirrored rows deduped): %+v", len(pages), pages)
	}
	// The vault file wins: it has no frontmatter, so the store row's
	// properties and original name must not leak.
	if topic[0].OriginalName != "notes/topic" {
		t.Errorf("OriginalName = %q, want %q (vault file entity must win)", topic[0].OriginalName, "notes/topic")
	}
	if topic[0].Properties != nil {
		t.Errorf("Properties = %v, want nil (store row properties must not leak)", topic[0].Properties)
	}
}

func TestGetAllPages_NoStoreParity(t *testing.T) {
	// No WithStore option: the enumeration must return the in-memory file
	// snapshot exactly as before (Constitution II — inherited-tool
	// compatibility), and a store containing no non-local rows must produce
	// the identical output (the merge layer is inert when it has nothing
	// to add).
	dir := t.TempDir()
	files := map[string]string{
		"readme.md":      "# Vault\n\nLocal page.\n",
		"notes/topic.md": "# Topic\n\nFile content.\n",
	}
	for rel, content := range files {
		abs := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", abs, err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%s): %v", abs, err)
		}
	}

	c := New(dir)
	if err := c.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	ctx := context.Background()
	pages, err := c.GetAllPages(ctx)
	if err != nil {
		t.Fatalf("GetAllPages: %v", err)
	}
	if len(pages) != 2 {
		t.Fatalf("pages = %d, want 2: %+v", len(pages), pages)
	}
	names := make(map[string]int)
	for _, p := range pages {
		names[p.Name]++
	}
	for _, want := range []string{"readme", "notes/topic"} {
		if names[want] != 1 {
			t.Errorf("page %q appears %d times, want exactly 1", want, names[want])
		}
	}

	// Same vault directory with an (empty) persistent store attached: the
	// result must be identical — the merge appends nothing.
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New(:memory:): %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	c2 := New(dir, WithStore(s))
	if err := c2.Load(); err != nil {
		t.Fatalf("Load (store-backed): %v", err)
	}
	pages2, err := c2.GetAllPages(ctx)
	if err != nil {
		t.Fatalf("GetAllPages (store-backed): %v", err)
	}

	// Map-ify to normalize map-iteration order before comparing.
	toMap := func(ps []types.PageEntity) map[string]types.PageEntity {
		m := make(map[string]types.PageEntity, len(ps))
		for _, p := range ps {
			m[p.Name] = p
		}
		return m
	}
	if !reflect.DeepEqual(toMap(pages), toMap(pages2)) {
		t.Errorf("store-backed (empty store) output %+v != no-store output %+v", toMap(pages2), toMap(pages))
	}
}

func TestGetAllPages_StoreErrorSurfaces(t *testing.T) {
	// A store failure must surface as an error, not be masked as an empty
	// or memory-only enumeration (design honest-error philosophy).
	c, s := newStoreBackedClient(t, nil)
	if err := s.Close(); err != nil {
		t.Fatalf("Close store: %v", err)
	}

	if _, err := c.GetAllPages(context.Background()); err == nil {
		t.Error("GetAllPages: expected store query error after store close, got nil")
	}
}

// --- FullTextSearch store merge tests (mcp-internal-store-access task 2.5) ---

func TestFullTextSearch_StoreHitSameSession(t *testing.T) {
	// A store_compiled-shaped page written after the client was created: it
	// exists only in the store, so only the live merge can surface it without
	// a restart (spec: Unified Full-Text Search — same-session compiled
	// article scenario).
	c, s := newStoreBackedClient(t, map[string]string{
		"readme.md": "# Vault\n\nLocal page.\n",
	})
	// Production wiring always builds the search index after Load (main.go);
	// BuildBacklinks is what populates it.
	c.BuildBacklinks()

	insertStorePageWithBlocks(t, s,
		&store.Page{
			Name:         "compiled/gateway",
			OriginalName: "gateway",
			SourceID:     "compiled",
			Properties:   `{"topic": "gateway", "tier": "draft"}`,
		},
		&store.Block{UUID: "gw-root", PageName: "compiled/gateway", Content: "# Gateway", HeadingLevel: 1, Position: 0},
		&store.Block{UUID: "gw-body", PageName: "compiled/gateway", Content: "Body text with unique-phrase.", Position: 1},
	)

	hits, err := c.FullTextSearch(context.Background(), "unique-phrase", 10)
	if err != nil {
		t.Fatalf("FullTextSearch: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("hits = %d, want 1: %+v", len(hits), hits)
	}
	if hits[0].PageName != "compiled/gateway" {
		t.Errorf("PageName = %q, want %q", hits[0].PageName, "compiled/gateway")
	}
	if hits[0].UUID != "gw-body" {
		t.Errorf("UUID = %q, want %q (the matching store block)", hits[0].UUID, "gw-body")
	}
	if !strings.Contains(hits[0].Content, "unique-phrase") {
		t.Errorf("Content = %q, want the stored block content", hits[0].Content)
	}
}

func TestFullTextSearch_DedupesMirroredPages(t *testing.T) {
	// The vault file notes/topic.md matches in memory; the store carries a
	// same-named page from another source plus a store-only page. The mirrored
	// rows must never surface twice (vault-file precedence, design D3), while
	// the store-only page — and each of its matching blocks — still appears
	// (per-block semantics, identical to a restart + LoadExternalPages).
	c, s := newStoreBackedClient(t, map[string]string{
		"notes/topic.md": "# Topic\n\nvault file content with mirror-phrase\n",
	})
	c.BuildBacklinks()

	insertStorePageWithBlocks(t, s,
		&store.Page{Name: "notes/topic", OriginalName: "notes/topic", SourceID: "github-org"},
		&store.Block{UUID: "mirror-row-1", PageName: "notes/topic", Content: "store-side mirror-phrase row one", Position: 0},
		&store.Block{UUID: "mirror-row-2", PageName: "notes/topic", Content: "store-side mirror-phrase row two", Position: 1},
	)
	insertStorePageWithBlocks(t, s,
		&store.Page{Name: "web-crawl/external", SourceID: "web-crawl"},
		&store.Block{UUID: "ext-1", PageName: "web-crawl/external", Content: "external mirror-phrase block one", Position: 0},
		&store.Block{UUID: "ext-2", PageName: "web-crawl/external", Content: "external mirror-phrase block two", Position: 1},
	)

	hits, err := c.FullTextSearch(context.Background(), "mirror-phrase", 10)
	if err != nil {
		t.Fatalf("FullTextSearch: %v", err)
	}

	var topicHits, externalHits int
	for _, h := range hits {
		switch h.PageName {
		case "notes/topic":
			topicHits++
			if !strings.Contains(h.Content, "vault file content") {
				t.Errorf("notes/topic hit content = %q, want the vault file version (store rows must not leak)", h.Content)
			}
		case "web-crawl/external":
			externalHits++
		default:
			t.Errorf("unexpected hit for page %q: %+v", h.PageName, h)
		}
	}
	if topicHits != 1 {
		t.Errorf("hits for notes/topic = %d, want exactly 1 (mirrored store rows deduplicated)", topicHits)
	}
	if externalHits != 2 {
		t.Errorf("hits for web-crawl/external = %d, want 2 (per-block store hits, not page-deduplicated)", externalHits)
	}
	if len(hits) != 3 {
		t.Errorf("total hits = %d, want 3 (1 vault + 2 store-only blocks)", len(hits))
	}
}

func TestFullTextSearch_RespectsLimitAcrossMerge(t *testing.T) {
	// Two vault-file blocks match in memory; three store-only pages match in
	// the store. The merged result must never exceed the caller's limit, and
	// when the in-memory hits already fill it, store rows must not appear.
	c, s := newStoreBackedClient(t, map[string]string{
		"alpha.md": "# Alpha\n\nalpha limit-needle line\n",
		"beta.md":  "# Beta\n\nbeta limit-needle line\n",
	})
	c.BuildBacklinks()

	storePages := []string{"compiled/store-a", "compiled/store-b", "compiled/store-c"}
	for i, name := range storePages {
		insertStorePageWithBlocks(t, s,
			// SourceDocID must be distinct per page: (source_id, source_doc_id)
			// is UNIQUE in the pages schema.
			&store.Page{Name: name, SourceID: "compiled", SourceDocID: fmt.Sprintf("doc-%d", i)},
			&store.Block{
				UUID:     fmt.Sprintf("store-block-%d", i),
				PageName: name,
				Content:  fmt.Sprintf("store %s limit-needle block", name),
				Position: 0,
			},
		)
	}

	ctx := context.Background()

	// limit=3: 2 in-memory hits + 1 store hit fill the merged set exactly.
	hits, err := c.FullTextSearch(ctx, "limit-needle", 3)
	if err != nil {
		t.Fatalf("FullTextSearch (limit 3): %v", err)
	}
	if len(hits) != 3 {
		t.Fatalf("hits = %d, want 3: %+v", len(hits), hits)
	}
	pages := searchHitPages(hits)
	if !pages["alpha"] || !pages["beta"] {
		t.Errorf("merged set missing in-memory pages: %v", pages)
	}
	if !pages["compiled/store-a"] {
		t.Errorf("merged set missing the first store page (store orders by page name): %v", pages)
	}

	// limit=2: the in-memory hits already fill the limit — no store rows.
	hits, err = c.FullTextSearch(ctx, "limit-needle", 2)
	if err != nil {
		t.Fatalf("FullTextSearch (limit 2): %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("hits = %d, want 2: %+v", len(hits), hits)
	}
	pages = searchHitPages(hits)
	if !pages["alpha"] || !pages["beta"] {
		t.Errorf("limit-2 set must be exactly the in-memory pages: %v", pages)
	}

	// limit=10: everything fits.
	hits, err = c.FullTextSearch(ctx, "limit-needle", 10)
	if err != nil {
		t.Fatalf("FullTextSearch (limit 10): %v", err)
	}
	if len(hits) != 5 {
		t.Errorf("hits = %d, want 5 (2 in-memory + 3 store)", len(hits))
	}

	// limit<=0 mirrors the in-memory normalization (cap 20): with more than
	// 20 matches available the merged set is still capped at 20, never
	// unbounded.
	for i := 3; i < 25; i++ {
		name := fmt.Sprintf("compiled/store-%02d", i)
		insertStorePageWithBlocks(t, s,
			&store.Page{Name: name, SourceID: "compiled", SourceDocID: fmt.Sprintf("doc-%d", i)},
			&store.Block{
				UUID:     fmt.Sprintf("store-block-%d", i),
				PageName: name,
				Content:  "overflow limit-needle block",
				Position: 0,
			},
		)
	}
	hits, err = c.FullTextSearch(ctx, "limit-needle", 0)
	if err != nil {
		t.Fatalf("FullTextSearch (limit 0): %v", err)
	}
	if len(hits) != 20 {
		t.Errorf("hits with limit 0 = %d, want 20 (normalized cap)", len(hits))
	}
}

func TestFullTextSearch_EmptyQueryNoStoreHits(t *testing.T) {
	// An empty or whitespace-only query must return no hits even with a
	// store configured: SearchStoreBlocks("") would otherwise run LIKE '%%'
	// and match every block up to the limit, while a store-less client
	// returns nothing (the in-memory index tokenizes to zero terms) —
	// asymmetric behavior the store-merge gate eliminates.
	c, s := newStoreBackedClient(t, map[string]string{
		"readme.md": "# Vault\n\nLocal page.\n",
	})
	c.BuildBacklinks()

	insertStorePageWithBlocks(t, s,
		&store.Page{
			Name:         "compiled/gateway",
			OriginalName: "gateway",
			SourceID:     "compiled",
			Properties:   `{"topic": "gateway"}`,
		},
		&store.Block{UUID: "gw-root", PageName: "compiled/gateway", Content: "# Gateway", HeadingLevel: 1, Position: 0},
		&store.Block{UUID: "gw-body", PageName: "compiled/gateway", Content: "Body text.", Position: 1},
	)

	ctx := context.Background()
	for _, query := range []string{"", "   "} {
		hits, err := c.FullTextSearch(ctx, query, 10)
		if err != nil {
			t.Fatalf("FullTextSearch(%q): %v", query, err)
		}
		if len(hits) != 0 {
			t.Errorf("FullTextSearch(%q) = %d hits, want 0 (no arbitrary store blocks): %+v", query, len(hits), hits)
		}
	}

	// The same client still merges store hits for a real query — the gate
	// affects blank queries only.
	hits, err := c.FullTextSearch(ctx, "body", 10)
	if err != nil {
		t.Fatalf("FullTextSearch(body): %v", err)
	}
	if len(hits) != 1 || hits[0].PageName != "compiled/gateway" || hits[0].UUID != "gw-body" {
		t.Errorf("FullTextSearch(body) = %+v, want the gw-body hit from compiled/gateway", hits)
	}
}

func TestFullTextSearch_NoStoreParity(t *testing.T) {
	// Without a store the results must be exactly the pre-change in-memory
	// output; attaching an empty store over the same vault must produce the
	// identical set (the merge layer is inert when the store has no matches).
	dir := t.TempDir()
	files := map[string]string{
		"readme.md":      "# Vault\n\nparity-needle readme line\n",
		"notes/topic.md": "# Topic\n\nparity-needle topic line\n",
	}
	for rel, content := range files {
		abs := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", abs, err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%s): %v", abs, err)
		}
	}

	ctx := context.Background()

	c1 := New(dir)
	if err := c1.Load(); err != nil {
		t.Fatalf("Load (no store): %v", err)
	}
	c1.BuildBacklinks()
	hits1, err := c1.FullTextSearch(ctx, "parity-needle", 10)
	if err != nil {
		t.Fatalf("FullTextSearch (no store): %v", err)
	}

	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New(:memory:): %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	c2 := New(dir, WithStore(s))
	if err := c2.Load(); err != nil {
		t.Fatalf("Load (store-backed): %v", err)
	}
	c2.BuildBacklinks()
	hits2, err := c2.FullTextSearch(ctx, "parity-needle", 10)
	if err != nil {
		t.Fatalf("FullTextSearch (store-backed): %v", err)
	}

	// Compare as (page, content) key sets: block UUIDs are generated per
	// client and hit order follows map iteration, neither stable across
	// clients.
	if !reflect.DeepEqual(searchHitKeySet(hits1), searchHitKeySet(hits2)) {
		t.Errorf("store-backed (empty store) hits %v != no-store hits %v",
			searchHitKeySet(hits2), searchHitKeySet(hits1))
	}

	// After a store-only page is written, the store-backed client surfaces it
	// while the no-store client's results are unchanged (Constitution II —
	// inherited-tool behavior identical without persistence).
	insertStorePageWithBlocks(t, s,
		&store.Page{Name: "compiled/store-only", SourceID: "compiled"},
		&store.Block{UUID: "store-only-block", PageName: "compiled/store-only", Content: "parity-needle store-only block", Position: 0},
	)

	hits2b, err := c2.FullTextSearch(ctx, "parity-needle", 10)
	if err != nil {
		t.Fatalf("FullTextSearch (after store write): %v", err)
	}
	if !searchHitPages(hits2b)["compiled/store-only"] {
		t.Errorf("store-backed client must surface the store-only page: %v", searchHitPages(hits2b))
	}

	hits1b, err := c1.FullTextSearch(ctx, "parity-needle", 10)
	if err != nil {
		t.Fatalf("FullTextSearch (no store, after write): %v", err)
	}
	if searchHitPages(hits1b)["compiled/store-only"] {
		t.Error("no-store client must never surface store-only content")
	}
	if !reflect.DeepEqual(searchHitKeySet(hits1), searchHitKeySet(hits1b)) {
		t.Errorf("no-store results changed after an unrelated store write: %v -> %v",
			searchHitKeySet(hits1), searchHitKeySet(hits1b))
	}
}

func TestFullTextSearch_StoreErrorSurfaces(t *testing.T) {
	// A store failure must surface as an error, never be masked as empty or
	// memory-only results (design honest-error philosophy, matching the
	// GetAllPages error contract).
	c, s := newStoreBackedClient(t, nil)

	// Closing the store makes every query fail deterministically.
	if err := s.Close(); err != nil {
		t.Fatalf("Close store: %v", err)
	}

	// No vault files → no in-memory hits → the store query is attempted.
	if _, err := c.FullTextSearch(context.Background(), "anything", 10); err == nil {
		t.Error("FullTextSearch: expected store query error after store close, got nil")
	}
}

// searchHitPages maps lowercased page names to presence in a hit set.
func searchHitPages(hits []backend.SearchHit) map[string]bool {
	pages := make(map[string]bool, len(hits))
	for _, h := range hits {
		pages[strings.ToLower(h.PageName)] = true
	}
	return pages
}

// searchHitKeySet builds a "page\x00content" key set for order-insensitive
// hit-set comparison.
func searchHitKeySet(hits []backend.SearchHit) map[string]bool {
	set := make(map[string]bool, len(hits))
	for _, h := range hits {
		set[h.PageName+"\x00"+h.Content] = true
	}
	return set
}

// --- Reload store-page preservation tests (mcp-internal-store-access task 2.7) ---

// TestReload_PreservesStoreBackedPages verifies that Reload re-runs the
// external-page load after re-reading files. The assertions inspect c.pages
// directly: a GetPage call would also succeed without the fix via the live
// store fallback (task 2.2), so only the in-memory index proves reload
// rebuilt the store-backed entries (spec: Reload Preserves Store-Backed
// Pages).
func TestReload_PreservesStoreBackedPages(t *testing.T) {
	c, s := newStoreBackedClient(t, map[string]string{
		"readme.md": "# Vault\n\nLocal page.\n",
	})

	// A learning page living only in the store — the store_learning shape
	// the startup path hydrates via LoadExternalPages.
	pageName := "learning/auth-20260101T000000-alice"
	insertStorePageWithBlocks(t, s,
		&store.Page{
			Name:         pageName,
			OriginalName: pageName,
			SourceID:     "learning",
			SourceDocID:  "auth-20260101T000000-alice.md",
			Properties:   `{"tag": "auth"}`,
		},
		&store.Block{UUID: "learning-reload-root", PageName: pageName, Content: "Use parameterized SQL.", Position: 0},
	)

	// Simulate the startup load so the store-backed page is in the
	// in-memory index before reload — the pre-reload read view the spec
	// pins reload to preserve.
	vs := c.Store()
	if vs == nil {
		t.Fatal("test precondition: vaultStore must be configured")
	}
	if _, err := vs.LoadExternalPages(c); err != nil {
		t.Fatalf("LoadExternalPages: %v", err)
	}

	c.mu.RLock()
	_, preReload := c.pages[strings.ToLower(pageName)]
	c.mu.RUnlock()
	if !preReload {
		t.Fatal("test precondition: LoadExternalPages did not populate c.pages")
	}

	if err := c.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	// Assert on the in-memory index itself: Reload must have re-run the
	// external-page load, not left the store page to the live fallback.
	c.mu.RLock()
	storePage, storePagePresent := c.pages[strings.ToLower(pageName)]
	storePageSourceID, storePageReadOnly := "", false
	if storePagePresent {
		storePageSourceID, storePageReadOnly = storePage.sourceID, storePage.readOnly
	}
	_, diskPagePresent := c.pages["readme"]
	c.mu.RUnlock()
	if !storePagePresent {
		t.Errorf("store-backed page %q missing from c.pages after Reload", pageName)
	} else {
		if storePageSourceID != "learning" {
			t.Errorf("reloaded store page sourceID = %q, want %q", storePageSourceID, "learning")
		}
		if !storePageReadOnly {
			t.Error("reloaded store page readOnly = false, want true")
		}
	}
	// Reload must still have done its normal job: re-read disk files.
	if !diskPagePresent {
		t.Error("disk vault page \"readme\" missing from c.pages after Reload")
	}
}

// TestReload_NoStoreConfigured verifies the no-store parity path: without
// persistence, Reload behaves exactly as before — files re-read, backlinks
// rebuilt, no external-page load attempted, no panic (Constitution II —
// inherited-tool compatibility).
func TestReload_NoStoreConfigured(t *testing.T) {
	c := testWritableVault(t)

	if c.vaultStore != nil {
		t.Fatal("test precondition: vaultStore must be nil without WithStore")
	}

	if err := c.Reload(); err != nil {
		t.Fatalf("Reload with no store: %v", err)
	}

	// Reload must still have re-read the vault files.
	pages, err := c.GetAllPages(context.Background())
	if err != nil {
		t.Fatalf("GetAllPages after reload: %v", err)
	}
	if len(pages) == 0 {
		t.Error("no pages after Reload — the file sweep did not run")
	}
}
