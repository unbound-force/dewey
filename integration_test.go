// PARALLEL SAFETY: Tests in this file MUST NOT use t.Parallel().
// They mutate process-global state via os.Chdir (working directory).
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	_ "github.com/unbound-force/dewey/chunker" // Register Go chunker for code source tests.
	"github.com/unbound-force/dewey/curate"
	"github.com/unbound-force/dewey/llm"
	"github.com/unbound-force/dewey/source"
	"github.com/unbound-force/dewey/store"
	"github.com/unbound-force/dewey/tools"
	"github.com/unbound-force/dewey/types"
	"github.com/unbound-force/dewey/vault"
)

// TestEndToEnd_InitIndexStatusFlow verifies the complete workflow:
// dewey init → dewey index (fixture vault) → store queries → dewey status.
// This is the integration test for T065.
func TestEndToEnd_InitIndexStatusFlow(t *testing.T) {
	tmpDir := t.TempDir()

	// Step 1: Create test vault with .md files.
	testFiles := map[string]string{
		"setup.md":                  "# Setup\n\n## Installation\n\nRun go install to set up.\n\n## Configuration\n\nEdit config.yaml for settings.",
		"api-guide.md":              "# API Guide\n\n## REST API\n\nThe API supports GET and POST methods.\n\nSee [[setup]] for installation.",
		"daily notes/2026-03-22.md": "# March 22\n\nToday's journal entry.",
	}
	for name, content := range testFiles {
		path := filepath.Join(tmpDir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write test file %s: %v", name, err)
		}
	}

	// Step 2: dewey init
	initCmd := newInitCmd()
	initCmd.SetArgs([]string{"--vault", tmpDir})
	if err := initCmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// Verify .uf/dewey/ was created.
	deweyDir := filepath.Join(tmpDir, deweyWorkspaceDir)
	if _, err := os.Stat(deweyDir); os.IsNotExist(err) {
		t.Fatal(".uf/dewey/ directory was not created")
	}

	// Verify config.yaml exists.
	if _, err := os.Stat(filepath.Join(deweyDir, "config.yaml")); os.IsNotExist(err) {
		t.Fatal("config.yaml was not created")
	}

	// Step 3: dewey index (with disk source pointing to tmpDir)
	// Update sources.yaml to point to the test vault.
	sourcesContent := `sources:
  - id: disk-local
    type: disk
    name: local
    config:
      path: "` + tmpDir + `"
`
	if err := os.WriteFile(filepath.Join(deweyDir, "sources.yaml"), []byte(sourcesContent), 0o644); err != nil {
		t.Fatalf("write sources.yaml: %v", err)
	}

	oldDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(oldDir) }()

	indexCmd := newIndexCmd()
	// Pass --no-embeddings because Ollama is not running in test env.
	indexCmd.SetArgs([]string{"--no-embeddings"})
	if err := indexCmd.Execute(); err != nil {
		t.Fatalf("index failed: %v", err)
	}

	// Step 4: Verify store has indexed pages.
	// Open the store, verify, then close it before running status commands
	// which also need an exclusive lock on the database.
	dbPath := filepath.Join(deweyDir, "graph.db")
	func() {
		s, err := store.New(dbPath)
		if err != nil {
			t.Fatalf("open store: %v", err)
		}
		defer func() { _ = s.Close() }()

		pages, err := s.ListPages()
		if err != nil {
			t.Fatalf("list pages: %v", err)
		}
		if len(pages) != 3 {
			t.Errorf("expected 3 pages, got %d", len(pages))
		}

		// Verify page provenance.
		for _, p := range pages {
			if p.SourceID != "disk-local" {
				t.Errorf("page %q source_id = %q, want %q", p.Name, p.SourceID, "disk-local")
			}
			if p.ContentHash == "" {
				t.Errorf("page %q has empty content_hash", p.Name)
			}
		}
	}()

	// Step 5: dewey status (text output)
	statusCmd := newStatusCmd()
	statusBuf := new(strings.Builder)
	statusCmd.SetOut(statusBuf)
	if err := statusCmd.Execute(); err != nil {
		t.Fatalf("status failed: %v", err)
	}

	statusOutput := statusBuf.String()
	if !strings.Contains(statusOutput, "Dewey Index Status") {
		t.Error("status output missing header")
	}
	if !strings.Contains(statusOutput, "Pages:") {
		t.Error("status output missing Pages")
	}

	// Step 6: dewey status --json
	statusJSONCmd := newStatusCmd()
	jsonBuf := new(strings.Builder)
	statusJSONCmd.SetOut(jsonBuf)
	statusJSONCmd.SetArgs([]string{"--json"})
	if err := statusJSONCmd.Execute(); err != nil {
		t.Fatalf("status --json failed: %v", err)
	}

	var jsonResult map[string]any
	if err := json.Unmarshal([]byte(jsonBuf.String()), &jsonResult); err != nil {
		t.Fatalf("invalid JSON status output: %v", err)
	}
	if _, ok := jsonResult["pages"]; !ok {
		t.Error("JSON status missing 'pages' field")
	}

	// Step 7: Verify source config round-trip.
	configs, err := source.LoadSourcesConfig(filepath.Join(deweyDir, "sources.yaml"))
	if err != nil {
		t.Fatalf("load sources config: %v", err)
	}
	if len(configs) != 1 {
		t.Errorf("expected 1 source config, got %d", len(configs))
	}
	if configs[0].ID != "disk-local" {
		t.Errorf("source id = %q, want %q", configs[0].ID, "disk-local")
	}
}

// TestEndToEnd_SourceAddAndIndex verifies adding a source and indexing it.
func TestEndToEnd_SourceAddAndIndex(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize.
	deweyDir := filepath.Join(tmpDir, deweyWorkspaceDir)
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	sourcesContent := `sources:
  - id: disk-local
    type: disk
    name: local
    config:
      path: "."
`
	if err := os.WriteFile(filepath.Join(deweyDir, "sources.yaml"), []byte(sourcesContent), 0o644); err != nil {
		t.Fatalf("write sources.yaml: %v", err)
	}

	oldDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(oldDir) }()

	// Add a web source.
	sourceCmd := newSourceCmd()
	sourceCmd.SetArgs([]string{"add", "web", "--url", "https://example.com", "--name", "example", "--depth", "0"})
	if err := sourceCmd.Execute(); err != nil {
		t.Fatalf("source add failed: %v", err)
	}

	// Verify source was added.
	configs, err := source.LoadSourcesConfig(filepath.Join(deweyDir, "sources.yaml"))
	if err != nil {
		t.Fatalf("load sources config: %v", err)
	}
	if len(configs) != 2 {
		t.Errorf("expected 2 source configs, got %d", len(configs))
	}

	foundWeb := false
	for _, cfg := range configs {
		if cfg.Type == "web" && cfg.ID == "web-example" {
			foundWeb = true
		}
	}
	if !foundWeb {
		t.Error("web source not found in config")
	}
}

// TestEndToEnd_ExternalPagesSurviveServeStartup verifies that external-source
// pages stored by `dewey index` are NOT deleted when the vault performs its
// incremental index during `dewey serve` startup.
//
// This is the regression test for the root cause identified in v1.3.1:
// VaultStore.LoadPages() was returning ALL pages (including external sources).
// IncrementalIndex() treated external pages as "deleted" because they had no
// corresponding .md files on disk, purging them from graph.db before
// LoadExternalPages() could load them.
func TestEndToEnd_ExternalPagesSurviveServeStartup(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a local .md file.
	if err := os.WriteFile(filepath.Join(tmpDir, "local-page.md"), []byte("# Local Page\n\nLocal content."), 0o644); err != nil {
		t.Fatalf("write local .md: %v", err)
	}

	// Initialize .uf/dewey/ and create store.
	deweyDir := filepath.Join(tmpDir, deweyWorkspaceDir)
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	dbPath := filepath.Join(deweyDir, "graph.db")
	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}

	// Insert external-source pages (simulating what dewey index creates).
	for _, name := range []string{"github-org/issues/1", "github-org/issues/2", "disk-gaze/readme.md"} {
		if err := s.InsertPage(&store.Page{
			Name:        name,
			SourceID:    strings.SplitN(name, "/", 2)[0],
			SourceDocID: strings.SplitN(name, "/", 2)[1],
			ContentHash: "hash-" + name,
			CreatedAt:   1000,
			UpdatedAt:   1000,
		}); err != nil {
			t.Fatalf("insert external page %q: %v", name, err)
		}
		// Insert a block so the page has content.
		if err := s.InsertBlock(&store.Block{
			UUID:     "block-" + name,
			PageName: name,
			Content:  "External content for " + name,
		}); err != nil {
			t.Fatalf("insert block for %q: %v", name, err)
		}
	}
	_ = s.Close()

	// Now simulate what dewey serve does: create vault with store,
	// run incremental index, then load external pages.
	s2, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer func() { _ = s2.Close() }()

	vc := vault.New(tmpDir, vault.WithStore(s2))
	vs := vc.Store()

	// Run incremental index (this is what was deleting external pages).
	stats, err := vs.IncrementalIndex(vc)
	if err != nil {
		t.Fatalf("IncrementalIndex: %v", err)
	}

	// The incremental index should only process local .md files.
	// It must NOT delete the 3 external pages.
	if stats.Deleted != 0 {
		t.Errorf("IncrementalIndex deleted %d pages, want 0 (external pages should be preserved)", stats.Deleted)
	}

	// Verify external pages still exist in the store.
	for _, name := range []string{"github-org/issues/1", "github-org/issues/2", "disk-gaze/readme.md"} {
		page, err := s2.GetPage(name)
		if err != nil || page == nil {
			t.Errorf("external page %q was deleted by IncrementalIndex — this is the v1.3.1 regression", name)
		}
	}

	// Load external pages into vault.
	extCount, err := vs.LoadExternalPages(vc)
	if err != nil {
		t.Fatalf("LoadExternalPages: %v", err)
	}
	if extCount != 3 {
		t.Errorf("LoadExternalPages loaded %d pages, want 3", extCount)
	}
}

// TestEndToEnd_MultiSourceIdenticalFiles verifies that indexing multiple
// disk sources with identical files (e.g., scaffolded AGENTS.md) does not
// produce UUID collisions. Each source's pages should have unique block UUIDs.
//
// This is the regression test for GitHub issue #17.
func TestEndToEnd_MultiSourceIdenticalFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create two "repos" with identical files.
	repoA := filepath.Join(tmpDir, "repo-a")
	repoB := filepath.Join(tmpDir, "repo-b")
	for _, dir := range []string{repoA, repoB} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		// Same content, same filename — would collide without namespaced UUIDs.
		if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# AGENTS\n\n## Project Overview\n\nScaffolded file."), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	// Initialize dewey.
	deweyDir := filepath.Join(tmpDir, deweyWorkspaceDir)
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("mkdir .uf/dewey: %v", err)
	}
	sourcesYAML := `sources:
  - id: disk-repo-a
    type: disk
    name: repo-a
    config:
      path: "` + repoA + `"
  - id: disk-repo-b
    type: disk
    name: repo-b
    config:
      path: "` + repoB + `"
`
	if err := os.WriteFile(filepath.Join(deweyDir, "sources.yaml"), []byte(sourcesYAML), 0o644); err != nil {
		t.Fatalf("write sources: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deweyDir, "config.yaml"), []byte("embedding:\n  model: test\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Run index.
	oldDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(oldDir) }()

	indexCmd := newIndexCmd()
	indexCmd.SetArgs([]string{"--no-embeddings"})
	if err := indexCmd.Execute(); err != nil {
		t.Fatalf("index failed: %v", err)
	}

	// Verify both sources have pages with blocks (no UUID collisions).
	dbPath := filepath.Join(deweyDir, "graph.db")
	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = s.Close() }()

	pagesA, err := s.ListPagesBySource("disk-repo-a")
	if err != nil {
		t.Fatalf("ListPagesBySource(disk-repo-a): %v", err)
	}
	pagesB, err := s.ListPagesBySource("disk-repo-b")
	if err != nil {
		t.Fatalf("ListPagesBySource(disk-repo-b): %v", err)
	}

	if len(pagesA) == 0 {
		t.Error("disk-repo-a has 0 pages")
	}
	if len(pagesB) == 0 {
		t.Error("disk-repo-b has 0 pages")
	}

	// Verify both have blocks (UUID collision would prevent block insertion
	// for the second source).
	blocksA, err := s.GetBlocksByPage("disk-repo-a/agents.md")
	if err != nil {
		t.Fatalf("GetBlocksByPage(disk-repo-a/agents.md): %v", err)
	}
	blocksB, err := s.GetBlocksByPage("disk-repo-b/agents.md")
	if err != nil {
		t.Fatalf("GetBlocksByPage(disk-repo-b/agents.md): %v", err)
	}

	if len(blocksA) == 0 {
		t.Error("disk-repo-a/agents.md has 0 blocks — UUID collision likely")
	}
	if len(blocksB) == 0 {
		t.Error("disk-repo-b/agents.md has 0 blocks — UUID collision likely")
	}

	// Verify UUIDs are unique across sources.
	if len(blocksA) > 0 && len(blocksB) > 0 {
		if blocksA[0].UUID == blocksB[0].UUID {
			t.Errorf("block UUIDs collide across sources: %s", blocksA[0].UUID)
		}
	}
}

// TestEndToEnd_GitignoreRespected verifies the full pipeline: a vault with a
// .gitignore file excludes matching directories from the in-memory index.
// Files inside ignored directories (e.g., node_modules/) must not appear as
// pages, while non-ignored files must be indexed normally.
//
// This is the end-to-end integration test for spec 006-unified-ignore (T029).
//
// PARALLEL SAFETY: This test can run in parallel — it does NOT use os.Chdir
// or mutate any process-global state. All paths are relative to t.TempDir().
func TestEndToEnd_GitignoreRespected(t *testing.T) {
	tmpDir := t.TempDir()

	// Step 1: Create vault structure with .gitignore and test files.
	//
	// Layout:
	//   .gitignore              → contains "node_modules/"
	//   node_modules/pkg/README.md  → should be EXCLUDED
	//   docs/guide.md              → should be INCLUDED
	//   .uf/dewey/config.yaml      → minimal config
	//   .uf/dewey/sources.yaml     → empty sources

	// Write .gitignore that excludes node_modules/.
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte("node_modules/\n"), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	// Create node_modules/pkg/README.md (should be excluded by .gitignore).
	nodeModulesDir := filepath.Join(tmpDir, "node_modules", "pkg")
	if err := os.MkdirAll(nodeModulesDir, 0o755); err != nil {
		t.Fatalf("mkdir node_modules/pkg: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nodeModulesDir, "README.md"), []byte("# Package Readme\n\nThis should be excluded."), 0o644); err != nil {
		t.Fatalf("write node_modules/pkg/README.md: %v", err)
	}

	// Create docs/guide.md (should be included).
	docsDir := filepath.Join(tmpDir, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, "guide.md"), []byte("# Guide\n\nThis should be included."), 0o644); err != nil {
		t.Fatalf("write docs/guide.md: %v", err)
	}

	// Create .uf/dewey/ directory with minimal config.
	deweyDir := filepath.Join(tmpDir, deweyWorkspaceDir)
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("mkdir .uf/dewey: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deweyDir, "config.yaml"), []byte("embedding:\n  model: test\n"), 0o644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deweyDir, "sources.yaml"), []byte("sources: []\n"), 0o644); err != nil {
		t.Fatalf("write sources.yaml: %v", err)
	}

	// Step 2: Initialize vault with store.
	dbPath := filepath.Join(deweyDir, "graph.db")
	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	vc := vault.New(tmpDir, vault.WithStore(s))
	if err := vc.Load(); err != nil {
		t.Fatalf("vault.Load: %v", err)
	}
	vc.BuildBacklinks()

	// Step 3: Verify the in-memory index via GetAllPages.
	ctx := context.Background()
	pages, err := vc.GetAllPages(ctx)
	if err != nil {
		t.Fatalf("GetAllPages: %v", err)
	}

	// Build a set of page names for easy lookup.
	pageNames := make(map[string]bool)
	for _, p := range pages {
		pageNames[strings.ToLower(p.Name)] = true
	}

	// docs/guide MUST be in the index.
	if !pageNames["docs/guide"] {
		t.Error("expected page \"docs/guide\" to be in the vault index, but it was not found")
	}

	// node_modules/pkg/README MUST NOT be in the index.
	if pageNames["node_modules/pkg/readme"] {
		t.Error("expected page \"node_modules/pkg/README\" to be excluded by .gitignore, but it was found in the vault index")
	}

	// Total page count should be exactly 1 (only docs/guide).
	if len(pages) != 1 {
		names := make([]string, 0, len(pages))
		for _, p := range pages {
			names = append(names, p.Name)
		}
		t.Errorf("expected 1 page in vault index, got %d: %v", len(pages), names)
	}
}

// TestEndToEnd_CodeSourceIndex verifies the full code source pipeline:
// configure a type:code source → fetch documents → index into store →
// verify pages and blocks contain exported declarations.
//
// This validates SC-001 (CLI command discovery) and SC-002 (API discovery)
// from spec 010-code-source-index.
//
// PARALLEL SAFETY: This test does NOT use os.Chdir or mutate process-global
// state. All paths are relative to t.TempDir(). It can run in parallel.
func TestEndToEnd_CodeSourceIndex(t *testing.T) {
	tmpDir := t.TempDir()

	// Step 1: Create a Go source file with an exported function + doc comment.
	// This is the file that should be indexed by the code source.
	libContent := `package mathlib

// Add returns the sum of two integers.
// It handles arbitrary int values.
func Add(a, b int) int {
	return a + b
}

// Subtract returns the difference of two integers.
func Subtract(a, b int) int {
	return a - b
}

// internal is unexported and should NOT appear in the index.
func internal() {}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "lib.go"), []byte(libContent), 0o644); err != nil {
		t.Fatalf("write lib.go: %v", err)
	}

	// Create a test file — should be excluded by the code source (FR-014).
	testContent := `package mathlib

import "testing"

func TestAdd(t *testing.T) {
	if Add(1, 2) != 3 {
		t.Error("expected 3")
	}
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "lib_test.go"), []byte(testContent), 0o644); err != nil {
		t.Fatalf("write lib_test.go: %v", err)
	}

	// Step 2: Create .uf/dewey/ directory with sources.yaml and config.yaml.
	deweyDir := filepath.Join(tmpDir, deweyWorkspaceDir)
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("mkdir .uf/dewey: %v", err)
	}

	sourcesYAML := `sources:
  - id: code-local
    type: code
    name: local-code
    config:
      path: "` + tmpDir + `"
      languages:
        - go
`
	if err := os.WriteFile(filepath.Join(deweyDir, "sources.yaml"), []byte(sourcesYAML), 0o644); err != nil {
		t.Fatalf("write sources.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deweyDir, "config.yaml"), []byte("embedding:\n  model: test\n"), 0o644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}

	// Step 3: Run the indexing pipeline manually (same pattern as
	// TestEndToEnd_ExternalPagesSurviveServeStartup).
	dbPath := filepath.Join(deweyDir, "graph.db")
	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	// Load sources config.
	configs, err := source.LoadSourcesConfig(filepath.Join(deweyDir, "sources.yaml"))
	if err != nil {
		t.Fatalf("load sources config: %v", err)
	}
	if len(configs) != 1 {
		t.Fatalf("expected 1 source config, got %d", len(configs))
	}
	if configs[0].Type != "code" {
		t.Fatalf("expected source type 'code', got %q", configs[0].Type)
	}

	// Create source manager and fetch documents.
	mgr := source.NewManager(configs, tmpDir, filepath.Join(deweyDir, "cache"))
	result, allDocs := mgr.FetchAll("", true, nil)

	if result.TotalErrs > 0 {
		t.Fatalf("fetch had %d errors", result.TotalErrs)
	}

	docs, ok := allDocs["code-local"]
	if !ok || len(docs) == 0 {
		t.Fatal("no documents fetched from code-local source")
	}

	// Step 4: Index documents into the store (same pipeline as cli.go indexDocuments).
	for _, doc := range docs {
		pageName := strings.ToLower("code-local/" + doc.ID)

		_, blocks := vault.ParseDocument(pageName, doc.Content)

		page := &store.Page{
			Name:        pageName,
			SourceID:    "code-local",
			SourceDocID: doc.ID,
			ContentHash: doc.ContentHash,
			CreatedAt:   doc.FetchedAt.UnixMilli(),
			UpdatedAt:   doc.FetchedAt.UnixMilli(),
		}
		if err := s.InsertPage(page); err != nil {
			t.Fatalf("insert page %q: %v", pageName, err)
		}

		if err := vault.PersistBlocks(s, pageName, blocks, sql.NullString{}, 0); err != nil {
			t.Fatalf("persist blocks for %q: %v", pageName, err)
		}

		if err := vault.PersistLinks(s, pageName, blocks); err != nil {
			t.Fatalf("persist links for %q: %v", pageName, err)
		}
	}

	// Step 5: Verify the store has pages from the code source.
	pages, err := s.ListPagesBySource("code-local")
	if err != nil {
		t.Fatalf("ListPagesBySource: %v", err)
	}
	if len(pages) == 0 {
		t.Fatal("expected at least 1 page from code-local source, got 0")
	}

	// Verify lib.go was indexed (page name is namespaced: code-local/lib.go).
	foundLib := false
	for _, p := range pages {
		if strings.Contains(p.Name, "lib.go") {
			foundLib = true
		}
	}
	if !foundLib {
		names := make([]string, 0, len(pages))
		for _, p := range pages {
			names = append(names, p.Name)
		}
		t.Errorf("expected a page containing 'lib.go', got pages: %v", names)
	}

	// Step 6: Verify the page content contains the exported function signature.
	// The code source formats blocks as markdown with ## headings per declaration.
	libPageName := strings.ToLower("code-local/lib.go")
	blocks, err := s.GetBlocksByPage(libPageName)
	if err != nil {
		t.Fatalf("GetBlocksByPage(%q): %v", libPageName, err)
	}
	if len(blocks) == 0 {
		t.Fatalf("expected blocks for %q, got 0", libPageName)
	}

	// Collect all block content to search for expected declarations.
	var allContent strings.Builder
	for _, b := range blocks {
		allContent.WriteString(b.Content)
		allContent.WriteString("\n")
	}
	content := allContent.String()

	// Exported function Add should be present.
	if !strings.Contains(content, "Add") {
		t.Error("expected block content to contain exported function 'Add'")
	}

	// Exported function Subtract should be present.
	if !strings.Contains(content, "Subtract") {
		t.Error("expected block content to contain exported function 'Subtract'")
	}

	// Doc comment should be preserved.
	if !strings.Contains(content, "returns the sum") {
		t.Error("expected block content to contain doc comment 'returns the sum'")
	}

	// Step 7: Verify test file content is NOT in the store.
	// The code source should skip *_test.go files (FR-014).
	testPageName := strings.ToLower("code-local/lib_test.go")
	testBlocks, err := s.GetBlocksByPage(testPageName)
	if err != nil {
		t.Fatalf("GetBlocksByPage(%q): %v", testPageName, err)
	}
	if len(testBlocks) > 0 {
		t.Error("test file lib_test.go should NOT be indexed, but found blocks for it")
	}

	// Also verify no page exists for the test file.
	testPage, _ := s.GetPage(testPageName)
	if testPage != nil {
		t.Error("test file lib_test.go should NOT have a page in the store")
	}

	// Verify unexported function is not in the content.
	if strings.Contains(content, "internal") {
		t.Error("unexported function 'internal' should NOT appear in indexed content")
	}
}

// TestEndToEnd_StoreCompileSearch verifies the full knowledge compilation
// pipeline: store learnings → compile → verify compiled articles exist
// in the store and filesystem.
//
// This validates SC-002 (temporal contradiction resolution) and SC-007
// (knowledge evolution) from spec 013-knowledge-compile.
//
// PARALLEL SAFETY: This test does NOT use os.Chdir or mutate process-global
// state. All paths are relative to t.TempDir(). It can run in parallel.
func TestEndToEnd_StoreCompileSearch(t *testing.T) {
	tmpDir := t.TempDir()

	// Step 1: Create .uf/dewey/ directory structure.
	deweyDir := filepath.Join(tmpDir, deweyWorkspaceDir)
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("mkdir .uf/dewey: %v", err)
	}

	// Step 2: Open store with schema v2.
	dbPath := filepath.Join(deweyDir, "graph.db")
	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	// Step 3: Store 3 learnings via the tools/learning handler.
	// Two "auth" decisions (temporal contradiction) and one "performance" pattern.
	learning := tools.NewLearning(nil, s, "") // nil embedder — no Ollama in tests.

	// Learning 1: auth decision — "Use Option A"
	result1, _, err := learning.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "Use Option A for authentication because it supports SSO.",
		Tag:         "auth",
		Category:    "decision",
	})
	if err != nil {
		t.Fatalf("StoreLearning 1 error: %v", err)
	}
	if result1.IsError {
		t.Fatalf("StoreLearning 1 returned error: %s", extractText(result1))
	}

	// Small delay to ensure distinct timestamps for temporal ordering.
	time.Sleep(10 * time.Millisecond)

	// Learning 2: auth decision — "Switch to Option B" (contradicts Learning 1).
	result2, _, err := learning.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "Switch to Option B for authentication due to rate limiting issues with Option A.",
		Tag:         "auth",
		Category:    "decision",
	})
	if err != nil {
		t.Fatalf("StoreLearning 2 error: %v", err)
	}
	if result2.IsError {
		t.Fatalf("StoreLearning 2 returned error: %s", extractText(result2))
	}

	// Learning 3: performance pattern — "Use connection pooling"
	result3, _, err := learning.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "Use connection pooling for database access to reduce latency under load.",
		Tag:         "performance",
		Category:    "pattern",
	})
	if err != nil {
		t.Fatalf("StoreLearning 3 error: %v", err)
	}
	if result3.IsError {
		t.Fatalf("StoreLearning 3 returned error: %s", extractText(result3))
	}

	// Step 4: Create a Compile tool with NoopSynthesizer.
	synth := &llm.NoopSynthesizer{
		Response: "## Current State\n\nCompiled article content from learnings.",
		Avail:    true,
		Model:    "test-noop",
	}
	compile := tools.NewCompile(s, nil, synth, tmpDir)

	// Step 5: Call CompileAll (no incremental identities).
	compileResult, _, err := compile.Compile(context.Background(), nil, types.CompileInput{})
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}
	if compileResult.IsError {
		t.Fatalf("Compile returned error: %s", extractText(compileResult))
	}

	// Parse the compile result to verify summary.
	compileText := extractText(compileResult)
	var compileSummary map[string]any
	if err := json.Unmarshal([]byte(compileText), &compileSummary); err != nil {
		t.Fatalf("unmarshal compile result: %v\ntext: %s", err, compileText)
	}

	if compileSummary["status"] != "compiled" {
		t.Errorf("compile status = %v, want 'compiled'", compileSummary["status"])
	}
	totalArticles, _ := compileSummary["total_articles"].(float64)
	if totalArticles != 2 {
		t.Errorf("total_articles = %v, want 2 (auth + performance)", totalArticles)
	}

	// Step 6: Verify compiled articles exist in the store.

	// 6a: Auth compiled article.
	authPage, err := s.GetPage("compiled/auth")
	if err != nil {
		t.Fatalf("GetPage(compiled/auth): %v", err)
	}
	if authPage == nil {
		t.Fatal("compiled/auth page not found in store")
	}
	if authPage.SourceID != "compiled" {
		t.Errorf("auth page source_id = %q, want %q", authPage.SourceID, "compiled")
	}
	if authPage.Tier != "draft" {
		t.Errorf("auth page tier = %q, want %q", authPage.Tier, "draft")
	}

	// 6b: Performance compiled article.
	perfPage, err := s.GetPage("compiled/performance")
	if err != nil {
		t.Fatalf("GetPage(compiled/performance): %v", err)
	}
	if perfPage == nil {
		t.Fatal("compiled/performance page not found in store")
	}
	if perfPage.SourceID != "compiled" {
		t.Errorf("performance page source_id = %q, want %q", perfPage.SourceID, "compiled")
	}

	// 6c: Verify compiled articles have blocks (searchable content).
	authBlocks, err := s.GetBlocksByPage("compiled/auth")
	if err != nil {
		t.Fatalf("GetBlocksByPage(compiled/auth): %v", err)
	}
	if len(authBlocks) == 0 {
		t.Error("compiled/auth has 0 blocks — article content was not persisted")
	}

	perfBlocks, err := s.GetBlocksByPage("compiled/performance")
	if err != nil {
		t.Fatalf("GetBlocksByPage(compiled/performance): %v", err)
	}
	if len(perfBlocks) == 0 {
		t.Error("compiled/performance has 0 blocks — article content was not persisted")
	}

	// 6d: Verify _index.md exists in .uf/dewey/compiled/.
	indexPath := filepath.Join(deweyDir, "compiled", "_index.md")
	indexData, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("failed to read _index.md: %v", err)
	}
	indexContent := string(indexData)
	if !strings.Contains(indexContent, "Auth") {
		t.Error("_index.md missing Auth topic")
	}
	if !strings.Contains(indexContent, "Performance") {
		t.Error("_index.md missing Performance topic")
	}

	// 6e: Verify compiled article files exist on disk.
	authArticlePath := filepath.Join(deweyDir, "compiled", "auth.md")
	if _, err := os.Stat(authArticlePath); os.IsNotExist(err) {
		t.Error("auth.md not written to filesystem")
	}
	perfArticlePath := filepath.Join(deweyDir, "compiled", "performance.md")
	if _, err := os.Stat(perfArticlePath); os.IsNotExist(err) {
		t.Error("performance.md not written to filesystem")
	}

	// 6f: Verify the original learnings still exist (compilation doesn't delete them).
	learningPages, err := s.ListLearningPages()
	if err != nil {
		t.Fatalf("ListLearningPages: %v", err)
	}
	if len(learningPages) != 3 {
		t.Errorf("expected 3 learning pages after compilation, got %d", len(learningPages))
	}
}

// TestEndToEnd_CurateKnowledgeStore verifies the full curated knowledge store
// pipeline: create source pages → configure a knowledge store → run curation
// → verify knowledge files created with correct frontmatter → verify pages
// in store with source_id=knowledge-{name} and tier=curated → verify
// curation checkpoint exists.
//
// This validates the end-to-end flow for spec 015-curated-knowledge-stores (T039).
//
// PARALLEL SAFETY: This test does NOT use os.Chdir or mutate process-global
// state. All paths are relative to t.TempDir(). It can run in parallel.
func TestEndToEnd_CurateKnowledgeStore(t *testing.T) {
	tmpDir := t.TempDir()

	// Step 1: Create .uf/dewey/ directory structure.
	deweyDir := filepath.Join(tmpDir, deweyWorkspaceDir)
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("mkdir .uf/dewey: %v", err)
	}

	// Step 2: Open store and create source pages simulating indexed meeting notes.
	dbPath := filepath.Join(deweyDir, "graph.db")
	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	// Insert source pages with blocks (simulating what dewey index creates).
	sourcePages := []struct {
		name    string
		content string
	}{
		{
			name:    "disk-meetings/sprint-planning-2026-04-01",
			content: "# Sprint Planning\n\nTeam decided to use OAuth2 for authentication.\nRationale: supports SSO and is industry standard.",
		},
		{
			name:    "disk-meetings/architecture-review-2026-04-05",
			content: "# Architecture Review\n\nAgreed to use connection pooling for database access.\nThis reduces latency under load by 40%.",
		},
	}

	for _, sp := range sourcePages {
		if err := s.InsertPage(&store.Page{
			Name:        sp.name,
			SourceID:    "disk-meetings",
			SourceDocID: sp.name,
			ContentHash: fmt.Sprintf("hash-%s", sp.name),
			Tier:        "authored",
		}); err != nil {
			t.Fatalf("insert page %q: %v", sp.name, err)
		}

		// Parse and persist blocks.
		_, blocks := vault.ParseDocument(sp.name, sp.content)
		if err := vault.PersistBlocks(s, sp.name, blocks, sql.NullString{}, 0); err != nil {
			t.Fatalf("persist blocks for %q: %v", sp.name, err)
		}
	}

	// Step 3: Create knowledge-stores.yaml with a store mapping to the source.
	ksContent := `stores:
  - name: team-knowledge
    sources:
      - disk-meetings
    curation_interval: "10m"
`
	if err := os.WriteFile(filepath.Join(deweyDir, "knowledge-stores.yaml"), []byte(ksContent), 0o644); err != nil {
		t.Fatalf("write knowledge-stores.yaml: %v", err)
	}

	// Step 4: Create a NoopSynthesizer that returns mock extracted knowledge items.
	mockResponse := `[
  {
    "tag": "authentication",
    "category": "decision",
    "confidence": "high",
    "quality_flags": [],
    "sources": [{"source_id": "disk-meetings", "document": "sprint-planning-2026-04-01", "excerpt": "Team decided to use OAuth2"}],
    "content": "Use OAuth2 for authentication. Supports SSO and is industry standard."
  },
  {
    "tag": "database-performance",
    "category": "pattern",
    "confidence": "medium",
    "quality_flags": [{"type": "implied_assumption", "detail": "Assumes current connection model is not pooled"}],
    "sources": [{"source_id": "disk-meetings", "document": "architecture-review-2026-04-05", "excerpt": "connection pooling reduces latency by 40%"}],
    "content": "Use connection pooling for database access to reduce latency under load by 40%."
  }
]`
	synth := &llm.NoopSynthesizer{
		Response: mockResponse,
		Avail:    true,
		Model:    "test-noop",
	}

	// Step 5: Create a Pipeline and run CurateStore.
	pipeline := curate.NewPipeline(s, synth, nil, tmpDir)

	storeCfg := curate.StoreConfig{
		Name:             "team-knowledge",
		Sources:          []string{"disk-meetings"},
		CurationInterval: "10m",
	}

	filesCreated, err := pipeline.CurateStore(context.Background(), storeCfg)
	if err != nil {
		t.Fatalf("CurateStore error: %v", err)
	}
	if filesCreated != 2 {
		t.Errorf("CurateStore created %d files, want 2", filesCreated)
	}

	// Step 6: Verify knowledge files exist in the store's output directory.
	storePath := curate.ResolveStorePath(storeCfg, tmpDir)

	// 6a: Check authentication knowledge file.
	authPath := filepath.Join(storePath, "authentication-1.md")
	authData, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatalf("read authentication-1.md: %v", err)
	}
	authContent := string(authData)
	if !strings.Contains(authContent, "tag: authentication") {
		t.Error("authentication-1.md missing 'tag: authentication' in frontmatter")
	}
	if !strings.Contains(authContent, "category: decision") {
		t.Error("authentication-1.md missing 'category: decision' in frontmatter")
	}
	if !strings.Contains(authContent, "confidence: high") {
		t.Error("authentication-1.md missing 'confidence: high' in frontmatter")
	}
	if !strings.Contains(authContent, "tier: curated") {
		t.Error("authentication-1.md missing 'tier: curated' in frontmatter")
	}
	if !strings.Contains(authContent, "OAuth2") {
		t.Error("authentication-1.md missing OAuth2 content")
	}

	// 6b: Check database-performance knowledge file.
	dbPerfPath := filepath.Join(storePath, "database-performance-2.md")
	dbPerfData, err := os.ReadFile(dbPerfPath)
	if err != nil {
		t.Fatalf("read database-performance-2.md: %v", err)
	}
	dbPerfContent := string(dbPerfData)
	if !strings.Contains(dbPerfContent, "tag: database-performance") {
		t.Error("database-performance-2.md missing 'tag: database-performance' in frontmatter")
	}
	if !strings.Contains(dbPerfContent, "category: pattern") {
		t.Error("database-performance-2.md missing 'category: pattern' in frontmatter")
	}
	if !strings.Contains(dbPerfContent, "implied_assumption") {
		t.Error("database-performance-2.md missing quality flag 'implied_assumption'")
	}

	// 6c: Check _index.md exists.
	indexPath := filepath.Join(storePath, "_index.md")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		t.Error("_index.md not created in knowledge store directory")
	}

	// Step 7: Verify curation checkpoint file exists.
	statePath := filepath.Join(storePath, ".curation-state.json")
	stateData, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read .curation-state.json: %v", err)
	}
	stateContent := string(stateData)
	if !strings.Contains(stateContent, "last_curated_at") {
		t.Error(".curation-state.json missing 'last_curated_at' field")
	}
	if !strings.Contains(stateContent, "disk-meetings") {
		t.Error(".curation-state.json missing 'disk-meetings' source checkpoint")
	}

	// Step 8: Verify curation state round-trips correctly.
	state, err := curate.LoadCurationState(storePath)
	if err != nil {
		t.Fatalf("LoadCurationState: %v", err)
	}
	if state.LastCuratedAt.IsZero() {
		t.Error("curation state has zero LastCuratedAt")
	}
	if _, ok := state.SourceCheckpoints["disk-meetings"]; !ok {
		t.Error("curation state missing 'disk-meetings' source checkpoint")
	}

	// Step 9: Now auto-index the curated files into the store (simulating
	// what the curate MCP tool does after curation). We use the Curate tool
	// handler to verify the full flow including auto-indexing.
	// Use Incremental=false to force re-processing (the first curation
	// already set a checkpoint, so incremental mode would skip all docs).
	forceIncremental := false
	curateTool := tools.NewCurate(s, nil, synth, tmpDir, nil)
	curateResult, _, err := curateTool.Curate(context.Background(), nil, types.CurateInput{
		Store:       "team-knowledge",
		Incremental: &forceIncremental,
	})
	if err != nil {
		t.Fatalf("Curate tool error: %v", err)
	}
	if curateResult.IsError {
		t.Fatalf("Curate tool returned error: %s", extractText(curateResult))
	}

	// Step 10: Verify knowledge pages are in the store with correct source_id and tier.
	knowledgePages, err := s.ListPagesBySource("knowledge-team-knowledge")
	if err != nil {
		t.Fatalf("ListPagesBySource(knowledge-team-knowledge): %v", err)
	}
	if len(knowledgePages) == 0 {
		t.Error("expected knowledge pages with source_id 'knowledge-team-knowledge', got 0")
	}

	for _, kp := range knowledgePages {
		if kp.Tier != "curated" {
			t.Errorf("knowledge page %q tier = %q, want %q", kp.Name, kp.Tier, "curated")
		}
		if kp.SourceID != "knowledge-team-knowledge" {
			t.Errorf("knowledge page %q source_id = %q, want %q", kp.Name, kp.SourceID, "knowledge-team-knowledge")
		}
	}

	// Step 11: Verify knowledge pages have blocks (searchable content).
	for _, kp := range knowledgePages {
		blocks, err := s.GetBlocksByPage(kp.Name)
		if err != nil {
			t.Fatalf("GetBlocksByPage(%q): %v", kp.Name, err)
		}
		if len(blocks) == 0 {
			t.Errorf("knowledge page %q has 0 blocks — content was not indexed", kp.Name)
		}
	}
}

// extractText extracts the text content from a CallToolResult.
// Used by integration tests to read MCP tool response text.
func extractText(result *mcp.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}
	// The Content field is []ContentPart. Each part has a Text field.
	// Use fmt.Sprintf to extract — the mcp package types are not directly
	// accessible from the main package, so we marshal and extract.
	data, err := json.Marshal(result.Content)
	if err != nil {
		return fmt.Sprintf("<marshal error: %v>", err)
	}
	var parts []map[string]any
	if err := json.Unmarshal(data, &parts); err != nil {
		return string(data)
	}
	var texts []string
	for _, p := range parts {
		if text, ok := p["text"].(string); ok {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, "\n")
}
