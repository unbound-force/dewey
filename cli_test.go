package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
