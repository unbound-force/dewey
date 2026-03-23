package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unbound-force/dewey/source"
	"github.com/unbound-force/dewey/store"
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

	// Verify .dewey/ was created.
	deweyDir := filepath.Join(tmpDir, ".dewey")
	if _, err := os.Stat(deweyDir); os.IsNotExist(err) {
		t.Fatal(".dewey/ directory was not created")
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
	deweyDir := filepath.Join(tmpDir, ".dewey")
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
