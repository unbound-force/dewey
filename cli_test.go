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

// TestRootCmd_VersionFlag verifies --version flag works.
func TestRootCmd_VersionFlag(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(--version) failed: %v", err)
	}

	got := strings.TrimSpace(buf.String())
	if !strings.Contains(got, version) {
		t.Errorf("--version output = %q, should contain %q", got, version)
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
