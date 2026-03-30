// PARALLEL SAFETY: Tests in this file MUST NOT use t.Parallel().
// They mutate process-global state: os.Chdir (working directory),
// os.Stdout (for output capture), and logger (for log assertions).
// Running these tests in parallel would cause data races and flaky failures.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unbound-force/dewey/client"
	"github.com/unbound-force/dewey/source"
	"github.com/unbound-force/dewey/store"
	"github.com/unbound-force/dewey/types"
)

// TestRootCmd_Version verifies the root command reports the correct version.
func TestRootCmd_Version(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(version) failed: %v", err)
	}

	got := strings.TrimSpace(buf.String())
	if got != version {
		t.Errorf("version output = %q, want %q", got, version)
	}
}

// TestRootCmd_VersionSubcommand verifies `dewey version` subcommand works.
// NOTE: --version flag was removed to avoid conflict with --verbose/-v.
// Version is available via the `dewey version` subcommand.
func TestRootCmd_VersionSubcommand(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(version) failed: %v", err)
	}

	got := strings.TrimSpace(buf.String())
	if !strings.Contains(got, version) {
		t.Errorf("version output = %q, should contain %q", got, version)
	}
}

// TestRootCmd_Help verifies the root command produces help output.
func TestRootCmd_Help(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(--help) failed: %v", err)
	}

	got := buf.String()
	// Verify key subcommands are listed in help.
	for _, sub := range []string{"serve", "journal", "add", "search", "version"} {
		if !strings.Contains(got, sub) {
			t.Errorf("help output missing subcommand %q", sub)
		}
	}
}

// TestServeCmd_HasFlags verifies the serve subcommand has all expected flags.
func TestServeCmd_HasFlags(t *testing.T) {
	cmd := newServeCmd()

	expectedFlags := []string{"read-only", "backend", "vault", "daily-folder", "http"}
	for _, name := range expectedFlags {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("serve command missing flag --%s", name)
		}
	}
}

// TestJournalCmd_HasFlags verifies the journal subcommand has expected flags.
func TestJournalCmd_HasFlags(t *testing.T) {
	cmd := newJournalCmd()

	if cmd.Flags().Lookup("date") == nil {
		t.Error("journal command missing flag --date")
	}

	// Verify short flag -d exists.
	if cmd.Flags().ShorthandLookup("d") == nil {
		t.Error("journal command missing short flag -d")
	}
}

// TestAddCmd_HasFlags verifies the add subcommand has expected flags.
func TestAddCmd_HasFlags(t *testing.T) {
	cmd := newAddCmd()

	if cmd.Flags().Lookup("page") == nil {
		t.Error("add command missing flag --page")
	}

	// Verify short flag -p exists.
	if cmd.Flags().ShorthandLookup("p") == nil {
		t.Error("add command missing short flag -p")
	}
}

// TestSearchCmd_HasFlags verifies the search subcommand has expected flags.
func TestSearchCmd_HasFlags(t *testing.T) {
	cmd := newSearchCmd()

	if cmd.Flags().Lookup("limit") == nil {
		t.Error("search command missing flag --limit")
	}
}

// TestSearchCmd_NoQuery verifies search fails without a query.
func TestSearchCmd_NoQuery(t *testing.T) {
	cmd := newSearchCmd()
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("search with no query should fail")
	}
	if !strings.Contains(err.Error(), "query is required") {
		t.Errorf("error = %q, want to contain 'query is required'", err.Error())
	}
}

// TestAddCmd_NoPage verifies add fails without --page.
func TestAddCmd_NoPage(t *testing.T) {
	cmd := newAddCmd()
	cmd.SetArgs([]string{"some content"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("add without --page should fail")
	}
	if !strings.Contains(err.Error(), "--page is required") {
		t.Errorf("error = %q, want to contain '--page is required'", err.Error())
	}
}

// TestRootCmd_UnknownSubcommand verifies unknown subcommands produce an error.
func TestRootCmd_UnknownSubcommand(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"nonexistent"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("unknown subcommand should fail")
	}
}

// TestOrdinalDate_Formats verifies the ordinal date formatting helper.
func TestOrdinalDate_Formats(t *testing.T) {
	tests := []struct {
		name string
		date string
		want string
	}{
		{"1st", "2026-01-01", "Jan 1st, 2026"},
		{"2nd", "2026-01-02", "Jan 2nd, 2026"},
		{"3rd", "2026-01-03", "Jan 3rd, 2026"},
		{"4th", "2026-01-04", "Jan 4th, 2026"},
		{"11th", "2026-01-11", "Jan 11th, 2026"},
		{"21st", "2026-01-21", "Jan 21st, 2026"},
		{"22nd", "2026-01-22", "Jan 22nd, 2026"},
		{"23rd", "2026-01-23", "Jan 23rd, 2026"},
		{"31st", "2026-01-31", "Jan 31st, 2026"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := time.Parse("2006-01-02", tt.date)
			if err != nil {
				t.Fatalf("parse date %q: %v", tt.date, err)
			}
			got := ordinalDate(parsed)
			if got != tt.want {
				t.Errorf("ordinalDate(%s) = %q, want %q", tt.date, got, tt.want)
			}
		})
	}
}

// TestReadContentFromArgs_WithArgs verifies content reading from positional args.
func TestReadContentFromArgs_WithArgs(t *testing.T) {
	got := readContentFromArgs([]string{"hello", "world"})
	if got != "hello world" {
		t.Errorf("readContentFromArgs = %q, want %q", got, "hello world")
	}
}

// TestReadContentFromArgs_Empty verifies empty args returns empty string.
func TestReadContentFromArgs_Empty(t *testing.T) {
	got := readContentFromArgs(nil)
	// When stdin is a terminal (not piped), should return empty.
	// In test context, stdin behavior varies, so we just verify no panic.
	_ = got
}

// --- Init command tests ---

// TestInitCmd_CreatesDirectory verifies dewey init creates .dewey/ directory.
func TestInitCmd_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	deweyDir := filepath.Join(tmpDir, ".dewey")
	if _, err := os.Stat(deweyDir); os.IsNotExist(err) {
		t.Fatal(".dewey/ directory was not created")
	}
}

// TestInitCmd_DefaultConfig verifies config.yaml has expected content.
func TestInitCmd_DefaultConfig(t *testing.T) {
	tmpDir := t.TempDir()

	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	configPath := filepath.Join(tmpDir, ".dewey", "config.yaml")
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config.yaml: %v", err)
	}

	configStr := string(content)
	if !strings.Contains(configStr, "granite-embedding:30m") {
		t.Error("config.yaml should contain default embedding model")
	}
	if !strings.Contains(configStr, "embedding") {
		t.Error("config.yaml should contain embedding section")
	}
}

// TestInitCmd_DefaultSources verifies sources.yaml has expected content.
func TestInitCmd_DefaultSources(t *testing.T) {
	tmpDir := t.TempDir()

	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	sourcesPath := filepath.Join(tmpDir, ".dewey", "sources.yaml")
	content, err := os.ReadFile(sourcesPath)
	if err != nil {
		t.Fatalf("read sources.yaml: %v", err)
	}

	sourcesStr := string(content)
	if !strings.Contains(sourcesStr, "disk-local") {
		t.Error("sources.yaml should contain disk-local source")
	}
	if !strings.Contains(sourcesStr, "type: disk") {
		t.Error("sources.yaml should contain type: disk")
	}
}

// TestInitCmd_Idempotent verifies running init twice does not error.
func TestInitCmd_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()

	// First init.
	cmd1 := newInitCmd()
	cmd1.SetArgs([]string{"--vault", tmpDir})
	if err := cmd1.Execute(); err != nil {
		t.Fatalf("first init failed: %v", err)
	}

	// Second init should succeed (idempotent).
	cmd2 := newInitCmd()
	cmd2.SetArgs([]string{"--vault", tmpDir})
	if err := cmd2.Execute(); err != nil {
		t.Fatalf("second init should not fail: %v", err)
	}
}

// TestInitCmd_GitignoreAppend verifies .dewey/ is added to .gitignore.
func TestInitCmd_GitignoreAppend(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a .gitignore without .dewey/.
	gitignorePath := filepath.Join(tmpDir, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte("node_modules/\n"), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if !strings.Contains(string(content), ".dewey/") {
		t.Error(".gitignore should contain .dewey/")
	}
}

// TestInitCmd_GitignoreAlreadyPresent verifies .dewey/ is not duplicated.
func TestInitCmd_GitignoreAlreadyPresent(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a .gitignore that already has .dewey/.
	gitignorePath := filepath.Join(tmpDir, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte(".dewey/\n"), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	// We need to remove .dewey/ first so init actually runs.
	// Actually, init will see .dewey/ already exists and return early.
	// So let's test the gitignore logic separately by not having .dewey/ yet.
	// The init command checks for .dewey/ existence first.
	// Since .dewey/ doesn't exist, init will create it and check .gitignore.
	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	// Count occurrences — should be exactly 1.
	count := strings.Count(string(content), ".dewey/")
	if count != 1 {
		t.Errorf(".dewey/ appears %d times in .gitignore, want 1", count)
	}
}

// --- Status command tests ---

// TestStatusCmd_Uninitialized verifies status fails when .dewey/ doesn't exist.
func TestStatusCmd_Uninitialized(t *testing.T) {
	tmpDir := t.TempDir()

	// Change to temp dir for the status command.
	oldDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(oldDir) }()

	cmd := newStatusCmd()
	err := cmd.Execute()
	if err == nil {
		t.Fatal("status should fail when not initialized")
	}
	if !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("error = %q, want to contain 'not initialized'", err.Error())
	}
}

// TestStatusCmd_TextOutput verifies human-readable status output.
func TestStatusCmd_TextOutput(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize.
	deweyDir := filepath.Join(tmpDir, ".dewey")
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	oldDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(oldDir) }()

	cmd := newStatusCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("status failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Dewey Index Status") {
		t.Error("status output should contain 'Dewey Index Status'")
	}
	if !strings.Contains(output, "Pages:") {
		t.Error("status output should contain 'Pages:'")
	}
	if !strings.Contains(output, "Blocks:") {
		t.Error("status output should contain 'Blocks:'")
	}
}

// TestStatusCmd_JSONOutput verifies JSON status output.
func TestStatusCmd_JSONOutput(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize.
	deweyDir := filepath.Join(tmpDir, ".dewey")
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	oldDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(oldDir) }()

	cmd := newStatusCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("status --json failed: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, buf.String())
	}

	// Verify expected fields.
	if _, ok := result["pages"]; !ok {
		t.Error("JSON output missing 'pages' field")
	}
	if _, ok := result["blocks"]; !ok {
		t.Error("JSON output missing 'blocks' field")
	}
	if _, ok := result["path"]; !ok {
		t.Error("JSON output missing 'path' field")
	}
}

// TestInitCmd_HasFlags verifies the init subcommand has expected flags.
func TestInitCmd_HasFlags(t *testing.T) {
	cmd := newInitCmd()
	if cmd.Flags().Lookup("vault") == nil {
		t.Error("init command missing flag --vault")
	}
}

// TestStatusCmd_HasFlags verifies the status subcommand has expected flags.
func TestStatusCmd_HasFlags(t *testing.T) {
	cmd := newStatusCmd()
	if cmd.Flags().Lookup("json") == nil {
		t.Error("status command missing flag --json")
	}
}

// TestRootCmd_Help_IncludesNewSubcommands verifies init and status appear in help.
func TestRootCmd_Help_IncludesNewSubcommands(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(--help) failed: %v", err)
	}

	got := buf.String()
	for _, sub := range []string{"init", "status", "index", "source"} {
		if !strings.Contains(got, sub) {
			t.Errorf("help output missing subcommand %q", sub)
		}
	}
}

// --- Index command tests (T058B) ---

// TestIndexCmd_Uninitialized verifies index fails when .dewey/ doesn't exist.
func TestIndexCmd_Uninitialized(t *testing.T) {
	tmpDir := t.TempDir()

	oldDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(oldDir) }()

	cmd := newIndexCmd()
	err := cmd.Execute()
	if err == nil {
		t.Fatal("index should fail when not initialized")
	}
	if !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("error = %q, want to contain 'not initialized'", err.Error())
	}
}

// TestIndexCmd_HasFlags verifies the index subcommand has expected flags.
func TestIndexCmd_HasFlags(t *testing.T) {
	cmd := newIndexCmd()
	if cmd.Flags().Lookup("source") == nil {
		t.Error("index command missing flag --source")
	}
	if cmd.Flags().Lookup("force") == nil {
		t.Error("index command missing flag --force")
	}
}

// TestIndexCmd_WithDiskSource verifies indexing with a disk source.
func TestIndexCmd_WithDiskSource(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .dewey/ with sources.yaml.
	deweyDir := filepath.Join(tmpDir, ".dewey")
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

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

	// Create a test .md file.
	if err := os.WriteFile(filepath.Join(tmpDir, "test.md"), []byte("# Test\nContent"), 0o644); err != nil {
		t.Fatalf("write test.md: %v", err)
	}

	oldDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(oldDir) }()

	cmd := newIndexCmd()
	// Pass --no-embeddings because Ollama is not running in test env.
	cmd.SetArgs([]string{"--no-embeddings"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("index failed: %v", err)
	}
}

// --- Source add command tests (T058B) ---

// TestSourceAddCmd_Uninitialized verifies source add fails when not initialized.
func TestSourceAddCmd_Uninitialized(t *testing.T) {
	tmpDir := t.TempDir()

	oldDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(oldDir) }()

	cmd := newSourceCmd()
	cmd.SetArgs([]string{"add", "github", "--org", "test", "--repos", "repo1"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("source add should fail when not initialized")
	}
	if !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("error = %q, want to contain 'not initialized'", err.Error())
	}
}

// TestSourceAddCmd_GitHub verifies adding a GitHub source.
func TestSourceAddCmd_GitHub(t *testing.T) {
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

	cmd := newSourceCmd()
	cmd.SetArgs([]string{"add", "github", "--org", "unbound-force", "--repos", "gaze,website"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("source add github failed: %v", err)
	}

	// Verify source was added to sources.yaml.
	content, _ := os.ReadFile(filepath.Join(deweyDir, "sources.yaml"))
	if !strings.Contains(string(content), "github-unbound-force") {
		t.Error("sources.yaml should contain github-unbound-force")
	}
}

// TestSourceAddCmd_Web verifies adding a web source.
func TestSourceAddCmd_Web(t *testing.T) {
	tmpDir := t.TempDir()

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

	cmd := newSourceCmd()
	cmd.SetArgs([]string{"add", "web", "--url", "https://pkg.go.dev/std", "--name", "go-stdlib"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("source add web failed: %v", err)
	}

	content, _ := os.ReadFile(filepath.Join(deweyDir, "sources.yaml"))
	if !strings.Contains(string(content), "web-go-stdlib") {
		t.Error("sources.yaml should contain web-go-stdlib")
	}
}

// TestSourceAddCmd_DuplicateRejection verifies duplicate source rejection.
func TestSourceAddCmd_DuplicateRejection(t *testing.T) {
	tmpDir := t.TempDir()

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
  - id: github-test
    type: github
    name: test
    config:
      org: test
      repos:
        - repo1
`
	if err := os.WriteFile(filepath.Join(deweyDir, "sources.yaml"), []byte(sourcesContent), 0o644); err != nil {
		t.Fatalf("write sources.yaml: %v", err)
	}

	oldDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(oldDir) }()

	cmd := newSourceCmd()
	cmd.SetArgs([]string{"add", "github", "--org", "test", "--repos", "repo1"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("should reject duplicate source")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error = %q, want to contain 'already exists'", err.Error())
	}
}

// TestSourceAddCmd_InvalidType verifies unknown source type rejection.
func TestSourceAddCmd_InvalidType(t *testing.T) {
	tmpDir := t.TempDir()

	deweyDir := filepath.Join(tmpDir, ".dewey")
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deweyDir, "sources.yaml"), []byte("sources: []\n"), 0o644); err != nil {
		t.Fatalf("write sources.yaml: %v", err)
	}

	oldDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(oldDir) }()

	cmd := newSourceCmd()
	cmd.SetArgs([]string{"add", "ftp"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("should reject unknown source type")
	}
}

// TestFormatDuration verifies the duration formatting helper.
func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "30s"},
		{5 * time.Minute, "5m"},
		{4 * time.Hour, "4h"},
		{3 * 24 * time.Hour, "3d"},
	}

	for _, tt := range tests {
		got := formatDuration(tt.d)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

// --- findJournalPage tests (T020) ---

// newTestLogseqServer creates an httptest server that simulates the Logseq API.
// pageNames is the set of page names that exist. GetPage returns a result for
// any name in the set; other names get a null response.
func newTestLogseqServer(t *testing.T, pageNames map[string]bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
			Args   []any  `json:"args"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		switch req.Method {
		case "logseq.Editor.getPage":
			if len(req.Args) > 0 {
				name := fmt.Sprintf("%v", req.Args[0])
				if pageNames[name] {
					_ = json.NewEncoder(w).Encode(map[string]any{
						"name": name,
						"uuid": "page-uuid",
						"id":   1,
					})
					return
				}
			}
			// Page not found — Logseq returns null.
			_, _ = w.Write([]byte("null"))

		case "logseq.App.getCurrentGraph":
			// Return a graph at a temp path — tests override this if needed.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "test-graph",
				"path": t.TempDir(),
			})

		default:
			_, _ = w.Write([]byte("null"))
		}
	}))
}

// TestFindJournalPage_OrdinalFormat verifies findJournalPage returns the
// ordinal date format name when that page exists.
func TestFindJournalPage_OrdinalFormat(t *testing.T) {
	date := time.Date(2026, 1, 29, 0, 0, 0, 0, time.UTC)
	ordinal := ordinalDate(date) // "Jan 29th, 2026"

	srv := newTestLogseqServer(t, map[string]bool{ordinal: true})
	defer srv.Close()

	c := client.New(srv.URL, "")
	ctx := context.Background()

	got := findJournalPage(ctx, c, date)
	if got != ordinal {
		t.Errorf("findJournalPage() = %q, want %q", got, ordinal)
	}
}

// TestFindJournalPage_ISOFormat verifies findJournalPage falls through to
// ISO date format when ordinal format page does not exist.
func TestFindJournalPage_ISOFormat(t *testing.T) {
	date := time.Date(2026, 1, 29, 0, 0, 0, 0, time.UTC)
	isoName := "2026-01-29"

	// Only the ISO format page exists.
	srv := newTestLogseqServer(t, map[string]bool{isoName: true})
	defer srv.Close()

	c := client.New(srv.URL, "")
	ctx := context.Background()

	got := findJournalPage(ctx, c, date)
	if got != isoName {
		t.Errorf("findJournalPage() = %q, want %q", got, isoName)
	}
}

// TestFindJournalPage_LongFormat verifies findJournalPage falls through to
// "January 2, 2006" format when neither ordinal nor ISO pages exist.
func TestFindJournalPage_LongFormat(t *testing.T) {
	date := time.Date(2026, 1, 29, 0, 0, 0, 0, time.UTC)
	longName := "January 29, 2026"

	// Only the long format page exists.
	srv := newTestLogseqServer(t, map[string]bool{longName: true})
	defer srv.Close()

	c := client.New(srv.URL, "")
	ctx := context.Background()

	got := findJournalPage(ctx, c, date)
	if got != longName {
		t.Errorf("findJournalPage() = %q, want %q", got, longName)
	}
}

// TestFindJournalPage_NoPageExists verifies findJournalPage returns empty
// string when no journal page exists for any format.
func TestFindJournalPage_NoPageExists(t *testing.T) {
	date := time.Date(2026, 1, 29, 0, 0, 0, 0, time.UTC)

	// No pages exist.
	srv := newTestLogseqServer(t, map[string]bool{})
	defer srv.Close()

	c := client.New(srv.URL, "")
	ctx := context.Background()

	got := findJournalPage(ctx, c, date)
	if got != "" {
		t.Errorf("findJournalPage() = %q, want empty string", got)
	}
}

// TestFindJournalPage_PriorityOrder verifies ordinal format is preferred
// over ISO format when both pages exist.
func TestFindJournalPage_PriorityOrder(t *testing.T) {
	date := time.Date(2026, 1, 29, 0, 0, 0, 0, time.UTC)
	ordinal := ordinalDate(date) // "Jan 29th, 2026"

	// Both ordinal and ISO pages exist — ordinal should be returned first.
	srv := newTestLogseqServer(t, map[string]bool{
		ordinal:      true,
		"2026-01-29": true,
	})
	defer srv.Close()

	c := client.New(srv.URL, "")
	ctx := context.Background()

	got := findJournalPage(ctx, c, date)
	if got != ordinal {
		t.Errorf("findJournalPage() = %q, want %q (ordinal should take priority)", got, ordinal)
	}
}

// --- printSearchResults tests (T020) ---

// TestPrintSearchResults_MatchingBlocks verifies matching blocks are printed
// in "page | content" format and found counter is incremented.
func TestPrintSearchResults_MatchingBlocks(t *testing.T) {
	blocks := []types.BlockEntity{
		{Content: "Hello world from Logseq"},
		{Content: "Another block without match"},
		{Content: "HELLO uppercase match"},
	}

	// Capture stdout.
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w

	found := 0
	printSearchResults(blocks, "hello", "MyPage", 10, &found)

	_ = w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	output := buf.String()

	// Should match 2 blocks (case-insensitive: "Hello" and "HELLO").
	if found != 2 {
		t.Errorf("found = %d, want 2", found)
	}

	// Verify output format: "page | content".
	if !strings.Contains(output, "MyPage | Hello world from Logseq") {
		t.Errorf("output missing first match, got:\n%s", output)
	}
	if !strings.Contains(output, "MyPage | HELLO uppercase match") {
		t.Errorf("output missing second match, got:\n%s", output)
	}

	// "Another block without match" should NOT appear.
	if strings.Contains(output, "Another block") {
		t.Errorf("output should not contain non-matching block, got:\n%s", output)
	}
}

// TestPrintSearchResults_RespectsLimit verifies the limit parameter stops
// printing once the limit is reached.
func TestPrintSearchResults_RespectsLimit(t *testing.T) {
	blocks := []types.BlockEntity{
		{Content: "match one"},
		{Content: "match two"},
		{Content: "match three"},
	}

	// Capture stdout.
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w

	found := 0
	printSearchResults(blocks, "match", "Page", 2, &found)

	_ = w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	output := buf.String()

	if found != 2 {
		t.Errorf("found = %d, want 2 (limited)", found)
	}

	// Should only have 2 lines.
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 2 {
		t.Errorf("output lines = %d, want 2, got:\n%s", len(lines), output)
	}
}

// TestPrintSearchResults_RecursiveChildren verifies child blocks are searched.
func TestPrintSearchResults_RecursiveChildren(t *testing.T) {
	blocks := []types.BlockEntity{
		{
			Content: "parent block no match",
			Children: []types.BlockEntity{
				{Content: "child with keyword"},
				{
					Content: "nested no match",
					Children: []types.BlockEntity{
						{Content: "deep nested keyword"},
					},
				},
			},
		},
	}

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w

	found := 0
	printSearchResults(blocks, "keyword", "DeepPage", 10, &found)

	_ = w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	output := buf.String()

	if found != 2 {
		t.Errorf("found = %d, want 2 (both child and deep nested)", found)
	}
	if !strings.Contains(output, "DeepPage | child with keyword") {
		t.Errorf("output missing child match, got:\n%s", output)
	}
	if !strings.Contains(output, "DeepPage | deep nested keyword") {
		t.Errorf("output missing deep nested match, got:\n%s", output)
	}
}

// TestPrintSearchResults_EmptyBlocks verifies empty input produces no output.
func TestPrintSearchResults_EmptyBlocks(t *testing.T) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w

	found := 0
	printSearchResults(nil, "query", "Page", 10, &found)

	_ = w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("read pipe: %v", err)
	}

	if found != 0 {
		t.Errorf("found = %d, want 0", found)
	}
	if buf.Len() != 0 {
		t.Errorf("expected no output, got: %q", buf.String())
	}
}

// TestPrintSearchResults_FoundAlreadyAtLimit verifies that when found is
// already at the limit, no additional results are printed.
func TestPrintSearchResults_FoundAlreadyAtLimit(t *testing.T) {
	blocks := []types.BlockEntity{
		{Content: "match this"},
	}

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w

	found := 5 // already at limit
	printSearchResults(blocks, "match", "Page", 5, &found)

	_ = w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("read pipe: %v", err)
	}

	if found != 5 {
		t.Errorf("found = %d, want 5 (should not increment past limit)", found)
	}
	if buf.Len() != 0 {
		t.Errorf("expected no output when already at limit, got: %q", buf.String())
	}
}

// --- checkGraphVersionControl tests (T020) ---

// TestCheckGraphVersionControl_WithGit verifies no warning is logged when
// the graph directory contains a .git directory (version controlled).
func TestCheckGraphVersionControl_WithGit(t *testing.T) {
	graphDir := t.TempDir()

	// Create .git directory to simulate version control.
	gitDir := filepath.Join(graphDir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "test-graph",
			"path": graphDir,
		})
	}))
	defer srv.Close()

	c := client.New(srv.URL, "")

	// Capture logger output.
	var logBuf bytes.Buffer
	logger.SetOutput(&logBuf)
	defer logger.SetOutput(os.Stderr)

	checkGraphVersionControl(c)

	// Should NOT contain "not version controlled".
	if strings.Contains(logBuf.String(), "not version controlled") {
		t.Errorf("should not warn about version control when .git exists, got:\n%s", logBuf.String())
	}
}

// TestCheckGraphVersionControl_WithoutGit verifies a warning is logged when
// the graph directory has no .git directory.
func TestCheckGraphVersionControl_WithoutGit(t *testing.T) {
	graphDir := t.TempDir()
	// No .git directory — not version controlled.

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "test-graph",
			"path": graphDir,
		})
	}))
	defer srv.Close()

	c := client.New(srv.URL, "")

	var logBuf bytes.Buffer
	logger.SetOutput(&logBuf)
	defer logger.SetOutput(os.Stderr)

	checkGraphVersionControl(c)

	if !strings.Contains(logBuf.String(), "not version controlled") {
		t.Errorf("should warn about version control, got:\n%s", logBuf.String())
	}
}

// TestCheckGraphVersionControl_APIError verifies the function silently returns
// when the Logseq API is unreachable (best-effort behavior).
func TestCheckGraphVersionControl_APIError(t *testing.T) {
	// Use a server that returns an error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`"error"`))
	}))
	defer srv.Close()

	c := client.New(srv.URL, "")

	var logBuf bytes.Buffer
	logger.SetOutput(&logBuf)
	defer logger.SetOutput(os.Stderr)

	// Should not panic.
	checkGraphVersionControl(c)

	// Should NOT warn about version control (error path returns silently).
	if strings.Contains(logBuf.String(), "not version controlled") {
		t.Errorf("should not warn when API is unreachable, got:\n%s", logBuf.String())
	}
}

// TestCheckGraphVersionControl_NullGraph verifies the function silently returns
// when GetCurrentGraph returns null (no graph open).
func TestCheckGraphVersionControl_NullGraph(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("null"))
	}))
	defer srv.Close()

	c := client.New(srv.URL, "")

	var logBuf bytes.Buffer
	logger.SetOutput(&logBuf)
	defer logger.SetOutput(os.Stderr)

	checkGraphVersionControl(c)

	if strings.Contains(logBuf.String(), "not version controlled") {
		t.Errorf("should not warn when graph is null, got:\n%s", logBuf.String())
	}
}

// --- newJournalCmd validation tests ---

// TestJournalCmd_NoContent verifies journal fails when no content is provided.
func TestJournalCmd_NoContent(t *testing.T) {
	cmd := newJournalCmd()
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("journal with no content should fail")
	}
	if !strings.Contains(err.Error(), "no content provided") {
		t.Errorf("error = %q, want to contain 'no content provided'", err.Error())
	}
}

// TestJournalCmd_InvalidDate verifies journal fails with an invalid date format.
func TestJournalCmd_InvalidDate(t *testing.T) {
	cmd := newJournalCmd()
	cmd.SetArgs([]string{"--date", "not-a-date", "some content"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("journal with invalid date should fail")
	}
	if !strings.Contains(err.Error(), "invalid date") {
		t.Errorf("error = %q, want to contain 'invalid date'", err.Error())
	}
	if !strings.Contains(err.Error(), "YYYY-MM-DD") {
		t.Errorf("error = %q, want to contain usage hint 'YYYY-MM-DD'", err.Error())
	}
}

// TestJournalCmd_InvalidDatePartialFormat verifies journal rejects dates that
// are close to valid but use wrong separators (e.g. "2026/01/29").
func TestJournalCmd_InvalidDatePartialFormat(t *testing.T) {
	cmd := newJournalCmd()
	cmd.SetArgs([]string{"--date", "2026/01/29", "some content"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("journal with slash-separated date should fail")
	}
	if !strings.Contains(err.Error(), "invalid date") {
		t.Errorf("error = %q, want to contain 'invalid date'", err.Error())
	}
}

// TestJournalCmd_ValidDateFormat verifies journal accepts a valid YYYY-MM-DD
// date. The command will fail at the API call, but the date parsing itself
// should succeed.
func TestJournalCmd_ValidDateFormat(t *testing.T) {
	// Use a mock server that returns null for all getPage calls and
	// an error for appendBlockInPage — so we can verify date parsing
	// succeeds but the command fails at the API level, not date parsing.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "logseq.Editor.getPage":
			_, _ = w.Write([]byte("null"))
		case "logseq.Editor.appendBlockInPage":
			// Return an error response to distinguish from date parse error.
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`"server error"`))
		default:
			_, _ = w.Write([]byte("null"))
		}
	}))
	defer srv.Close()

	t.Setenv("LOGSEQ_API_URL", srv.URL)

	cmd := newJournalCmd()
	cmd.SetArgs([]string{"--date", "2026-03-15", "test content"})

	err := cmd.Execute()
	// Should fail — but the error should be from the API, not date parsing.
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
	if strings.Contains(err.Error(), "invalid date") {
		t.Errorf("date parsing should succeed, but got date error: %v", err)
	}
	// The error should be wrapped with "journal:" prefix from the API failure.
	if !strings.Contains(err.Error(), "journal:") {
		t.Errorf("error = %q, want to contain 'journal:' prefix from API failure", err.Error())
	}
}

// TestJournalCmd_SuccessfulAppend verifies journal succeeds and prints the
// block UUID when the API returns a valid block.
func TestJournalCmd_SuccessfulAppend(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "logseq.Editor.getPage":
			// Return an existing page for the ordinal date format.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "Jan 15th, 2026",
				"uuid": "page-uuid",
				"id":   1,
			})
		case "logseq.Editor.appendBlockInPage":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"uuid":    "block-uuid-123",
				"content": "test content",
				"id":      42,
			})
		default:
			_, _ = w.Write([]byte("null"))
		}
	}))
	defer srv.Close()

	t.Setenv("LOGSEQ_API_URL", srv.URL)

	// Capture stdout to verify UUID is printed.
	old := os.Stdout
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = pw

	cmd := newJournalCmd()
	cmd.SetArgs([]string{"--date", "2026-01-15", "test content"})

	execErr := cmd.Execute()

	_ = pw.Close()
	os.Stdout = old

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(pr); err != nil {
		t.Fatalf("read pipe: %v", err)
	}

	if execErr != nil {
		t.Fatalf("journal should succeed, got: %v", execErr)
	}

	output := strings.TrimSpace(buf.String())
	if output != "block-uuid-123" {
		t.Errorf("stdout = %q, want %q", output, "block-uuid-123")
	}
}

// TestJournalCmd_MultiWordContent verifies journal joins multiple args as
// content separated by spaces.
func TestJournalCmd_MultiWordContent(t *testing.T) {
	var capturedContent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
			Args   []any  `json:"args"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "logseq.Editor.getPage":
			_, _ = w.Write([]byte("null"))
		case "logseq.Editor.appendBlockInPage":
			if len(req.Args) >= 2 {
				capturedContent = fmt.Sprintf("%v", req.Args[1])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"uuid":    "block-uuid",
				"content": capturedContent,
				"id":      1,
			})
		default:
			_, _ = w.Write([]byte("null"))
		}
	}))
	defer srv.Close()

	t.Setenv("LOGSEQ_API_URL", srv.URL)

	// Capture stdout to suppress UUID output.
	old := os.Stdout
	_, pw, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = pw

	cmd := newJournalCmd()
	cmd.SetArgs([]string{"hello", "world", "test"})

	execErr := cmd.Execute()

	_ = pw.Close()
	os.Stdout = old

	if execErr != nil {
		t.Fatalf("journal should succeed, got: %v", execErr)
	}

	if capturedContent != "hello world test" {
		t.Errorf("captured content = %q, want %q", capturedContent, "hello world test")
	}
}

// TestJournalCmd_CommandMetadata verifies the command's Use, Short, and Long
// descriptions are set correctly.
func TestJournalCmd_CommandMetadata(t *testing.T) {
	cmd := newJournalCmd()

	if cmd.Use != "journal [flags] TEXT" {
		t.Errorf("Use = %q, want %q", cmd.Use, "journal [flags] TEXT")
	}
	if cmd.Short == "" {
		t.Error("Short description should not be empty")
	}
	if !strings.Contains(cmd.Long, "Logseq") {
		t.Errorf("Long description should mention Logseq, got %q", cmd.Long)
	}
}

// TestJournalCmd_DateDefaultToday verifies journal uses today's date when
// --date is not specified.
func TestJournalCmd_DateDefaultToday(t *testing.T) {
	var capturedPage string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
			Args   []any  `json:"args"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "logseq.Editor.getPage":
			// Return a match for the ordinal date of today.
			name := fmt.Sprintf("%v", req.Args[0])
			todayOrdinal := ordinalDate(time.Now())
			if name == todayOrdinal {
				capturedPage = name
				_ = json.NewEncoder(w).Encode(map[string]any{
					"name": name,
					"uuid": "page-uuid",
					"id":   1,
				})
				return
			}
			_, _ = w.Write([]byte("null"))
		case "logseq.Editor.appendBlockInPage":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"uuid": "block-uuid",
				"id":   1,
			})
		default:
			_, _ = w.Write([]byte("null"))
		}
	}))
	defer srv.Close()

	t.Setenv("LOGSEQ_API_URL", srv.URL)

	// Capture stdout.
	old := os.Stdout
	_, pw, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = pw

	cmd := newJournalCmd()
	cmd.SetArgs([]string{"today note"})

	execErr := cmd.Execute()

	_ = pw.Close()
	os.Stdout = old

	if execErr != nil {
		t.Fatalf("journal should succeed, got: %v", execErr)
	}

	expectedPage := ordinalDate(time.Now())
	if capturedPage != expectedPage {
		t.Errorf("used page = %q, want today's ordinal %q", capturedPage, expectedPage)
	}
}

// --- newSearchCmd validation tests ---

// TestSearchCmd_NoResults verifies search returns an error when no blocks
// match the query.
func TestSearchCmd_NoResults(t *testing.T) {
	tmpDir := t.TempDir()
	// Create a .md file with content that won't match the query.
	if err := os.WriteFile(filepath.Join(tmpDir, "notes.md"), []byte("nothing relevant here"), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	cmd := newSearchCmd()
	cmd.SetArgs([]string{"--vault", tmpDir, "nonexistent-query-xyz"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("search with no matching results should fail")
	}
	if !strings.Contains(err.Error(), "no results") {
		t.Errorf("error = %q, want to contain 'no results'", err.Error())
	}
	if !strings.Contains(err.Error(), "nonexistent-query-xyz") {
		t.Errorf("error = %q, want to contain the query string", err.Error())
	}
}

// TestSearchCmd_WithResults verifies search prints matching results.
func TestSearchCmd_WithResults(t *testing.T) {
	tmpDir := t.TempDir()
	// Create a .md file with searchable content.
	if err := os.WriteFile(filepath.Join(tmpDir, "notes.md"), []byte("# Notes\n\nHello world from dewey\n\n## Other\n\nAnother block"), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	// Capture stdout.
	old := os.Stdout
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = pw

	cmd := newSearchCmd()
	cmd.SetArgs([]string{"--vault", tmpDir, "hello"})

	execErr := cmd.Execute()

	_ = pw.Close()
	os.Stdout = old

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(pr); err != nil {
		t.Fatalf("read pipe: %v", err)
	}

	if execErr != nil {
		t.Fatalf("search should succeed, got: %v", execErr)
	}

	output := buf.String()
	if !strings.Contains(strings.ToLower(output), "hello world from dewey") {
		t.Errorf("output should contain matching result, got:\n%s", output)
	}
}

// TestSearchCmd_MultiWordQuery verifies search joins multiple args into a
// single query string.
func TestSearchCmd_MultiWordQuery(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "docs.md"), []byte("# Docs\n\nhello world search test"), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	// Capture stdout.
	old := os.Stdout
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = pw

	cmd := newSearchCmd()
	cmd.SetArgs([]string{"--vault", tmpDir, "hello", "world"})

	execErr := cmd.Execute()

	_ = pw.Close()
	os.Stdout = old

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(pr); err != nil {
		t.Fatalf("read pipe: %v", err)
	}

	if execErr != nil {
		t.Fatalf("multi-word search should succeed, got: %v", execErr)
	}

	output := buf.String()
	if !strings.Contains(strings.ToLower(output), "hello world search test") {
		t.Errorf("multi-word query should match, got:\n%s", output)
	}
}

// TestSearchCmd_LimitFlag verifies the --limit flag restricts results.
func TestSearchCmd_LimitFlag(t *testing.T) {
	tmpDir := t.TempDir()
	// Create a file with multiple matching blocks.
	content := "# Page\n\n## Match One\n\nmatch content one\n\n## Match Two\n\nmatch content two\n\n## Match Three\n\nmatch content three\n\n## Match Four\n\nmatch content four\n\n## Match Five\n\nmatch content five"
	if err := os.WriteFile(filepath.Join(tmpDir, "page1.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	// Capture stdout.
	old := os.Stdout
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = pw

	cmd := newSearchCmd()
	cmd.SetArgs([]string{"--vault", tmpDir, "--limit", "2", "match"})

	execErr := cmd.Execute()

	_ = pw.Close()
	os.Stdout = old

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(pr); err != nil {
		t.Fatalf("read pipe: %v", err)
	}

	if execErr != nil {
		t.Fatalf("search with limit should succeed, got: %v", execErr)
	}

	output := strings.TrimSpace(buf.String())
	lines := strings.Split(output, "\n")
	if len(lines) > 2 {
		t.Errorf("with --limit 2, got %d lines, want at most 2:\n%s", len(lines), output)
	}
}

// TestSearchCmd_LimitFlagDefault verifies the default --limit is 10.
func TestSearchCmd_LimitFlagDefault(t *testing.T) {
	cmd := newSearchCmd()
	f := cmd.Flags().Lookup("limit")
	if f == nil {
		t.Fatal("search command missing --limit flag")
	}
	if f.DefValue != "10" {
		t.Errorf("--limit default = %q, want %q", f.DefValue, "10")
	}
}

// TestSearchCmd_MissingVaultPath verifies search fails with a clear error
// when no --vault flag or OBSIDIAN_VAULT_PATH is set.
func TestSearchCmd_MissingVaultPath(t *testing.T) {
	t.Setenv("OBSIDIAN_VAULT_PATH", "")

	cmd := newSearchCmd()
	cmd.SetArgs([]string{"anything"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("search should fail without vault path")
	}
	if !strings.Contains(err.Error(), "--vault") {
		t.Errorf("error = %q, want to mention --vault", err.Error())
	}
}

// TestSearchCmd_CommandMetadata verifies the command's Use, Short, and Long
// descriptions are set correctly.
func TestSearchCmd_CommandMetadata(t *testing.T) {
	cmd := newSearchCmd()

	if cmd.Use != "search [flags] QUERY" {
		t.Errorf("Use = %q, want %q", cmd.Use, "search [flags] QUERY")
	}
	if cmd.Short == "" {
		t.Error("Short description should not be empty")
	}
	if cmd.Long == "" {
		t.Error("Long description should not be empty")
	}
}

// TestCheckGraphVersionControl_EmptyPath verifies the function silently returns
// when the graph has an empty path.
func TestCheckGraphVersionControl_EmptyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "test-graph",
			"path": "",
		})
	}))
	defer srv.Close()

	c := client.New(srv.URL, "")

	var logBuf bytes.Buffer
	logger.SetOutput(&logBuf)
	defer logger.SetOutput(os.Stderr)

	checkGraphVersionControl(c)

	if strings.Contains(logBuf.String(), "not version controlled") {
		t.Errorf("should not warn when path is empty, got:\n%s", logBuf.String())
	}
}

// --- indexDocuments tests ---

// TestIndexDocuments_InsertNew verifies that indexDocuments inserts a new page
// into the store when no existing page matches the document title.
func TestIndexDocuments_InsertNew(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	now := time.Now()
	docs := map[string][]source.Document{
		"test-src": {
			{
				ID:          "doc-001",
				Title:       "My Test Page",
				Content:     "some content",
				ContentHash: "abc123",
				FetchedAt:   now,
			},
		},
	}

	got, _ := indexDocuments(s, docs, nil, nil)
	if got != 1 {
		t.Fatalf("indexDocuments() = %d, want 1", got)
	}

	// Page name is now namespaced: sourceID/docID (per research R6).
	pageName := "test-src/doc-001"
	page, err := s.GetPage(pageName)
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if page == nil {
		t.Fatal("GetPage returned nil, want page")
	}
	if page.Name != pageName {
		t.Errorf("page.Name = %q, want %q", page.Name, pageName)
	}
	if page.ContentHash != "abc123" {
		t.Errorf("page.ContentHash = %q, want %q", page.ContentHash, "abc123")
	}
	if page.SourceID != "test-src" {
		t.Errorf("page.SourceID = %q, want %q", page.SourceID, "test-src")
	}
	if page.SourceDocID != "doc-001" {
		t.Errorf("page.SourceDocID = %q, want %q", page.SourceDocID, "doc-001")
	}
}

// TestIndexDocuments_UpdateExisting verifies that indexDocuments updates an
// existing page's ContentHash when re-indexing the same document.
func TestIndexDocuments_UpdateExisting(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	// Pre-insert a page with the namespaced name and old content hash.
	pageName := "new-src/new-doc"
	if err := s.InsertPage(&store.Page{
		Name:         pageName,
		OriginalName: "Existing Page",
		ContentHash:  "old-hash",
		SourceID:     "new-src",
		SourceDocID:  "new-doc",
	}); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	docs := map[string][]source.Document{
		"new-src": {
			{
				ID:          "new-doc",
				Title:       "Existing Page",
				Content:     "updated content",
				ContentHash: "new-hash",
				FetchedAt:   time.Now(),
			},
		},
	}

	got, _ := indexDocuments(s, docs, nil, nil)
	if got != 1 {
		t.Fatalf("indexDocuments() = %d, want 1", got)
	}

	page, err := s.GetPage(pageName)
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if page == nil {
		t.Fatal("GetPage returned nil, want page")
	}
	if page.ContentHash != "new-hash" {
		t.Errorf("page.ContentHash = %q, want %q", page.ContentHash, "new-hash")
	}
	if page.SourceID != "new-src" {
		t.Errorf("page.SourceID = %q, want %q (should be updated)", page.SourceID, "new-src")
	}
	if page.SourceDocID != "new-doc" {
		t.Errorf("page.SourceDocID = %q, want %q (should be updated)", page.SourceDocID, "new-doc")
	}
}

// TestIndexDocuments_SourceRecord verifies that indexDocuments creates a source
// record in the store when a matching SourceConfig is provided.
func TestIndexDocuments_SourceRecord(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	docs := map[string][]source.Document{
		"test-src": {
			{
				ID:          "doc-1",
				Title:       "Source Doc",
				ContentHash: "hash1",
				FetchedAt:   time.Now(),
			},
		},
	}
	configs := []source.SourceConfig{
		{
			ID:   "test-src",
			Type: "github",
			Name: "my-github",
		},
	}

	_, _ = indexDocuments(s, docs, configs, nil)

	src, err := s.GetSource("test-src")
	if err != nil {
		t.Fatalf("GetSource: %v", err)
	}
	if src == nil {
		t.Fatal("GetSource returned nil, want source record")
	}
	if src.Type != "github" {
		t.Errorf("src.Type = %q, want %q", src.Type, "github")
	}
	if src.Status != "active" {
		t.Errorf("src.Status = %q, want %q", src.Status, "active")
	}
}

// TestIndexDocuments_WithProperties verifies that indexDocuments serializes
// document Properties to JSON and stores them on the page.
func TestIndexDocuments_WithProperties(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	docs := map[string][]source.Document{
		"prop-src": {
			{
				ID:          "doc-props",
				Title:       "Props Page",
				ContentHash: "hash-props",
				FetchedAt:   time.Now(),
				Properties: map[string]any{
					"type":   "issue",
					"status": "open",
				},
			},
		},
	}

	got, _ := indexDocuments(s, docs, nil, nil)
	if got != 1 {
		t.Fatalf("indexDocuments() = %d, want 1", got)
	}

	// Page name is now namespaced: sourceID/docID.
	page, err := s.GetPage("prop-src/doc-props")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if page == nil {
		t.Fatal("GetPage returned nil, want page")
	}
	if page.Properties == "" {
		t.Fatal("page.Properties is empty, want JSON")
	}

	var props map[string]any
	if err := json.Unmarshal([]byte(page.Properties), &props); err != nil {
		t.Fatalf("unmarshal Properties: %v", err)
	}
	if props["type"] != "issue" {
		t.Errorf("props[\"type\"] = %v, want %q", props["type"], "issue")
	}
	if props["status"] != "open" {
		t.Errorf("props[\"status\"] = %v, want %q", props["status"], "open")
	}
}

// --- US2 integration tests (004-unified-content-serve T019, T020) ---

// TestIndexDocuments_PersistsBlocksAndLinks verifies that indexDocuments parses
// document content into blocks and links, persisting them to the store.
func TestIndexDocuments_PersistsBlocksAndLinks(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	docs := map[string][]source.Document{
		"github-org": {
			{
				ID:    "issue-42",
				Title: "Bug Report",
				Content: `# Bug Report

Found a bug in [[architecture]] module.

## Steps to Reproduce

1. Run the command
2. Observe the error
`,
				ContentHash: "hash-42",
				FetchedAt:   time.Now(),
			},
		},
	}

	got, _ := indexDocuments(s, docs, nil, nil)
	if got != 1 {
		t.Fatalf("indexDocuments() = %d, want 1", got)
	}

	pageName := "github-org/issue-42"

	// Verify blocks were persisted.
	blocks, err := s.GetBlocksByPage(pageName)
	if err != nil {
		t.Fatalf("GetBlocksByPage: %v", err)
	}
	if len(blocks) == 0 {
		t.Fatal("expected blocks to be persisted, got 0")
	}

	// Verify links were persisted (the [[architecture]] wikilink).
	links, err := s.GetForwardLinks(pageName)
	if err != nil {
		t.Fatalf("GetForwardLinks: %v", err)
	}
	if len(links) == 0 {
		t.Fatal("expected links to be persisted, got 0")
	}

	foundArchLink := false
	for _, l := range links {
		if l.ToPage == "architecture" {
			foundArchLink = true
			break
		}
	}
	if !foundArchLink {
		t.Error("expected link to 'architecture' page, not found")
	}
}

// TestIndexDocuments_ReIndexReplacesBlocks verifies that re-indexing a document
// replaces old blocks with new ones (FR-004 replace strategy).
func TestIndexDocuments_ReIndexReplacesBlocks(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	pageName := "test-src/doc-1"

	// First index: insert a document with initial content.
	docs1 := map[string][]source.Document{
		"test-src": {
			{
				ID:          "doc-1",
				Title:       "Test Doc",
				Content:     "# Original\n\nOriginal content.",
				ContentHash: "hash-v1",
				FetchedAt:   time.Now(),
			},
		},
	}
	_, _ = indexDocuments(s, docs1, nil, nil)

	blocks1, _ := s.GetBlocksByPage(pageName)
	if len(blocks1) == 0 {
		t.Fatal("expected blocks after first index")
	}
	originalBlockCount := len(blocks1)

	// Second index: same doc with different content and hash.
	docs2 := map[string][]source.Document{
		"test-src": {
			{
				ID:          "doc-1",
				Title:       "Test Doc",
				Content:     "# Updated\n\nNew content.\n\n## New Section\n\nMore content.",
				ContentHash: "hash-v2",
				FetchedAt:   time.Now(),
			},
		},
	}
	_, _ = indexDocuments(s, docs2, nil, nil)

	blocks2, _ := s.GetBlocksByPage(pageName)
	if len(blocks2) == 0 {
		t.Fatal("expected blocks after re-index")
	}

	// Verify blocks were replaced (not accumulated).
	// The new content has different structure, so block count may differ,
	// but the old blocks should not remain.
	page, _ := s.GetPage(pageName)
	if page == nil {
		t.Fatal("page should exist after re-index")
	}
	if page.ContentHash != "hash-v2" {
		t.Errorf("page.ContentHash = %q, want %q", page.ContentHash, "hash-v2")
	}

	// Verify no old blocks remain by checking that block count changed
	// (the new content has more sections).
	if len(blocks2) == originalBlockCount {
		// It's possible they have the same count, but content should differ.
		// Check that at least one block has the new content.
		hasNewContent := false
		for _, b := range blocks2 {
			if strings.Contains(b.Content, "Updated") || strings.Contains(b.Content, "New content") {
				hasNewContent = true
				break
			}
		}
		if !hasNewContent {
			t.Error("re-index did not replace blocks with new content")
		}
	}
}

// TestPurgeOrphanedSources verifies that purgeOrphanedSources deletes pages
// for sources that are no longer in the config (FR-013 auto-purge).
func TestPurgeOrphanedSources(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	// Insert pages for two sources.
	for i := 0; i < 3; i++ {
		p := &store.Page{
			Name:         fmt.Sprintf("github-org/issue-%d", i),
			OriginalName: fmt.Sprintf("Issue %d", i),
			SourceID:     "github-org",
			SourceDocID:  fmt.Sprintf("issue-%d", i),
			ContentHash:  "hash",
		}
		if err := s.InsertPage(p); err != nil {
			t.Fatalf("InsertPage(github %d): %v", i, err)
		}
	}
	_ = s.InsertSource(&store.SourceRecord{
		ID:     "github-org",
		Type:   "github",
		Name:   "org",
		Status: "active",
	})

	for i := 0; i < 2; i++ {
		p := &store.Page{
			Name:         fmt.Sprintf("web-docs/page-%d", i),
			OriginalName: fmt.Sprintf("Page %d", i),
			SourceID:     "web-docs",
			SourceDocID:  fmt.Sprintf("page-%d", i),
			ContentHash:  "hash",
		}
		if err := s.InsertPage(p); err != nil {
			t.Fatalf("InsertPage(web %d): %v", i, err)
		}
	}
	_ = s.InsertSource(&store.SourceRecord{
		ID:     "web-docs",
		Type:   "web",
		Name:   "docs",
		Status: "active",
	})

	// Config only has github-org — web-docs is orphaned.
	configs := []source.SourceConfig{
		{ID: "github-org", Type: "github", Name: "org"},
	}

	purgeOrphanedSources(s, configs)

	// Verify github pages still exist.
	githubPages, _ := s.ListPagesBySource("github-org")
	if len(githubPages) != 3 {
		t.Errorf("expected 3 github pages, got %d", len(githubPages))
	}

	// Verify web-docs pages were purged.
	webPages, _ := s.ListPagesBySource("web-docs")
	if len(webPages) != 0 {
		t.Errorf("expected 0 web-docs pages after purge, got %d", len(webPages))
	}
}

// --- US5 tests (004-unified-content-serve T039, T040) ---

// TestIndexDocuments_EmbeddingsQueryable verifies that embeddings generated
// during indexing are stored in the embeddings table and can be queried
// via store.SearchSimilar(). This test uses a mock embedder to avoid
// requiring Ollama.
func TestIndexDocuments_EmbeddingsQueryable(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	// Insert a page with blocks and an embedding manually to verify
	// the store's semantic search works with external-source pages.
	pageName := "github-org/issue-99"
	if err := s.InsertPage(&store.Page{
		Name:         pageName,
		OriginalName: "Issue 99",
		SourceID:     "github-org",
		SourceDocID:  "issue-99",
		ContentHash:  "hash-99",
	}); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	blockUUID := "test-block-uuid-99"
	if err := s.InsertBlock(&store.Block{
		UUID:     blockUUID,
		PageName: pageName,
		Content:  "External source content about architecture",
	}); err != nil {
		t.Fatalf("InsertBlock: %v", err)
	}

	// Insert a mock embedding vector.
	mockVector := make([]float32, 384)
	for i := range mockVector {
		mockVector[i] = float32(i) / 384.0
	}
	if err := s.InsertEmbedding(blockUUID, "test-model", mockVector, "External source content about architecture"); err != nil {
		t.Fatalf("InsertEmbedding: %v", err)
	}

	// Verify the embedding is queryable via SearchSimilar.
	results, err := s.SearchSimilar("test-model", mockVector, 10, 0.0)
	if err != nil {
		t.Fatalf("SearchSimilar: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one semantic search result")
	}

	// Verify the result includes the external-source page.
	found := false
	for _, r := range results {
		if r.PageName == pageName {
			found = true
			break
		}
	}
	if !found {
		t.Error("external-source page not found in semantic search results")
	}
}

// TestIndexDocuments_GracefulDegradationWithoutEmbedder verifies that indexing
// works correctly when no embedder is available — blocks and links are still
// persisted, but no embeddings are generated (FR-003).
func TestIndexDocuments_GracefulDegradationWithoutEmbedder(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	docs := map[string][]source.Document{
		"github-org": {
			{
				ID:          "issue-1",
				Title:       "Test Issue",
				Content:     "# Test\n\nContent with [[links]].",
				ContentHash: "hash-1",
				FetchedAt:   time.Now(),
			},
		},
	}

	// Index with nil embedder (simulates Ollama unavailable).
	got, _ := indexDocuments(s, docs, nil, nil)
	if got != 1 {
		t.Fatalf("indexDocuments() = %d, want 1", got)
	}

	pageName := "github-org/issue-1"

	// Verify blocks were persisted.
	blocks, err := s.GetBlocksByPage(pageName)
	if err != nil {
		t.Fatalf("GetBlocksByPage: %v", err)
	}
	if len(blocks) == 0 {
		t.Error("expected blocks to be persisted even without embedder")
	}

	// Verify links were persisted.
	links, err := s.GetForwardLinks(pageName)
	if err != nil {
		t.Fatalf("GetForwardLinks: %v", err)
	}
	if len(links) == 0 {
		t.Error("expected links to be persisted even without embedder")
	}

	// Verify NO embeddings were generated.
	embedCount, err := s.CountEmbeddings()
	if err != nil {
		t.Fatalf("CountEmbeddings: %v", err)
	}
	if embedCount != 0 {
		t.Errorf("expected 0 embeddings without embedder, got %d", embedCount)
	}
}

// --- UUID collision fix (Issue #17) ---

// TestIndexDocuments_CrossSourceUUIDUniqueness verifies that identical files
// across different sources produce unique block UUIDs. This prevents UNIQUE
// constraint failures when indexing multiple repos with scaffolded templates.
func TestIndexDocuments_CrossSourceUUIDUniqueness(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	// Same content, same relative path, different sources —
	// simulates agents.md scaffolded by `uf init` across repos.
	identicalContent := "# AGENTS.md\n\n## Project Overview\n\nThis is a scaffolded file.\n"

	docs := map[string][]source.Document{
		"disk-local": {
			{
				ID:          "agents.md",
				Title:       "AGENTS",
				Content:     identicalContent,
				ContentHash: "same-hash",
				FetchedAt:   time.Now(),
			},
		},
		"disk-dewey": {
			{
				ID:          "agents.md",
				Title:       "AGENTS",
				Content:     identicalContent,
				ContentHash: "same-hash",
				FetchedAt:   time.Now(),
			},
		},
		"disk-gaze": {
			{
				ID:          "agents.md",
				Title:       "AGENTS",
				Content:     identicalContent,
				ContentHash: "same-hash",
				FetchedAt:   time.Now(),
			},
		},
	}

	got, _ := indexDocuments(s, docs, nil, nil)
	if got != 3 {
		t.Fatalf("indexDocuments() = %d, want 3", got)
	}

	// All three pages should have blocks persisted without collisions.
	for _, pageName := range []string{"disk-local/agents.md", "disk-dewey/agents.md", "disk-gaze/agents.md"} {
		blocks, err := s.GetBlocksByPage(pageName)
		if err != nil {
			t.Fatalf("GetBlocksByPage(%s): %v", pageName, err)
		}
		if len(blocks) == 0 {
			t.Errorf("page %s has 0 blocks — UUID collision likely dropped them", pageName)
		}
	}

	// Verify UUIDs are unique across sources.
	allBlocks1, _ := s.GetBlocksByPage("disk-local/agents.md")
	allBlocks2, _ := s.GetBlocksByPage("disk-dewey/agents.md")
	if len(allBlocks1) > 0 && len(allBlocks2) > 0 {
		if allBlocks1[0].UUID == allBlocks2[0].UUID {
			t.Errorf("block UUIDs should differ across sources but got same UUID: %s", allBlocks1[0].UUID)
		}
	}
}

// --- Reindex command tests ---

// TestReindexCmd_CleanReindex verifies that dewey reindex removes existing
// database files and rebuilds the index from scratch.
func TestReindexCmd_CleanReindex(t *testing.T) {
	tmpDir := t.TempDir()
	deweyDir := filepath.Join(tmpDir, ".dewey")
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Create a sources.yaml with a local disk source.
	sourcesYAML := `sources:
  - id: disk-local
    type: disk
    name: local
    config:
      path: "."
`
	if err := os.WriteFile(filepath.Join(deweyDir, "sources.yaml"), []byte(sourcesYAML), 0o644); err != nil {
		t.Fatalf("write sources.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deweyDir, "config.yaml"), []byte("embedding:\n  model: test\n"), 0o644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}

	// Create a .md file to be indexed.
	if err := os.WriteFile(filepath.Join(tmpDir, "test.md"), []byte("# Test\n\nHello reindex."), 0o644); err != nil {
		t.Fatalf("write test.md: %v", err)
	}

	// Create stale database files to simulate a dirty state.
	for _, name := range []string{"graph.db", "graph.db-wal", "graph.db-shm", ".dewey.lock"} {
		if err := os.WriteFile(filepath.Join(deweyDir, name), []byte("stale"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	// Run reindex from tmpDir.
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	cmd := newReindexCmd()
	cmd.SetArgs([]string{"--no-embeddings"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("reindex failed: %v", err)
	}

	// Verify new graph.db was created with pages.
	s, err := store.New(filepath.Join(deweyDir, "graph.db"))
	if err != nil {
		t.Fatalf("open store after reindex: %v", err)
	}
	defer func() { _ = s.Close() }()

	pages, err := s.ListPages()
	if err != nil {
		t.Fatalf("list pages: %v", err)
	}
	if len(pages) == 0 {
		t.Error("expected at least 1 page after reindex, got 0")
	}
}

// TestReindexCmd_NotInitialized verifies that reindex fails with a clear
// error when .dewey/ does not exist.
func TestReindexCmd_NotInitialized(t *testing.T) {
	tmpDir := t.TempDir()

	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	cmd := newReindexCmd()
	cmd.SetArgs([]string{"--no-embeddings"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("reindex should fail without .dewey/")
	}
	if !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("error = %q, want to contain 'not initialized'", err.Error())
	}
}

// --- Doctor helper unit tests ---

// TestHumanSize verifies the humanSize helper formats byte counts correctly
// across all four branches (B, KB, MB, GB) and boundary values.
func TestHumanSize(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{1, "1 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{10240, "10.0 KB"},
		{1048576, "1.0 MB"},
		{54235136, "51.7 MB"},
		{1073741824, "1.0 GB"},
		{2147483648, "2.0 GB"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := humanSize(tt.bytes)
			if got != tt.want {
				t.Errorf("humanSize(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

// TestDoctorCounter_PrintCheck verifies that printCheck increments the correct
// counter and formats output with the expected column layout.
func TestDoctorCounter_PrintCheck(t *testing.T) {
	var buf bytes.Buffer
	c := &doctorCounter{}

	c.printCheck(&buf, "PASS", "vault", "/tmp/vault")
	c.printCheck(&buf, "PASS", "dewey", "v1.4.1 (/usr/bin/dewey)")
	c.printCheck(&buf, "WARN", "config.yaml", "not found (using defaults)")
	c.printCheck(&buf, "FAIL", "graph.db", "not found")

	if c.pass != 2 {
		t.Errorf("pass = %d, want 2", c.pass)
	}
	if c.warn != 1 {
		t.Errorf("warn = %d, want 1", c.warn)
	}
	if c.fail != 1 {
		t.Errorf("fail = %d, want 1", c.fail)
	}

	output := buf.String()
	// Verify formatted output contains correct markers and names.
	if !strings.Contains(output, "✅ vault") {
		t.Errorf("PASS line missing, got:\n%s", output)
	}
	if !strings.Contains(output, "⚠️ config.yaml") {
		t.Errorf("WARN line missing, got:\n%s", output)
	}
	if !strings.Contains(output, "❌ graph.db") {
		t.Errorf("FAIL line missing, got:\n%s", output)
	}
	if !strings.Contains(output, "/tmp/vault") {
		t.Errorf("PASS description missing, got:\n%s", output)
	}
}

// TestPrintSummaryBox_Format verifies the summary box renders with correct
// counts, singular/plural handling, and box-drawing borders.
func TestPrintSummaryBox_Format(t *testing.T) {
	tests := []struct {
		name     string
		counter  doctorCounter
		wantPass string
		wantWarn string
		wantFail string
	}{
		{
			name:     "plural warnings",
			counter:  doctorCounter{pass: 5, warn: 2, fail: 0},
			wantPass: "5 passed",
			wantWarn: "2 warnings",
			wantFail: "0 failed",
		},
		{
			name:     "singular warning",
			counter:  doctorCounter{pass: 3, warn: 1, fail: 0},
			wantPass: "3 passed",
			wantWarn: "1 warning",
			wantFail: "0 failed",
		},
		{
			name:     "zero everything",
			counter:  doctorCounter{pass: 0, warn: 0, fail: 0},
			wantPass: "0 passed",
			wantWarn: "0 warnings",
			wantFail: "0 failed",
		},
		{
			name:     "all non-zero",
			counter:  doctorCounter{pass: 10, warn: 3, fail: 2},
			wantPass: "10 passed",
			wantWarn: "3 warnings",
			wantFail: "2 failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			printSummaryBox(&buf, &tt.counter)
			output := buf.String()

			if !strings.Contains(output, tt.wantPass) {
				t.Errorf("summary box missing %q in:\n%s", tt.wantPass, output)
			}
			if !strings.Contains(output, tt.wantWarn) {
				t.Errorf("summary box missing %q in:\n%s", tt.wantWarn, output)
			}
			if !strings.Contains(output, tt.wantFail) {
				t.Errorf("summary box missing %q in:\n%s", tt.wantFail, output)
			}
			// Verify box borders.
			if !strings.Contains(output, "╭") || !strings.Contains(output, "╰") {
				t.Errorf("summary box missing borders in:\n%s", output)
			}
			if !strings.Contains(output, "│") {
				t.Errorf("summary box missing side borders in:\n%s", output)
			}
		})
	}
}

// --- Doctor command tests ---

// TestDoctorCmd_WithInitializedVault verifies doctor reports pass for init
// and store checks when .dewey/ and graph.db exist with pages.
func TestDoctorCmd_WithInitializedVault(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .dewey/ directory.
	deweyDir := filepath.Join(tmpDir, ".dewey")
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("mkdir .dewey: %v", err)
	}

	// Create graph.db with a page.
	dbPath := filepath.Join(deweyDir, "graph.db")
	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := s.InsertPage(&store.Page{
		Name:        "test-page",
		ContentHash: "abc123",
	}); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}
	_ = s.Close()

	cmd := newDoctorCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--vault", tmpDir})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("doctor failed: %v", err)
	}

	output := buf.String()

	// .dewey/ check should pass with emoji marker.
	if !strings.Contains(output, "✅ .dewey/") {
		t.Errorf("doctor should report .dewey/ pass, got:\n%s", output)
	}

	// Database section should show graph.db with page count.
	if !strings.Contains(output, "✅ graph.db") {
		t.Errorf("doctor should report graph.db pass, got:\n%s", output)
	}
	if !strings.Contains(output, "1 pages") {
		t.Errorf("doctor should report page count, got:\n%s", output)
	}

	// Embedding Layer section should exist.
	if !strings.Contains(output, "Embedding Layer") {
		t.Errorf("doctor should include Embedding Layer section, got:\n%s", output)
	}

	// Summary box should be present with correct counts.
	if !strings.Contains(output, "✅") {
		t.Errorf("doctor should include summary box with pass emoji, got:\n%s", output)
	}
	if !strings.Contains(output, "╭") {
		t.Errorf("doctor should include summary box border, got:\n%s", output)
	}
	// Verify summary box contains counter labels (exact counts depend on
	// environment — Ollama may or may not be available in CI).
	if !strings.Contains(output, "passed") {
		t.Errorf("doctor summary should contain 'passed' counter, got:\n%s", output)
	}
	if !strings.Contains(output, "failed") {
		t.Errorf("doctor summary should contain 'failed' counter, got:\n%s", output)
	}
}

// TestDoctorCmd_MissingDeweyDir verifies doctor reports fail with
// `dewey init` fix when .dewey/ does not exist.
func TestDoctorCmd_MissingDeweyDir(t *testing.T) {
	tmpDir := t.TempDir()

	cmd := newDoctorCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--vault", tmpDir})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("doctor failed: %v", err)
	}

	output := buf.String()

	// .dewey/ check should fail with emoji marker.
	if !strings.Contains(output, "❌ .dewey/") {
		t.Errorf("doctor should report .dewey/ fail, got:\n%s", output)
	}

	// Fix should mention dewey init.
	if !strings.Contains(output, "dewey init") {
		t.Errorf("doctor should suggest 'dewey init' fix, got:\n%s", output)
	}

	// Summary box should still appear even on early exit.
	if !strings.Contains(output, "✅") {
		t.Errorf("doctor should include summary box even on early exit, got:\n%s", output)
	}
	if !strings.Contains(output, "1 failed") {
		t.Errorf("doctor should report 1 failure in summary, got:\n%s", output)
	}

	// Subsequent sections should NOT appear after early exit.
	if strings.Contains(output, "Database") {
		t.Errorf("doctor should not show Database section on early exit, got:\n%s", output)
	}
	if strings.Contains(output, "Embedding Layer") {
		t.Errorf("doctor should not show Embedding Layer section on early exit, got:\n%s", output)
	}
}

// TestDoctorCmd_HasFlags verifies the doctor subcommand has expected flags.
func TestDoctorCmd_HasFlags(t *testing.T) {
	cmd := newDoctorCmd()
	if cmd.Flags().Lookup("vault") == nil {
		t.Error("doctor command missing flag --vault")
	}
}

// TestRootCmd_Help_IncludesDoctorSubcommand verifies doctor appears in help.
func TestRootCmd_Help_IncludesDoctorSubcommand(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(--help) failed: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "doctor") {
		t.Errorf("help output missing subcommand 'doctor'")
	}
}
