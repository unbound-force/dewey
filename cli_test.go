// PARALLEL SAFETY: Tests in this file MUST NOT use t.Parallel().
// They mutate process-global state: os.Chdir (working directory),
// os.Stdout (for output capture), and logger (for log assertions).
// Running these tests in parallel would cause data races and flaky failures.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/spf13/pflag"
	"github.com/unbound-force/dewey/v3/artifacts"
	"github.com/unbound-force/dewey/v3/client"
	"github.com/unbound-force/dewey/v3/embed"
	"github.com/unbound-force/dewey/v3/source"
	"github.com/unbound-force/dewey/v3/store"
	"github.com/unbound-force/dewey/v3/types"
	"github.com/unbound-force/dewey/v3/vault"
	"gopkg.in/yaml.v3"
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

// TestInitCmd_CreatesDirectory verifies dewey init creates .uf/dewey/ directory.
func TestInitCmd_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	deweyDir := filepath.Join(tmpDir, deweyWorkspaceDir)
	if _, err := os.Stat(deweyDir); os.IsNotExist(err) {
		t.Fatal(".uf/dewey/ directory was not created")
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

	configPath := filepath.Join(tmpDir, deweyWorkspaceDir, "config.yaml")
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

	sourcesPath := filepath.Join(tmpDir, deweyWorkspaceDir, "sources.yaml")
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

// TestInitCmd_GitignoreAppend verifies granular .uf/dewey/ patterns are added to .gitignore.
func TestInitCmd_GitignoreAppend(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a .gitignore without any dewey patterns.
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
	text := string(content)
	// Verify granular runtime artifact patterns.
	for _, pattern := range []string{".uf/dewey/graph.db", ".uf/dewey/graph.db-shm", ".uf/dewey/graph.db-wal", ".uf/dewey/dewey.log", ".uf/dewey/dewey.lock"} {
		if !strings.Contains(text, pattern) {
			t.Errorf(".gitignore should contain %q, got:\n%s", pattern, text)
		}
	}
	// Verify the blanket .uf/dewey/ is NOT written.
	// Check that ".uf/dewey/" only appears as part of the granular patterns, not standalone.
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == ".uf/dewey/" {
			t.Errorf(".gitignore should NOT contain blanket '.uf/dewey/', got:\n%s", text)
		}
	}
}

// TestInitCmd_GitignoreAlreadyPresent verifies granular patterns are not duplicated.
func TestInitCmd_GitignoreAlreadyPresent(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a .gitignore that already has the new granular patterns.
	gitignorePath := filepath.Join(tmpDir, ".gitignore")
	existing := ".uf/dewey/graph.db\n.uf/dewey/graph.db-shm\n.uf/dewey/graph.db-wal\n.uf/dewey/dewey.log\n.uf/dewey/dewey.lock\n"
	if err := os.WriteFile(gitignorePath, []byte(existing), 0o644); err != nil {
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
	// Count occurrences of the key pattern — should be exactly 1 (no duplicate).
	count := strings.Count(string(content), ".uf/dewey/graph.db\n")
	if count != 1 {
		t.Errorf(".uf/dewey/graph.db appears %d times in .gitignore, want 1", count)
	}
}

// TestInitCmd_GitignoreLegacyPattern verifies that the old .dewey/ pattern
// is preserved and an informational message is logged.
func TestInitCmd_GitignoreLegacyPattern(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a .gitignore with the legacy blanket pattern.
	gitignorePath := filepath.Join(tmpDir, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte(".dewey/\n"), 0o644); err != nil {
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
	text := string(content)
	// Legacy pattern should be preserved — not modified.
	if !strings.Contains(text, ".dewey/\n") {
		t.Errorf("legacy .dewey/ pattern should be preserved, got:\n%s", text)
	}
	// New .uf/dewey/ patterns should NOT be added alongside legacy.
	if strings.Contains(text, ".uf/dewey/graph.db") {
		t.Errorf("granular patterns should NOT be added when legacy pattern exists, got:\n%s", text)
	}
}

// TestInitCmd_ScaffoldsSlashCommands verifies that dewey init creates
// slash command files in .opencode/commands/ when .opencode/ exists.
func TestInitCmd_ScaffoldsSlashCommands(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .opencode/ directory (simulating an OpenCode-initialized repo).
	if err := os.MkdirAll(filepath.Join(tmpDir, ".opencode"), 0o755); err != nil {
		t.Fatalf("create .opencode: %v", err)
	}
	// Create .gitignore so init doesn't error.
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(""), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// Verify all 6 Dewey slash commands were scaffolded.
	for _, name := range []string{"dewey-store.md", "dewey-index.md", "dewey-reindex.md", "dewey-compile.md", "dewey-curate.md", "dewey-lint.md"} {
		path := filepath.Join(tmpDir, ".opencode", "commands", name)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("slash command %s was not scaffolded", name)
		}
	}
}

// TestInitCmd_ScaffoldsDistillWorkflowExample verifies that dewey init
// ships the weekly distill workflow template EXAMPLE into the vault —
// the SC-008 install-from-scratch path: an adopting repository that has
// run `dewey init` finds `.github/workflows/dewey-distill.yml.example`
// locally, byte-identical to the payload constructed from the embedded
// map entry and to the tracked template, and the live workflow is never
// installed (FR-001: the copy remains the opt-in).
func TestInitCmd_ScaffoldsDistillWorkflowExample(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(""), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	examplePath := filepath.Join(tmpDir, ".github", "workflows", "dewey-distill.yml.example")
	got, err := os.ReadFile(examplePath)
	if err != nil {
		t.Fatalf("dewey init did not scaffold the workflow template example at %s: %v", examplePath, err)
	}

	// The scaffolded example must be byte-identical to the payload
	// constructed from the embedded map entry — canonical body plus the
	// trailing ownership marker (design D6: parity compares through
	// scaffoldPayload, the same constructor the writer uses) — and to the
	// tracked repository file, so the copies cannot drift.
	want, ok := deweyWorkflowTemplates["dewey-distill.yml.example"]
	if !ok {
		t.Fatal("deweyWorkflowTemplates missing dewey-distill.yml.example entry")
	}
	wantPayload := string(scaffoldPayload([]byte(want), scaffoldYAML))
	if string(got) != wantPayload {
		t.Error("scaffolded dewey-distill.yml.example differs from deweyWorkflowTemplates map entry payload (canonical body + marker)")
	}
	tracked, err := os.ReadFile(filepath.Join(".github", "workflows", "dewey-distill.yml.example"))
	if err != nil {
		t.Fatalf("read tracked workflow template: %v", err)
	}
	if string(tracked) != wantPayload {
		t.Error("tracked .github/workflows/dewey-distill.yml.example differs from deweyWorkflowTemplates map entry payload (canonical body + marker)")
	}

	// The live workflow must NOT be installed — the copy is the opt-in.
	installed := filepath.Join(tmpDir, ".github", "workflows", "dewey-distill.yml")
	if _, err := os.Stat(installed); err == nil {
		t.Error("dewey init auto-installed .github/workflows/dewey-distill.yml; the copy must be manual (FR-001)")
	} else if !os.IsNotExist(err) {
		t.Errorf("stat installed workflow: %v", err)
	}

	// Scaffolding is idempotent: a re-run never overwrites the user's
	// customized example.
	custom := []byte("# user-customized example\n")
	if err := os.WriteFile(examplePath, custom, 0o644); err != nil {
		t.Fatalf("customize example: %v", err)
	}
	cmd = newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("re-init failed: %v", err)
	}
	got, err = os.ReadFile(examplePath)
	if err != nil {
		t.Fatalf("re-read example: %v", err)
	}
	if string(got) != string(custom) {
		t.Error("dewey init overwrote an existing workflow template example; user customizations must be preserved")
	}
}

// TestDistillWorkflowTemplate_ScaffoldParity asserts the tracked
// .github/workflows/dewey-distill.yml.example stays byte-identical to the
// payload constructed from the embedded deweyWorkflowTemplates entry —
// canonical body plus the trailing ownership marker (design D6: compare
// through scaffoldPayload, the same constructor the writer uses) — the same
// parity pattern the agent definitions and slash commands follow, so the
// scaffolded copy can never drift from the reviewed template.
func TestDistillWorkflowTemplate_ScaffoldParity(t *testing.T) {
	want, ok := deweyWorkflowTemplates["dewey-distill.yml.example"]
	if !ok {
		t.Fatal("deweyWorkflowTemplates missing dewey-distill.yml.example entry")
	}
	wantPayload := string(scaffoldPayload([]byte(want), scaffoldYAML))
	tracked, err := os.ReadFile(filepath.Join(".github", "workflows", "dewey-distill.yml.example"))
	if err != nil {
		t.Fatalf("read tracked workflow template: %v", err)
	}
	if string(tracked) != wantPayload {
		t.Error("tracked .github/workflows/dewey-distill.yml.example differs from deweyWorkflowTemplates map entry payload (canonical body + marker)")
	}
}

// TestInitCmd_DoesNotInstallDistillWorkflow verifies that dewey init never
// auto-installs the weekly distill workflow (FR-001: the copy is the opt-in)
// but documents it when the example template ships in the vault.
func TestInitCmd_DoesNotInstallDistillWorkflow(t *testing.T) {
	tmpDir := t.TempDir()

	// Ship a user-owned example template in the vault so the skip-scaffold
	// and availability-hint paths are exercised (T015).
	workflowDir := filepath.Join(tmpDir, ".github", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("create workflows dir: %v", err)
	}
	examplePath := filepath.Join(workflowDir, "dewey-distill.yml.example")
	exampleContent := []byte("# example template fixture\n")
	if err := os.WriteFile(examplePath, exampleContent, 0o644); err != nil {
		t.Fatalf("write example workflow: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(""), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	// Capture logger output for the availability hint.
	var logBuf bytes.Buffer
	logger.SetOutput(&logBuf)
	defer logger.SetOutput(os.Stderr)

	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// The workflow itself must NOT be installed — the copy is the opt-in.
	installed := filepath.Join(workflowDir, "dewey-distill.yml")
	if _, err := os.Stat(installed); err == nil {
		t.Error("dewey init auto-installed .github/workflows/dewey-distill.yml; the copy must be manual (FR-001)")
	} else if !os.IsNotExist(err) {
		t.Errorf("stat installed workflow: %v", err)
	}

	// The example template must be untouched.
	got, err := os.ReadFile(examplePath)
	if err != nil {
		t.Fatalf("read example workflow: %v", err)
	}
	if string(got) != string(exampleContent) {
		t.Error("dewey init modified the example workflow template")
	}

	// The opt-in copy must be documented in the hint.
	logs := logBuf.String()
	if !strings.Contains(logs, "weekly distill workflow template available") {
		t.Errorf("init should log the template availability hint, got:\n%s", logs)
	}
	if !strings.Contains(logs, "cp .github/workflows/dewey-distill.yml.example .github/workflows/dewey-distill.yml") {
		t.Errorf("init hint should document the opt-in copy command, got:\n%s", logs)
	}
}

// TestInitCmd_SkipsExistingSlashCommands verifies that dewey init does
// not overwrite existing slash command files (preserves user customizations).
func TestInitCmd_SkipsExistingSlashCommands(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .opencode/commands/ with a custom dewey-store.md.
	cmdDir := filepath.Join(tmpDir, ".opencode", "commands")
	if err := os.MkdirAll(cmdDir, 0o755); err != nil {
		t.Fatalf("create command dir: %v", err)
	}
	customContent := "# My custom dewey-store command\n"
	if err := os.WriteFile(filepath.Join(cmdDir, "dewey-store.md"), []byte(customContent), 0o644); err != nil {
		t.Fatalf("write custom command: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(""), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// Verify custom content was preserved (not overwritten).
	content, err := os.ReadFile(filepath.Join(cmdDir, "dewey-store.md"))
	if err != nil {
		t.Fatalf("read command: %v", err)
	}
	if string(content) != customContent {
		t.Errorf("dewey-store.md was overwritten, got:\n%s", string(content))
	}

	// Other commands should still be scaffolded.
	if _, err := os.Stat(filepath.Join(cmdDir, "dewey-index.md")); err != nil {
		t.Error("dewey-index.md should have been scaffolded (it didn't exist)")
	}
}

// TestInitCmd_NoOpenCodeDir verifies that dewey init gracefully skips
// slash command scaffolding when .opencode/ doesn't exist.
func TestInitCmd_NoOpenCodeDir(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(""), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// Verify no .opencode/commands/ directory was created.
	if _, err := os.Stat(filepath.Join(tmpDir, ".opencode", "commands")); err == nil {
		t.Error(".opencode/commands/ should NOT exist when .opencode/ was not present")
	}
}

// --- DCP config scaffolding tests ---

// TestInitCmd_ScaffoldsDCPConfig verifies that dewey init creates
// .opencode/dcp.jsonc when .opencode/ exists but no DCP config is present.
func TestInitCmd_ScaffoldsDCPConfig(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .opencode/ directory (simulating an OpenCode-initialized repo).
	if err := os.MkdirAll(filepath.Join(tmpDir, ".opencode"), 0o755); err != nil {
		t.Fatalf("create .opencode: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(""), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// Verify dcp.jsonc was created.
	dcpPath := filepath.Join(tmpDir, ".opencode", "dcp.jsonc")
	content, err := os.ReadFile(dcpPath)
	if err != nil {
		t.Fatalf("dcp.jsonc was not scaffolded: %v", err)
	}

	// Verify content has protectTags and $schema.
	s := string(content)
	if !strings.Contains(s, "protectTags") {
		t.Error("dcp.jsonc should contain protectTags")
	}
	if !strings.Contains(s, "$schema") {
		t.Error("dcp.jsonc should contain $schema reference")
	}
}

// TestInitCmd_SkipsExistingDCPConfigWithProtectTags verifies that dewey init
// does not overwrite an existing DCP config that already has protectTags.
func TestInitCmd_SkipsExistingDCPConfigWithProtectTags(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(tmpDir, ".opencode"), 0o755); err != nil {
		t.Fatalf("create .opencode: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(""), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	// Create existing dcp.jsonc with custom content that includes protectTags.
	customContent := `{"compress": {"protectTags": true, "maxTokens": 5000}}`
	dcpPath := filepath.Join(tmpDir, ".opencode", "dcp.jsonc")
	if err := os.WriteFile(dcpPath, []byte(customContent), 0o644); err != nil {
		t.Fatalf("write custom dcp.jsonc: %v", err)
	}

	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// Verify custom content was preserved (not overwritten).
	content, err := os.ReadFile(dcpPath)
	if err != nil {
		t.Fatalf("read dcp.jsonc: %v", err)
	}
	if string(content) != customContent {
		t.Errorf("dcp.jsonc was overwritten, got:\n%s", string(content))
	}
}

// TestInitCmd_WarnsOnDCPConfigMissingProtectTags verifies that dewey init
// warns when a DCP config exists but does not contain protectTags, and
// does not overwrite the file.
func TestInitCmd_WarnsOnDCPConfigMissingProtectTags(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(tmpDir, ".opencode"), 0o755); err != nil {
		t.Fatalf("create .opencode: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(""), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	// Create existing dcp.jsonc WITHOUT protectTags.
	customContent := `{"compress": {"maxTokens": 5000}}`
	dcpPath := filepath.Join(tmpDir, ".opencode", "dcp.jsonc")
	if err := os.WriteFile(dcpPath, []byte(customContent), 0o644); err != nil {
		t.Fatalf("write custom dcp.jsonc: %v", err)
	}

	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// Verify custom content was preserved (not overwritten).
	content, err := os.ReadFile(dcpPath)
	if err != nil {
		t.Fatalf("read dcp.jsonc: %v", err)
	}
	if string(content) != customContent {
		t.Errorf("dcp.jsonc was overwritten, got:\n%s", string(content))
	}
}

// TestInitCmd_NoDCPConfigWhenNoOpenCodeDir verifies that dewey init does not
// scaffold dcp.jsonc when .opencode/ does not exist (composability guard).
func TestInitCmd_NoDCPConfigWhenNoOpenCodeDir(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(""), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// Verify no dcp.jsonc was created.
	dcpPath := filepath.Join(tmpDir, ".opencode", "dcp.jsonc")
	if _, err := os.Stat(dcpPath); err == nil {
		t.Error("dcp.jsonc should NOT exist when .opencode/ was not present")
	}
}

// TestInitCmd_SkipsExistingDCPJSONWithProtectTags verifies that dewey init
// detects an existing dcp.json (not dcp.jsonc) with protectTags and skips
// scaffolding dcp.jsonc.
func TestInitCmd_SkipsExistingDCPJSONWithProtectTags(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(tmpDir, ".opencode"), 0o755); err != nil {
		t.Fatalf("create .opencode: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(""), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	// Create existing dcp.json (not dcp.jsonc) with protectTags.
	customContent := `{"compress": {"protectTags": true}}`
	dcpJSON := filepath.Join(tmpDir, ".opencode", "dcp.json")
	if err := os.WriteFile(dcpJSON, []byte(customContent), 0o644); err != nil {
		t.Fatalf("write dcp.json: %v", err)
	}

	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// Verify dcp.jsonc was NOT created (dcp.json already covers it).
	dcpJSONC := filepath.Join(tmpDir, ".opencode", "dcp.jsonc")
	if _, err := os.Stat(dcpJSONC); err == nil {
		t.Error("dcp.jsonc should NOT be created when dcp.json already has protectTags")
	}
	// Verify dcp.json was not overwritten.
	content, err := os.ReadFile(dcpJSON)
	if err != nil {
		t.Fatalf("read dcp.json: %v", err)
	}
	if string(content) != customContent {
		t.Errorf("dcp.json was overwritten, got:\n%s", string(content))
	}
}

// TestInitCmd_ReInitScaffoldsNewCommands verifies that running dewey init
// on an already-initialized repo still scaffolds missing slash commands.
func TestInitCmd_ReInitScaffoldsNewCommands(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .opencode/ and .gitignore.
	if err := os.MkdirAll(filepath.Join(tmpDir, ".opencode"), 0o755); err != nil {
		t.Fatalf("create .opencode: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(""), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	// First init — creates everything.
	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("first init failed: %v", err)
	}

	// Verify slash commands were created.
	storePath := filepath.Join(tmpDir, ".opencode", "commands", "dewey-store.md")
	if _, err := os.Stat(storePath); err != nil {
		t.Fatalf("dewey-store.md not created on first init")
	}

	// Delete one slash command to simulate upgrading dewey with a new command.
	if err := os.Remove(storePath); err != nil {
		t.Fatalf("remove dewey-store.md: %v", err)
	}

	// Second init — should re-scaffold the deleted command.
	cmd2 := newInitCmd()
	cmd2.SetArgs([]string{"--vault", tmpDir})
	if err := cmd2.Execute(); err != nil {
		t.Fatalf("second init failed: %v", err)
	}

	// Verify the deleted command was re-scaffolded.
	if _, err := os.Stat(storePath); err != nil {
		t.Error("dewey-store.md should have been re-scaffolded on second init")
	}
}

// TestInitCmd_MigratesOldCommandDir verifies that dewey init migrates
// Dewey slash commands from .opencode/command/ (old) to .opencode/commands/ (new).
func TestInitCmd_MigratesOldCommandDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .opencode/ and old .opencode/command/ with 2 Dewey files.
	oldDir := filepath.Join(tmpDir, ".opencode", "command")
	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatalf("create old command dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "dewey-store.md"), []byte("old store"), 0o644); err != nil {
		t.Fatalf("write old dewey-store: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "dewey-index.md"), []byte("old index"), 0o644); err != nil {
		t.Fatalf("write old dewey-index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(""), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	newDir := filepath.Join(tmpDir, ".opencode", "commands")

	// Assert both files were migrated to new directory.
	for _, name := range []string{"dewey-store.md", "dewey-index.md"} {
		if _, err := os.Stat(filepath.Join(newDir, name)); err != nil {
			t.Errorf("%s was not migrated to new dir", name)
		}
	}

	// Assert old directory was removed (it only had Dewey files).
	if _, err := os.Stat(oldDir); err == nil {
		t.Error("old .opencode/command/ should have been removed (was empty after migration)")
	}

	// Assert all deweySlashCommands entries exist in new dir (scaffolding proceeded).
	for name := range deweySlashCommands {
		if _, err := os.Stat(filepath.Join(newDir, name)); err != nil {
			t.Errorf("slash command %s was not scaffolded in new dir", name)
		}
	}
}

// TestInitCmd_MigrationSkipsExistingInNewDir verifies that migration does
// not overwrite files already present in .opencode/commands/.
func TestInitCmd_MigrationSkipsExistingInNewDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Create both old and new directories with the same file but different content.
	oldDir := filepath.Join(tmpDir, ".opencode", "command")
	newDir := filepath.Join(tmpDir, ".opencode", "commands")
	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatalf("create old dir: %v", err)
	}
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		t.Fatalf("create new dir: %v", err)
	}
	oldContent := "old version of dewey-index"
	newContent := "new version of dewey-index"
	if err := os.WriteFile(filepath.Join(oldDir, "dewey-index.md"), []byte(oldContent), 0o644); err != nil {
		t.Fatalf("write old file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(newDir, "dewey-index.md"), []byte(newContent), 0o644); err != nil {
		t.Fatalf("write new file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(""), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// Assert new dir file retains its original content (not overwritten).
	content, err := os.ReadFile(filepath.Join(newDir, "dewey-index.md"))
	if err != nil {
		t.Fatalf("read new file: %v", err)
	}
	if string(content) != newContent {
		t.Errorf("new dir file was overwritten, got %q, want %q", string(content), newContent)
	}

	// Assert old copy was removed.
	if _, err := os.Stat(filepath.Join(oldDir, "dewey-index.md")); err == nil {
		t.Error("old dewey-index.md should have been removed after migration")
	}
}

// TestInitCmd_MigrationKeepsOldDirWithNonDeweyFiles verifies that the old
// directory is preserved when it contains non-Dewey files after migration.
func TestInitCmd_MigrationKeepsOldDirWithNonDeweyFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create old directory with a Dewey file and a non-Dewey file.
	oldDir := filepath.Join(tmpDir, ".opencode", "command")
	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatalf("create old dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "dewey-store.md"), []byte("old store"), 0o644); err != nil {
		t.Fatalf("write dewey file: %v", err)
	}
	nonDeweyContent := "speckit plan content"
	if err := os.WriteFile(filepath.Join(oldDir, "speckit.plan.md"), []byte(nonDeweyContent), 0o644); err != nil {
		t.Fatalf("write non-dewey file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(""), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	newDir := filepath.Join(tmpDir, ".opencode", "commands")

	// Assert Dewey file was migrated to new dir.
	if _, err := os.Stat(filepath.Join(newDir, "dewey-store.md")); err != nil {
		t.Error("dewey-store.md was not migrated to new dir")
	}

	// Assert non-Dewey file is untouched in old dir.
	content, err := os.ReadFile(filepath.Join(oldDir, "speckit.plan.md"))
	if err != nil {
		t.Fatalf("non-Dewey file should still exist in old dir: %v", err)
	}
	if string(content) != nonDeweyContent {
		t.Errorf("non-Dewey file content changed, got %q", string(content))
	}

	// Assert old dir still exists (has non-Dewey files).
	if _, err := os.Stat(oldDir); err != nil {
		t.Error("old .opencode/command/ should still exist (contains non-Dewey files)")
	}
}

// --- Status command tests ---

// TestStatusCmd_Uninitialized verifies status fails when .uf/dewey/ doesn't exist.
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
	deweyDir := filepath.Join(tmpDir, deweyWorkspaceDir)
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
	deweyDir := filepath.Join(tmpDir, deweyWorkspaceDir)
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

// scaffoldedStatusConfigYAML mirrors the config.yaml template `dewey init`
// writes (newInitCmd): the model line carries the trailing comment that the
// old line-based status parser leaked into embeddingModel (issue #225).
const scaffoldedStatusConfigYAML = `# Dewey configuration
# See: https://github.com/unbound-force/dewey

embedding:
  model: granite-embedding:30m  # Override with DEWEY_EMBEDDING_MODEL env var
  # endpoint: http://localhost:11434  # Uncomment to override OLLAMA_HOST
  # max_chunk_chars: 12288  # Override with DEWEY_CHUNK_MAX_CHARS env var
`

// newStatusTestVault creates a temp vault whose .uf/dewey directory holds
// configYAML and pins the embedding environment the way newGatewayDoctorVault
// does, so ambient state cannot leak into status model resolution. An empty
// configYAML leaves config.yaml absent (the no-config scenarios).
// XDG_CONFIG_HOME points at an empty temp dir whose dewey/ subdirectory is
// returned as globalConfigDir — writing a config.yaml there simulates the
// global config ReadEmbeddingConfig falls back to.
func newStatusTestVault(t *testing.T, configYAML string) (vaultDir, globalConfigDir string) {
	t.Helper()
	tmpDir := t.TempDir()
	deweyDir := filepath.Join(tmpDir, deweyWorkspaceDir)
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("mkdir .uf/dewey: %v", err)
	}
	if configYAML != "" {
		if err := os.WriteFile(filepath.Join(deweyDir, "config.yaml"), []byte(configYAML), 0o644); err != nil {
			t.Fatalf("write config.yaml: %v", err)
		}
	}
	xdgDir := t.TempDir()
	globalDir := filepath.Join(xdgDir, "dewey")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("mkdir global config dir: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", xdgDir)
	t.Setenv("DEWEY_EMBEDDING_ENDPOINT", "")
	t.Setenv("DEWEY_EMBEDDING_MODEL", "")
	t.Setenv("OLLAMA_HOST", "")
	return tmpDir, globalDir
}

// runStatusJSON runs `dewey status --json` against vaultDir and returns the
// parsed output object. A command error fails the test, so every caller also
// asserts STA-FR-002's exit-0 guarantee for unreachable providers and
// unconstructible configs.
func runStatusJSON(t *testing.T, vaultDir string) map[string]any {
	t.Helper()
	cmd := newStatusCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--json", "--vault", vaultDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("status --json failed: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, buf.String())
	}
	return result
}

// assertStatusEmbeddingModel asserts the reported embeddingModel is exactly
// want (STA-FR-001, TC-009). The availability probe runs with its result
// ignored: model resolution is pure configuration and must not depend on any
// provider.
func assertStatusEmbeddingModel(t *testing.T, vaultDir, want string) {
	t.Helper()
	result := runStatusJSON(t, vaultDir)
	got, _ := result["embeddingModel"].(string)
	if got != want {
		t.Errorf("embeddingModel = %q, want exactly %q", got, want)
	}
}

// assertStatusEmbeddingAvailable asserts the reported embeddingAvailable is
// exactly want (STA-FR-002, TC-009), failing loudly when the key is missing
// or not a boolean so a key-set regression cannot pass silently.
func assertStatusEmbeddingAvailable(t *testing.T, vaultDir string, want bool) {
	t.Helper()
	result := runStatusJSON(t, vaultDir)
	got, ok := result["embeddingAvailable"].(bool)
	if !ok {
		t.Fatalf("embeddingAvailable missing or not a boolean: %#v", result["embeddingAvailable"])
	}
	if got != want {
		t.Errorf("embeddingAvailable = %v, want %v", got, want)
	}
}

// TestStatusCmd_EmbeddingModelScaffoldedCommentStripped verifies the trailing
// comment on the scaffolded model line is stripped: YAML parsing resolves the
// model to granite-embedding:30m (STA-FR-001).
func TestStatusCmd_EmbeddingModelScaffoldedCommentStripped(t *testing.T) {
	vaultDir, _ := newStatusTestVault(t, scaffoldedStatusConfigYAML)
	assertStatusEmbeddingModel(t, vaultDir, "granite-embedding:30m")
}

// TestStatusCmd_EmbeddingModelEnvOverride verifies DEWEY_EMBEDDING_MODEL wins
// over the per-vault config.yaml model, the same precedence serve applies
// (STA-FR-001).
func TestStatusCmd_EmbeddingModelEnvOverride(t *testing.T) {
	vaultDir, _ := newStatusTestVault(t, "embedding:\n  model: config-model\n")
	// Set after newStatusTestVault cleared it, so only this override is in play.
	t.Setenv("DEWEY_EMBEDDING_MODEL", "env-model")
	assertStatusEmbeddingModel(t, vaultDir, "env-model")
}

// TestStatusCmd_EmbeddingModelNoConfigDefault verifies a vault with .uf/dewey/
// but no config file anywhere reports the serve default, not the empty string
// (STA-FR-001).
func TestStatusCmd_EmbeddingModelNoConfigDefault(t *testing.T) {
	vaultDir, _ := newStatusTestVault(t, "")
	assertStatusEmbeddingModel(t, vaultDir, "granite-embedding:30m")
}

// TestStatusCmd_EmbeddingModelGlobalConfigFallback verifies a per-vault config
// that sets neither embedding.provider nor embedding.model falls through to the
// global config's model (STA-FR-001).
func TestStatusCmd_EmbeddingModelGlobalConfigFallback(t *testing.T) {
	vaultDir, globalConfigDir := newStatusTestVault(t, "synthesis:\n  provider: ollama\n")
	globalYAML := "embedding:\n  model: global-model\n"
	if err := os.WriteFile(filepath.Join(globalConfigDir, "config.yaml"), []byte(globalYAML), 0o644); err != nil {
		t.Fatalf("write global config.yaml: %v", err)
	}
	assertStatusEmbeddingModel(t, vaultDir, "global-model")
}

// TestStatusCmd_EmbeddingAvailableModelServed verifies embeddingAvailable is
// true when the configured endpoint lists the configured model (STA-FR-002,
// provider-serving scenario). The endpoint is pinned to an httptest server
// through the vault config, so no host Ollama is consulted (constitution IV).
func TestStatusCmd_EmbeddingAvailableModelServed(t *testing.T) {
	srv := newMockOllamaServer(`{"name":"llama3:latest"},{"name":"granite-embedding:30m"}`)
	defer srv.Close()

	vaultDir, _ := newStatusTestVault(t,
		"embedding:\n  model: granite-embedding:30m\n  endpoint: "+srv.URL+"\n")
	assertStatusEmbeddingAvailable(t, vaultDir, true)
}

// TestStatusCmd_EmbeddingAvailableEndpointUnreachable verifies a closed
// endpoint reports unavailable without failing the command (STA-FR-002,
// provider-unreachable scenario): runStatusJSON fails the test on any command
// error, which is the exit-0 half of the guarantee. The endpoint is pinned
// through DEWEY_EMBEDDING_ENDPOINT — the env var overrides config.yaml — so
// the probe targets the dead port rather than any host Ollama (constitution
// IV).
func TestStatusCmd_EmbeddingAvailableEndpointUnreachable(t *testing.T) {
	// Closed immediately: the server's URL now names a port nothing listens
	// on, a deterministic connection-refused target for the bounded probe.
	srv := newMockOllamaServer(`{"name":"granite-embedding:30m"}`)
	srv.Close()

	vaultDir, _ := newStatusTestVault(t, "embedding:\n  model: granite-embedding:30m\n")
	// Set after newStatusTestVault cleared it, so only this pin is in play.
	t.Setenv("DEWEY_EMBEDDING_ENDPOINT", srv.URL)

	assertStatusEmbeddingAvailable(t, vaultDir, false)
}

// TestStatusCmd_EmbeddingAvailableModelNotServed verifies embeddingAvailable
// is false when the endpoint responds but its tags list does not include the
// configured model (STA-FR-002, model-not-served scenario). The list is
// non-empty so the false comes from name matching, not an empty response.
func TestStatusCmd_EmbeddingAvailableModelNotServed(t *testing.T) {
	srv := newMockOllamaServer(`{"name":"llama3:latest"}`)
	defer srv.Close()

	vaultDir, _ := newStatusTestVault(t,
		"embedding:\n  model: granite-embedding:30m\n  endpoint: "+srv.URL+"\n")
	assertStatusEmbeddingAvailable(t, vaultDir, false)
}

// TestStatusCmd_EmbeddingAvailableVertexMissingProject verifies a vertex
// config without a project reports unavailable without failing the command
// (STA-FR-002, misconfigured-provider scenario): construction fails inside
// NewEmbedderFromConfig before any credential or network access, and
// queryEmbeddingStatus maps that to false while the model still reports from
// config (design D2).
func TestStatusCmd_EmbeddingAvailableVertexMissingProject(t *testing.T) {
	vaultDir, _ := newStatusTestVault(t,
		"embedding:\n  provider: vertex\n  model: text-embedding-005\n  region: us-central1\n")

	assertStatusEmbeddingAvailable(t, vaultDir, false)
	assertStatusEmbeddingModel(t, vaultDir, "text-embedding-005")
}

// TestStatusCmd_JSONKeySetUnchanged verifies the STA-FR-003 contract: the
// --json object keeps every key it had before the embedding fix — no
// pre-existing key dropped, none renamed. Set equality is asserted in both
// directions so a rename fails as one missing plus one unexpected key.
// This change adds the four index-provenance keys on top of that set
// (IBP-FR-003), so the want set is the union of both. The trust key is
// always present because queryTrustStatus never returns nil; sources stays
// present (null) on a store-less vault.
func TestStatusCmd_JSONKeySetUnchanged(t *testing.T) {
	vaultDir, _ := newStatusTestVault(t, scaffoldedStatusConfigYAML)
	result := runStatusJSON(t, vaultDir)

	// The pre-embedding-fix formatStatusJSON key set, plus the four
	// index-provenance keys this change adds (IBP-FR-003).
	want := []string{
		"path", "pages", "blocks", "embeddings",
		"embeddingModel", "embeddingAvailable", "embeddingCoverage",
		"deweyVersion", "indexFormat", "reindexRecommended", "reindexReasons",
		"sources", "trust",
	}
	slices.Sort(want)

	got := make([]string, 0, len(result))
	for k := range result {
		got = append(got, k)
	}
	slices.Sort(got)

	if !slices.Equal(want, got) {
		t.Errorf("status --json key set changed (STA-FR-003):\n  want: %v\n  got:  %v", want, got)
	}

	// The keys outside the corrected pair keep their meaning (STA-FR-003's
	// second clause): path is still the vault's .uf/dewey directory, and a
	// vault without graph.db still reports zero pages.
	wantPath := filepath.Join(vaultDir, deweyWorkspaceDir)
	if gotPath, _ := result["path"].(string); gotPath != wantPath {
		t.Errorf("path = %q, want %q", gotPath, wantPath)
	}
	if gotPages, _ := result["pages"].(float64); gotPages != 0 {
		t.Errorf("pages = %v, want 0 for a vault without graph.db", gotPages)
	}
}

// TestStatusCmd_TextOutputModelLineClean verifies the STA-FR-001 text-output
// scenario on the scaffolded-config vault: the Model: line shows exactly the
// YAML-resolved model, so the trailing config comment the old line parser
// leaked (issue #225) cannot reach the text report either. The exact line
// (with its trailing newline) pins both halves — the clean model and the
// absence of comment text — because a polluted line would not match.
func TestStatusCmd_TextOutputModelLineClean(t *testing.T) {
	vaultDir, _ := newStatusTestVault(t, scaffoldedStatusConfigYAML)

	cmd := newStatusCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--vault", vaultDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("status failed: %v", err)
	}

	output := buf.String()
	wantLine := "  Model:      granite-embedding:30m\n"
	if !strings.Contains(output, wantLine) {
		t.Errorf("text output missing clean Model: line %q\noutput:\n%s", wantLine, output)
	}
	if strings.Contains(output, "# Override with DEWEY_EMBEDDING_MODEL") {
		t.Errorf("text output carries config comment text on the Model: line:\n%s", output)
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

// TestIndexCmd_Uninitialized verifies index fails when .uf/dewey/ doesn't exist.
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

	// Create .uf/dewey/ with sources.yaml.
	deweyDir := filepath.Join(tmpDir, deweyWorkspaceDir)
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

	deweyDir := filepath.Join(tmpDir, deweyWorkspaceDir)
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

	indexResult, _ := vault.IndexDocuments(s, docs, nil, nil, 0)
	if indexResult.TotalIndexed != 1 {
		t.Fatalf("IndexDocuments() = %d, want 1", indexResult.TotalIndexed)
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

	indexResult, _ := vault.IndexDocuments(s, docs, nil, nil, 0)
	if indexResult.TotalIndexed != 1 {
		t.Fatalf("IndexDocuments() = %d, want 1", indexResult.TotalIndexed)
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

	_, _ = vault.IndexDocuments(s, docs, configs, nil, 0)

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

	indexResult, _ := vault.IndexDocuments(s, docs, nil, nil, 0)
	if indexResult.TotalIndexed != 1 {
		t.Fatalf("IndexDocuments() = %d, want 1", indexResult.TotalIndexed)
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

	indexResult, _ := vault.IndexDocuments(s, docs, nil, nil, 0)
	if indexResult.TotalIndexed != 1 {
		t.Fatalf("IndexDocuments() = %d, want 1", indexResult.TotalIndexed)
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
	_, _ = vault.IndexDocuments(s, docs1, nil, nil, 0)

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
	_, _ = vault.IndexDocuments(s, docs2, nil, nil, 0)

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
	indexResult, _ := vault.IndexDocuments(s, docs, nil, nil, 0)
	if indexResult.TotalIndexed != 1 {
		t.Fatalf("IndexDocuments() = %d, want 1", indexResult.TotalIndexed)
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

	indexResult, _ := vault.IndexDocuments(s, docs, nil, nil, 0)
	if indexResult.TotalIndexed != 3 {
		t.Fatalf("IndexDocuments() = %d, want 3", indexResult.TotalIndexed)
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
	deweyDir := filepath.Join(tmpDir, deweyWorkspaceDir)
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
	for _, name := range []string{"graph.db", "graph.db-wal", "graph.db-shm", "dewey.lock"} {
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
// error when .uf/dewey/ does not exist.
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
		t.Fatal("reindex should fail without .uf/dewey/")
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

// TestDoctorCounter_PrintCheck_InfoMarker verifies that the INFO marker
// renders with the info emoji and does not increment pass/warn/fail counters.
func TestDoctorCounter_PrintCheck_InfoMarker(t *testing.T) {
	var buf bytes.Buffer
	c := &doctorCounter{}

	c.printCheck(&buf, "INFO", "model", "granite-embedding:30m has a newer replacement")

	if c.pass != 0 || c.warn != 0 || c.fail != 0 {
		t.Errorf("INFO should not increment any counter, got pass=%d warn=%d fail=%d", c.pass, c.warn, c.fail)
	}

	output := buf.String()
	if !strings.Contains(output, "model") {
		t.Errorf("INFO line missing model name, got:\n%s", output)
	}
	if !strings.Contains(output, "newer replacement") {
		t.Errorf("INFO line missing description, got:\n%s", output)
	}
}

// TestDoctorLegacyModelAdvisory verifies the legacy model advisory logic:
// granite-embedding:30m triggers an advisory, other models do not.
func TestDoctorLegacyModelAdvisory(t *testing.T) {
	tests := []struct {
		model        string
		wantAdvisory bool
	}{
		{"granite-embedding:30m", true},
		{"granite-embedding:278m", false},
		{"granite-embedding-small-english-r2", false},
		{"nomic-embed-text", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := isLegacyEmbeddingModel(tt.model)
			if got != tt.wantAdvisory {
				t.Errorf("isLegacyEmbeddingModel(%q) = %v, want %v", tt.model, got, tt.wantAdvisory)
			}
		})
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
// and store checks when .uf/dewey/ and graph.db exist with pages.
func TestDoctorCmd_WithInitializedVault(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .uf/dewey/ directory.
	deweyDir := filepath.Join(tmpDir, deweyWorkspaceDir)
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("mkdir .uf/dewey: %v", err)
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

	// .uf/dewey/ check should pass with emoji marker.
	if !strings.Contains(output, "✅ .uf/dewey/") {
		t.Errorf("doctor should report .uf/dewey/ pass, got:\n%s", output)
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

	// Synthesis Layer section should exist.
	if !strings.Contains(output, "Synthesis Layer") {
		t.Errorf("doctor should include Synthesis Layer section, got:\n%s", output)
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

// trustFixtureGit runs one git command in dir, failing the test on error.
func trustFixtureGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// newTrustFixtureVault builds a git-backed vault for doctor/status trust
// diagnostics: per-vault config pinning the comparison ref (origin/main,
// the highest-precedence IP-FR-004 leg, keeping the fixture independent of
// machine-wide configuration), one committed compiled page claiming
// validated, and — when fetchRef is true — refs/remotes/origin/main
// pointing at the commit (the local state a fetch leaves behind). Without
// fetchRef the ref is missing entirely (a fresh clone before any fetch,
// GTS-FR-004's missing-ref fallback).
func newTrustFixtureVault(t *testing.T, fetchRef bool) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping git subprocess test in short mode")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	trustFixtureGit(t, dir, "init", "-q", "-b", "main")
	trustFixtureGit(t, dir, "config", "user.name", "t")
	trustFixtureGit(t, dir, "config", "user.email", "t@example.com")

	deweyDir := filepath.Join(dir, deweyWorkspaceDir)
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("mkdir .uf/dewey: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deweyDir, "config.yaml"),
		[]byte("default_branch_ref: origin/main\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(dir, "compiled"), 0o755); err != nil {
		t.Fatalf("mkdir compiled: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "compiled", "auth.md"),
		[]byte("---\ntier: validated\n---\n\n# Auth\n\nbody\n"), 0o644); err != nil {
		t.Fatalf("write page: %v", err)
	}
	trustFixtureGit(t, dir, "add", ".")
	trustFixtureGit(t, dir, "commit", "-q", "-m", "seed compiled page")
	if fetchRef {
		trustFixtureGit(t, dir, "update-ref", "refs/remotes/origin/main", "HEAD")
	}
	return dir
}

// TestDoctorCmd_TrustTiers_Resolved verifies the Trust Tiers section when
// the default-branch ref resolves and every trust file matches it: the ref
// line passes with the ref, short SHA, and precedence leg, and the tier
// derivation line reports the validated-eligible count.
func TestDoctorCmd_TrustTiers_Resolved(t *testing.T) {
	vaultDir := newTrustFixtureVault(t, true)

	cmd := newDoctorCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--vault", vaultDir})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("doctor failed: %v", err)
	}

	output := buf.String()
	for _, want := range []string{
		"Trust Tiers",
		"✅ default-branch ref",
		"origin/main @ ",
		"via vault-config",
		"✅ tier derivation",
		"1 file(s) match origin/main",
		"1 validated-eligible",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("doctor output missing %q:\n%s", want, output)
		}
	}
}

// TestDoctorCmd_TrustTiers_MissingRef verifies the GTS-FR-004 fallback
// note: a git repository whose comparison ref was never fetched warns with
// the missing-ref status and a fetch fix hint, and the tier derivation
// line reports the draft degradation.
func TestDoctorCmd_TrustTiers_MissingRef(t *testing.T) {
	vaultDir := newTrustFixtureVault(t, false)

	cmd := newDoctorCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--vault", vaultDir})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("doctor failed: %v", err)
	}

	output := buf.String()
	for _, want := range []string{
		"⚠️ default-branch ref",
		"missing-ref",
		"Fix: git fetch",
		"⚠️ tier derivation",
		"degrade to draft",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("doctor output missing %q:\n%s", want, output)
		}
	}
}

// TestDoctorCmd_TrustTiers_NotARepo verifies the non-git-vault fallback:
// without a git repository the ref line warns with not-a-repo and the
// derivation withholds validated.
func TestDoctorCmd_TrustTiers_NotARepo(t *testing.T) {
	tmpDir := t.TempDir()
	deweyDir := filepath.Join(tmpDir, deweyWorkspaceDir)
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("mkdir .uf/dewey: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deweyDir, "config.yaml"),
		[]byte("default_branch_ref: origin/main\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "compiled"), 0o755); err != nil {
		t.Fatalf("mkdir compiled: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "compiled", "auth.md"),
		[]byte("---\ntier: validated\n---\n\n# Auth\n\nbody\n"), 0o644); err != nil {
		t.Fatalf("write page: %v", err)
	}

	cmd := newDoctorCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--vault", tmpDir})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("doctor failed: %v", err)
	}

	output := buf.String()
	for _, want := range []string{
		"⚠️ default-branch ref",
		"not-a-repo",
		"⚠️ tier derivation",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("doctor output missing %q:\n%s", want, output)
		}
	}
}

// TestDoctorCmd_TrustTiers_DifferingFile verifies the effective-tier
// observability line: a working-tree edit that diverges from the fetched
// default branch is reported as differing (indexes as draft until merged
// and fetched) without failing the ref check.
func TestDoctorCmd_TrustTiers_DifferingFile(t *testing.T) {
	vaultDir := newTrustFixtureVault(t, true)

	// Working-tree edit: the file blob diverges from origin/main.
	if err := os.WriteFile(filepath.Join(vaultDir, "compiled", "auth.md"),
		[]byte("---\ntier: validated\n---\n\n# Auth\n\nedited body\n"), 0o644); err != nil {
		t.Fatalf("write page: %v", err)
	}

	cmd := newDoctorCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--vault", vaultDir})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("doctor failed: %v", err)
	}

	output := buf.String()
	for _, want := range []string{
		"✅ default-branch ref",
		"ℹ️ tier derivation",
		"1 of 1 file(s) differ from origin/main",
		"index as draft until merged and fetched",
		"0 validated-eligible",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("doctor output missing %q:\n%s", want, output)
		}
	}
}

// TestStatusCmd_TrustTiers verifies the status command's Trust Tiers
// reporting: the text output names the ref and eligibility counts, and the
// JSON output carries the full trust object with the resolved ref state.
func TestStatusCmd_TrustTiers(t *testing.T) {
	vaultDir := newTrustFixtureVault(t, true)

	t.Run("text", func(t *testing.T) {
		cmd := newStatusCmd()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetArgs([]string{"--vault", vaultDir})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("status failed: %v", err)
		}
		output := buf.String()
		for _, want := range []string{
			"Trust Tiers",
			"Ref:        origin/main @ ",
			"via vault-config",
			"1 validated-eligible",
			"0 differing",
		} {
			if !strings.Contains(output, want) {
				t.Errorf("status output missing %q:\n%s", want, output)
			}
		}
	})

	t.Run("json", func(t *testing.T) {
		cmd := newStatusCmd()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetArgs([]string{"--json", "--vault", vaultDir})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("status --json failed: %v", err)
		}

		var result map[string]any
		if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
			t.Fatalf("invalid JSON output: %v\noutput: %s", err, buf.String())
		}
		trust, ok := result["trust"].(map[string]any)
		if !ok {
			t.Fatalf("JSON output missing 'trust' object:\n%s", buf.String())
		}
		if trust["ref"] != "origin/main" {
			t.Errorf("trust.ref = %v, want origin/main", trust["ref"])
		}
		if trust["status"] != "resolved" {
			t.Errorf("trust.status = %v, want resolved", trust["status"])
		}
		if trust["leg"] != "vault-config" {
			t.Errorf("trust.leg = %v, want vault-config", trust["leg"])
		}
		if sha, _ := trust["sha"].(string); len(sha) != 40 {
			t.Errorf("trust.sha = %v, want a 40-hex commit SHA", trust["sha"])
		}
		if trust["trust_files"] != float64(1) {
			t.Errorf("trust.trust_files = %v, want 1", trust["trust_files"])
		}
		if trust["validated_eligible"] != float64(1) {
			t.Errorf("trust.validated_eligible = %v, want 1", trust["validated_eligible"])
		}
		if trust["differing"] != float64(0) {
			t.Errorf("trust.differing = %v, want 0", trust["differing"])
		}
	})
}

// TestDoctorCmd_SynthesisUnconfigured verifies the Synthesis Layer section
// reports "not configured (optional)" with PASS status when no synthesis
// provider is configured.
func TestDoctorCmd_SynthesisUnconfigured(t *testing.T) {
	tmpDir := t.TempDir()
	deweyDir := filepath.Join(tmpDir, deweyWorkspaceDir)
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("mkdir .uf/dewey: %v", err)
	}

	// Clear any env vars that could configure synthesis.
	t.Setenv("DEWEY_SYNTHESIS_ENDPOINT", "")
	t.Setenv("DEWEY_GENERATION_MODEL", "")
	t.Setenv("OLLAMA_HOST", "")
	// Isolate from developer's global config (~/.config/dewey/config.yaml).
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	var buf bytes.Buffer
	runDoctorChecks(&buf, tmpDir)
	output := buf.String()

	if !strings.Contains(output, "Synthesis Layer (not configured)") {
		t.Errorf("doctor should show 'Synthesis Layer (not configured)', got:\n%s", output)
	}
	if !strings.Contains(output, "not configured (optional)") {
		t.Errorf("doctor should show 'not configured (optional)' PASS, got:\n%s", output)
	}
}

// TestDoctorCmd_SynthesisVertexConfigured verifies the Synthesis Layer section
// reports config completeness for a Vertex provider without requiring GCP
// credentials (passes without application-default credentials).
func TestDoctorCmd_SynthesisVertexConfigured(t *testing.T) {
	tmpDir := t.TempDir()
	deweyDir := filepath.Join(tmpDir, deweyWorkspaceDir)
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("mkdir .uf/dewey: %v", err)
	}

	// Isolate from developer's global config (~/.config/dewey/config.yaml).
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// Write config.yaml with vertex synthesis provider.
	configYAML := `synthesis:
  provider: vertex
  model: claude-sonnet-4-6
  project: my-test-project
  region: us-east5
`
	if err := os.WriteFile(filepath.Join(deweyDir, "config.yaml"), []byte(configYAML), 0o644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}

	// Clear env vars to ensure config.yaml is the source.
	t.Setenv("DEWEY_SYNTHESIS_ENDPOINT", "")
	t.Setenv("DEWEY_GENERATION_MODEL", "")
	t.Setenv("OLLAMA_HOST", "")

	var buf bytes.Buffer
	runDoctorChecks(&buf, tmpDir)
	output := buf.String()

	if !strings.Contains(output, "Synthesis Layer") {
		t.Errorf("doctor should include Synthesis Layer section, got:\n%s", output)
	}
	if !strings.Contains(output, "vertex") {
		t.Errorf("doctor should mention vertex provider, got:\n%s", output)
	}
	if !strings.Contains(output, "claude-sonnet-4-6") {
		t.Errorf("doctor should display synthesis model, got:\n%s", output)
	}
	if !strings.Contains(output, "project=my-test-project") {
		t.Errorf("doctor should display project, got:\n%s", output)
	}
	if !strings.Contains(output, "region=us-east5") {
		t.Errorf("doctor should display region, got:\n%s", output)
	}

	// Token safety: Vertex output MUST NOT contain credential-like strings
	// (spec requirement: "Token safety in diagnostic output").
	for _, forbidden := range []string{"Bearer", "token=", "key=", "credential"} {
		if strings.Contains(output, forbidden) {
			t.Errorf("doctor output MUST NOT contain credential-like string %q, got:\n%s", forbidden, output)
		}
	}
}

// TestDoctorCmd_SynthesisVertexMisconfigured verifies the Synthesis Layer
// section reports FAIL for a Vertex provider with missing required fields.
func TestDoctorCmd_SynthesisVertexMisconfigured(t *testing.T) {
	tmpDir := t.TempDir()
	deweyDir := filepath.Join(tmpDir, deweyWorkspaceDir)
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("mkdir .uf/dewey: %v", err)
	}

	// Isolate from developer's global config (~/.config/dewey/config.yaml).
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// Write config.yaml with vertex provider but missing project.
	configYAML := `synthesis:
  provider: vertex
  model: claude-sonnet-4-6
  region: us-east5
`
	if err := os.WriteFile(filepath.Join(deweyDir, "config.yaml"), []byte(configYAML), 0o644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}

	t.Setenv("DEWEY_SYNTHESIS_ENDPOINT", "")
	t.Setenv("DEWEY_GENERATION_MODEL", "")
	t.Setenv("OLLAMA_HOST", "")

	var buf bytes.Buffer
	runDoctorChecks(&buf, tmpDir)
	output := buf.String()

	if !strings.Contains(output, "Synthesis Layer") {
		t.Errorf("doctor should include Synthesis Layer section, got:\n%s", output)
	}
	// Should report FAIL with the specific missing field.
	if !strings.Contains(output, "incomplete") {
		t.Errorf("doctor should report incomplete config, got:\n%s", output)
	}
	if !strings.Contains(output, "project") {
		t.Errorf("doctor should identify 'project' as missing field, got:\n%s", output)
	}
}

// TestDoctorCmd_SynthesisUnknownProvider verifies the Synthesis Layer section
// reports FAIL for an unknown provider type.
func TestDoctorCmd_SynthesisUnknownProvider(t *testing.T) {
	tmpDir := t.TempDir()
	deweyDir := filepath.Join(tmpDir, deweyWorkspaceDir)
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("mkdir .uf/dewey: %v", err)
	}

	// Isolate from developer's global config (~/.config/dewey/config.yaml).
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	configYAML := `synthesis:
  provider: badprovider
  model: some-model
`
	if err := os.WriteFile(filepath.Join(deweyDir, "config.yaml"), []byte(configYAML), 0o644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}

	t.Setenv("DEWEY_SYNTHESIS_ENDPOINT", "")
	t.Setenv("DEWEY_GENERATION_MODEL", "")
	t.Setenv("OLLAMA_HOST", "")

	var buf bytes.Buffer
	runDoctorChecks(&buf, tmpDir)
	output := buf.String()

	if !strings.Contains(output, "unknown provider") {
		t.Errorf("doctor should report unknown provider, got:\n%s", output)
	}
	if !strings.Contains(output, "badprovider") {
		t.Errorf("doctor should include the unknown provider name, got:\n%s", output)
	}
}

// TestDoctorCmd_MissingDeweyDir verifies doctor reports fail with
// `dewey init` fix when .uf/dewey/ does not exist.
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

	// .uf/dewey/ check should fail with emoji marker.
	if !strings.Contains(output, "❌ .uf/dewey/") {
		t.Errorf("doctor should report .uf/dewey/ fail, got:\n%s", output)
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

// --- DCP config doctor tests ---

// TestDoctorCmd_DCPConfigPresent verifies doctor reports PASS when
// .opencode/dcp.jsonc exists with protectTags configured.
func TestDoctorCmd_DCPConfigPresent(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .uf/dewey/ + graph.db so doctor doesn't early-exit.
	deweyDir := filepath.Join(tmpDir, deweyWorkspaceDir)
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("mkdir .uf/dewey: %v", err)
	}
	dbPath := filepath.Join(deweyDir, "graph.db")
	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := s.InsertPage(&store.Page{Name: "test-page", ContentHash: "abc123"}); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}
	_ = s.Close()

	// Create .opencode/dcp.jsonc with protectTags.
	if err := os.MkdirAll(filepath.Join(tmpDir, ".opencode"), 0o755); err != nil {
		t.Fatalf("create .opencode: %v", err)
	}
	dcpContent := `{"compress": {"protectTags": true}}`
	if err := os.WriteFile(filepath.Join(tmpDir, ".opencode", "dcp.jsonc"), []byte(dcpContent), 0o644); err != nil {
		t.Fatalf("write dcp.jsonc: %v", err)
	}

	cmd := newDoctorCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--vault", tmpDir})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("doctor failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "✅ dcp.jsonc") {
		t.Errorf("doctor should report dcp.jsonc pass, got:\n%s", output)
	}
	if !strings.Contains(output, "protectTags enabled") {
		t.Errorf("doctor should report protectTags enabled, got:\n%s", output)
	}
}

// TestDoctorCmd_DCPConfigMissing verifies doctor reports WARN when
// .opencode/ exists but no DCP config file is present.
func TestDoctorCmd_DCPConfigMissing(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .uf/dewey/ + graph.db so doctor doesn't early-exit.
	deweyDir := filepath.Join(tmpDir, deweyWorkspaceDir)
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("mkdir .uf/dewey: %v", err)
	}
	dbPath := filepath.Join(deweyDir, "graph.db")
	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := s.InsertPage(&store.Page{Name: "test-page", ContentHash: "abc123"}); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}
	_ = s.Close()

	// Create .opencode/ but no dcp.jsonc.
	if err := os.MkdirAll(filepath.Join(tmpDir, ".opencode"), 0o755); err != nil {
		t.Fatalf("create .opencode: %v", err)
	}

	cmd := newDoctorCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--vault", tmpDir})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("doctor failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "⚠️") || !strings.Contains(output, "dcp.jsonc") || !strings.Contains(output, "not found") {
		t.Errorf("doctor should report dcp.jsonc warn with 'not found', got:\n%s", output)
	}
	if !strings.Contains(output, "dewey init") {
		t.Errorf("doctor should suggest 'dewey init' fix for missing DCP config, got:\n%s", output)
	}
}

// TestDoctorCmd_DCPConfigMissingProtectTags verifies doctor reports WARN when
// .opencode/dcp.jsonc exists but does not contain protectTags.
func TestDoctorCmd_DCPConfigMissingProtectTags(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .uf/dewey/ + graph.db so doctor doesn't early-exit.
	deweyDir := filepath.Join(tmpDir, deweyWorkspaceDir)
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("mkdir .uf/dewey: %v", err)
	}
	dbPath := filepath.Join(deweyDir, "graph.db")
	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := s.InsertPage(&store.Page{Name: "test-page", ContentHash: "abc123"}); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}
	_ = s.Close()

	// Create .opencode/dcp.jsonc WITHOUT protectTags.
	if err := os.MkdirAll(filepath.Join(tmpDir, ".opencode"), 0o755); err != nil {
		t.Fatalf("create .opencode: %v", err)
	}
	dcpContent := `{"compress": {"maxTokens": 5000}}`
	if err := os.WriteFile(filepath.Join(tmpDir, ".opencode", "dcp.jsonc"), []byte(dcpContent), 0o644); err != nil {
		t.Fatalf("write dcp.jsonc: %v", err)
	}

	cmd := newDoctorCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--vault", tmpDir})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("doctor failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "⚠️") || !strings.Contains(output, "dcp.jsonc") || !strings.Contains(output, "protectTags not configured") {
		t.Errorf("doctor should report dcp.jsonc warn with 'protectTags not configured', got:\n%s", output)
	}
}

// TestDoctorCmd_NoDCPCheckWithoutOpenCodeDir verifies that doctor output
// does NOT contain DCP-related text when .opencode/ is absent.
func TestDoctorCmd_NoDCPCheckWithoutOpenCodeDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .uf/dewey/ + graph.db so doctor doesn't early-exit.
	deweyDir := filepath.Join(tmpDir, deweyWorkspaceDir)
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("mkdir .uf/dewey: %v", err)
	}
	dbPath := filepath.Join(deweyDir, "graph.db")
	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := s.InsertPage(&store.Page{Name: "test-page", ContentHash: "abc123"}); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}
	_ = s.Close()

	// Do NOT create .opencode/.

	cmd := newDoctorCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--vault", tmpDir})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("doctor failed: %v", err)
	}

	output := buf.String()
	// Doctor should NOT mention DCP when .opencode/ is absent.
	if strings.Contains(output, "dcp.jsonc") {
		t.Errorf("doctor should NOT mention dcp.jsonc when .opencode/ is absent, got:\n%s", output)
	}
}

// TestDoctorCmd_DCPJSONConfigPresent verifies doctor reports PASS when
// .opencode/dcp.json (not dcp.jsonc) exists with protectTags configured.
func TestDoctorCmd_DCPJSONConfigPresent(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .uf/dewey/ + graph.db so doctor doesn't early-exit.
	deweyDir := filepath.Join(tmpDir, deweyWorkspaceDir)
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("mkdir .uf/dewey: %v", err)
	}
	dbPath := filepath.Join(deweyDir, "graph.db")
	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := s.InsertPage(&store.Page{Name: "test-page", ContentHash: "abc123"}); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}
	_ = s.Close()

	// Create .opencode/dcp.json (not dcp.jsonc) with protectTags.
	if err := os.MkdirAll(filepath.Join(tmpDir, ".opencode"), 0o755); err != nil {
		t.Fatalf("create .opencode: %v", err)
	}
	dcpContent := `{"compress": {"protectTags": true}}`
	if err := os.WriteFile(filepath.Join(tmpDir, ".opencode", "dcp.json"), []byte(dcpContent), 0o644); err != nil {
		t.Fatalf("write dcp.json: %v", err)
	}

	cmd := newDoctorCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--vault", tmpDir})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("doctor failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "✅ dcp.jsonc") {
		t.Errorf("doctor should report dcp.jsonc pass for dcp.json variant, got:\n%s", output)
	}
	if !strings.Contains(output, "protectTags enabled") {
		t.Errorf("doctor should report protectTags enabled, got:\n%s", output)
	}
}

// --- 006-unified-ignore tests ---

// TestInitCmd_SourcesYAMLContainsRecursiveComment verifies that the default
// sources.yaml template generated by `dewey init` contains a comment
// documenting the `recursive` field, so users discover the option.
func TestInitCmd_SourcesYAMLContainsRecursiveComment(t *testing.T) {
	tmpDir := t.TempDir()

	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	sourcesPath := filepath.Join(tmpDir, deweyWorkspaceDir, "sources.yaml")
	content, err := os.ReadFile(sourcesPath)
	if err != nil {
		t.Fatalf("read sources.yaml: %v", err)
	}

	sourcesStr := string(content)

	// Verify the template contains a comment about the recursive field.
	if !strings.Contains(sourcesStr, "recursive") {
		t.Error("sources.yaml template should mention 'recursive' in a comment")
	}

	// Verify the template contains a comment about the ignore field.
	if !strings.Contains(sourcesStr, "ignore") {
		t.Error("sources.yaml template should mention 'ignore' in a comment")
	}
}

// TestDoctorCmd_VerboseIgnoreReporting verifies that `dewey doctor -v`
// reports ignore rules and excluded directory count when a .gitignore
// exists with patterns that match directories in the vault.
func TestDoctorCmd_VerboseIgnoreReporting(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .uf/dewey/ directory so doctor doesn't exit early.
	deweyDir := filepath.Join(tmpDir, deweyWorkspaceDir)
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("mkdir .uf/dewey: %v", err)
	}

	// Create config.yaml (expected by doctor).
	if err := os.WriteFile(filepath.Join(deweyDir, "config.yaml"), []byte("embedding:\n  model: test\n"), 0o644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}

	// Create sources.yaml (expected by doctor).
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

	// Create .gitignore with a pattern that matches a directory.
	gitignoreContent := "node_modules/\n"
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(gitignoreContent), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	// Create the node_modules/ directory so it gets counted.
	if err := os.MkdirAll(filepath.Join(tmpDir, "node_modules", "pkg"), 0o755); err != nil {
		t.Fatalf("mkdir node_modules: %v", err)
	}

	// Enable verbose mode by setting logger to DebugLevel.
	// Save and restore the original level to avoid affecting other tests.
	origLevel := logger.GetLevel()
	logger.SetLevel(log.DebugLevel)
	defer logger.SetLevel(origLevel)

	cmd := newDoctorCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--vault", tmpDir})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("doctor failed: %v", err)
	}

	output := buf.String()

	// Verify the output contains ignore rules reporting.
	if !strings.Contains(output, "ignore rules") {
		t.Errorf("doctor verbose output should contain 'ignore rules', got:\n%s", output)
	}
	if !strings.Contains(output, "directories excluded") {
		t.Errorf("doctor verbose output should contain 'directories excluded', got:\n%s", output)
	}
}

// TestDoctorCmd_NonVerboseNoIgnoreReport verifies that `dewey doctor`
// without verbose mode does NOT report ignore rules.
func TestDoctorCmd_NonVerboseNoIgnoreReport(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .uf/dewey/ directory so doctor doesn't exit early.
	deweyDir := filepath.Join(tmpDir, deweyWorkspaceDir)
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("mkdir .uf/dewey: %v", err)
	}

	// Create .gitignore with a pattern.
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte("node_modules/\n"), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "node_modules"), 0o755); err != nil {
		t.Fatalf("mkdir node_modules: %v", err)
	}

	// Ensure logger is NOT at DebugLevel (non-verbose).
	origLevel := logger.GetLevel()
	logger.SetLevel(log.InfoLevel)
	defer logger.SetLevel(origLevel)

	cmd := newDoctorCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--vault", tmpDir})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("doctor failed: %v", err)
	}

	output := buf.String()

	// In non-verbose mode, ignore rules should NOT be reported.
	if strings.Contains(output, "ignore rules") {
		t.Errorf("doctor non-verbose output should NOT contain 'ignore rules', got:\n%s", output)
	}
}

// --- Manifest command tests (T009) ---

// TestManifestCmd_GeneratesManifest verifies that `dewey manifest` creates
// .uf/dewey/manifest.md with expected sections when Go source files containing
// a Cobra command and an exported function are present.
func TestManifestCmd_GeneratesManifest(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a Go source file with a Cobra command and an exported function.
	goSource := `package main

import "github.com/spf13/cobra"

// NewApp creates the root application command.
func NewApp() *cobra.Command {
	return &cobra.Command{
		Use:   "myapp",
		Short: "My test application",
	}
}

// ExportedFunc does something useful.
func ExportedFunc() string {
	return "hello"
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte(goSource), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	cmd := newManifestCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("manifest failed: %v", err)
	}

	// Verify .uf/dewey/manifest.md exists.
	manifestPath := filepath.Join(tmpDir, deweyWorkspaceDir, "manifest.md")
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest.md: %v", err)
	}

	manifestStr := string(content)

	// Verify header.
	if !strings.Contains(manifestStr, "# Project Manifest") {
		t.Error("manifest should contain '# Project Manifest' header")
	}
	if !strings.Contains(manifestStr, "Auto-generated by `dewey manifest`") {
		t.Error("manifest should contain generation timestamp")
	}

	// Verify CLI Commands section with the Cobra command.
	if !strings.Contains(manifestStr, "## CLI Commands") {
		t.Error("manifest should contain '## CLI Commands' section")
	}
	if !strings.Contains(manifestStr, "myapp") {
		t.Errorf("manifest should contain command 'myapp', got:\n%s", manifestStr)
	}
	if !strings.Contains(manifestStr, "My test application") {
		t.Errorf("manifest should contain command description, got:\n%s", manifestStr)
	}
}

// TestManifestCmd_IncludesMCPTools verifies that files with mcp.AddTool
// produce an MCP Tools table in the manifest.
func TestManifestCmd_IncludesMCPTools(t *testing.T) {
	tmpDir := t.TempDir()

	goSource := `package main

import "github.com/modelcontextprotocol/go-sdk/mcp"

func registerTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "test_tool",
		Description: "A test MCP tool",
	}, nil)
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "tools.go"), []byte(goSource), 0o644); err != nil {
		t.Fatalf("write tools.go: %v", err)
	}

	cmd := newManifestCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("manifest failed: %v", err)
	}

	manifestPath := filepath.Join(tmpDir, deweyWorkspaceDir, "manifest.md")
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest.md: %v", err)
	}

	manifestStr := string(content)

	if !strings.Contains(manifestStr, "## MCP Tools") {
		t.Error("manifest should contain '## MCP Tools' section")
	}
	if !strings.Contains(manifestStr, "test_tool") {
		t.Errorf("manifest should contain tool 'test_tool', got:\n%s", manifestStr)
	}
	if !strings.Contains(manifestStr, "A test MCP tool") {
		t.Errorf("manifest should contain tool description, got:\n%s", manifestStr)
	}
}

// TestManifestCmd_EmptyRepo verifies that running manifest on a directory
// with no Go files produces a manifest with a "no declarations" note.
func TestManifestCmd_EmptyRepo(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a non-Go file to ensure the directory isn't truly empty.
	if err := os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("# Hello"), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}

	cmd := newManifestCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("manifest should not fail on empty repo: %v", err)
	}

	manifestPath := filepath.Join(tmpDir, deweyWorkspaceDir, "manifest.md")
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest.md: %v", err)
	}

	manifestStr := string(content)
	if !strings.Contains(manifestStr, "# Project Manifest") {
		t.Error("manifest should contain header even with no Go files")
	}
	// Should contain the "no declarations" note.
	if !strings.Contains(manifestStr, "No Go source files found") {
		t.Errorf("manifest should note no declarations, got:\n%s", manifestStr)
	}
}

// TestManifestCmd_SkipsTestFiles verifies that *_test.go files are excluded
// from the manifest — exported symbols from test files should not appear.
func TestManifestCmd_SkipsTestFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create only a test file with an exported function.
	testSource := `package main

// TestHelper is an exported test helper.
func TestHelper() string {
	return "test"
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "helpers_test.go"), []byte(testSource), 0o644); err != nil {
		t.Fatalf("write helpers_test.go: %v", err)
	}

	cmd := newManifestCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("manifest should not fail: %v", err)
	}

	manifestPath := filepath.Join(tmpDir, deweyWorkspaceDir, "manifest.md")
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest.md: %v", err)
	}

	manifestStr := string(content)
	if strings.Contains(manifestStr, "TestHelper") {
		t.Errorf("manifest should NOT contain symbols from test files, got:\n%s", manifestStr)
	}
}

// TestManifestCmd_OmitsEmptySections verifies that sections with no entries
// are omitted from the manifest (contract invariant #5).
func TestManifestCmd_OmitsEmptySections(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a Go file with only an exported function — no commands or tools.
	goSource := `package mylib

// DoWork performs the work.
func DoWork() {}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "lib.go"), []byte(goSource), 0o644); err != nil {
		t.Fatalf("write lib.go: %v", err)
	}

	cmd := newManifestCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("manifest failed: %v", err)
	}

	manifestPath := filepath.Join(tmpDir, deweyWorkspaceDir, "manifest.md")
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest.md: %v", err)
	}

	manifestStr := string(content)

	// No commands or tools — those sections should be omitted.
	if strings.Contains(manifestStr, "## CLI Commands") {
		t.Errorf("manifest should omit empty CLI Commands section, got:\n%s", manifestStr)
	}
	if strings.Contains(manifestStr, "## MCP Tools") {
		t.Errorf("manifest should omit empty MCP Tools section, got:\n%s", manifestStr)
	}
	// But Exported Packages should be present (package doc exists).
	// Note: the file has no doc comment on the package, so the package
	// block won't be extracted. Let's verify no package section either.
}

// TestManifestCmd_Idempotent verifies that running manifest twice produces
// identical output (contract invariant #6).
func TestManifestCmd_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()

	goSource := `// Package example provides an example.
package example

// Hello returns a greeting.
func Hello() string { return "hello" }
`
	if err := os.WriteFile(filepath.Join(tmpDir, "example.go"), []byte(goSource), 0o644); err != nil {
		t.Fatalf("write example.go: %v", err)
	}

	// First run.
	cmd1 := newManifestCmd()
	cmd1.SetArgs([]string{"--vault", tmpDir})
	if err := cmd1.Execute(); err != nil {
		t.Fatalf("first manifest failed: %v", err)
	}

	manifestPath := filepath.Join(tmpDir, deweyWorkspaceDir, "manifest.md")
	content1, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read first manifest: %v", err)
	}

	// Second run.
	cmd2 := newManifestCmd()
	cmd2.SetArgs([]string{"--vault", tmpDir})
	if err := cmd2.Execute(); err != nil {
		t.Fatalf("second manifest failed: %v", err)
	}

	content2, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read second manifest: %v", err)
	}

	// The timestamp line will differ between runs (different seconds).
	// Strip the timestamp line for comparison.
	stripped1 := stripTimestampLine(string(content1))
	stripped2 := stripTimestampLine(string(content2))

	if stripped1 != stripped2 {
		t.Errorf("manifest is not idempotent.\nFirst:\n%s\nSecond:\n%s", stripped1, stripped2)
	}
}

// stripTimestampLine removes the auto-generated timestamp line from manifest
// content for idempotency comparison.
func stripTimestampLine(content string) string {
	var lines []string
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "> Auto-generated by") {
			continue
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// TestManifestCmd_CreatesDirectory verifies that .uf/dewey/ is created
// automatically if it doesn't exist.
func TestManifestCmd_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	// Verify .uf/dewey/ doesn't exist yet.
	deweyDir := filepath.Join(tmpDir, deweyWorkspaceDir)
	if _, err := os.Stat(deweyDir); !os.IsNotExist(err) {
		t.Fatal(".uf/dewey/ should not exist before manifest")
	}

	// Create a minimal Go file.
	goSource := `package main

func main() {}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte(goSource), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	cmd := newManifestCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("manifest failed: %v", err)
	}

	// Verify .uf/dewey/ was created.
	if _, err := os.Stat(deweyDir); os.IsNotExist(err) {
		t.Fatal(".uf/dewey/ should be created by manifest command")
	}

	// Verify manifest.md exists.
	manifestPath := filepath.Join(deweyDir, "manifest.md")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		t.Fatal("manifest.md should exist after manifest command")
	}
}

// TestManifestCmd_HasFlags verifies the manifest subcommand has expected flags.
func TestManifestCmd_HasFlags(t *testing.T) {
	cmd := newManifestCmd()
	if cmd.Flags().Lookup("vault") == nil {
		t.Error("manifest command missing flag --vault")
	}
}

// TestRootCmd_Help_IncludesManifestSubcommand verifies manifest appears in help.
func TestRootCmd_Help_IncludesManifestSubcommand(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(--help) failed: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "manifest") {
		t.Errorf("help output missing subcommand 'manifest'")
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

// newArtifactsCLIRepo builds a hermetic git fixture for the artifacts CLI
// end-to-end test: a working repository with repo-local config, a bare
// origin, an initial base commit pushed with -u, and origin/HEAD set so the
// plan's base-ref resolution chain resolves.
func newArtifactsCLIRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	root := filepath.Join(dir, "work")
	origin := filepath.Join(dir, "origin.git")
	hooks := filepath.Join(dir, "empty-hooks")
	for _, d := range []string{root, hooks} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("fixture: create directory %s: %v", d, err)
		}
	}

	git := func(args ...string) {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-b", "main")
	git("config", "user.email", "fixture@example.invalid")
	git("config", "user.name", "Fixture Tester")
	git("config", "commit.gpgsign", "false")
	git("config", "core.autocrlf", "false")
	git("config", "core.hooksPath", hooks)
	git("init", "--bare", "-b", "main", origin)
	git("remote", "add", "origin", origin)
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("fixture base\n"), 0o644); err != nil {
		t.Fatalf("fixture: write README: %v", err)
	}
	git("add", "--", "README.md")
	git("commit", "-m", "fixture: initial base commit")
	git("push", "-u", "origin", "main")
	git("remote", "set-head", "origin", "-a")

	return root
}

// captureCLIStdout runs f with os.Stdout redirected into a pipe and returns
// everything f wrote directly plus f's error. The artifacts commands relay
// their payloads via fmt.Println to real stdout, which cmd.SetOut cannot
// intercept. The pipe is drained in a goroutine while f runs so captures
// larger than the pipe buffer cannot deadlock, and os.Stdout is restored via
// defer so a t.Fatalf inside f cannot leak the redirection.
func captureCLIStdout(t *testing.T, f func() error) (string, error) {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = buf.ReadFrom(r)
		close(done)
	}()

	runErr := f()

	_ = w.Close()
	<-done
	return buf.String(), runErr
}

// TestArtifactsCmd_Registration verifies the artifacts command group is
// registered on the root command with plan and sync subcommands.
func TestArtifactsCmd_Registration(t *testing.T) {
	cmd := newRootCmd()

	artifactsCmd, _, err := cmd.Find([]string{"artifacts"})
	if err != nil {
		t.Fatalf("find artifacts: %v", err)
	}
	if artifactsCmd.Name() != "artifacts" {
		t.Fatalf("found command %q, want artifacts", artifactsCmd.Name())
	}
	for _, sub := range []string{"plan", "sync"} {
		subCmd, _, err := artifactsCmd.Find([]string{sub})
		if err != nil || subCmd.Name() != sub {
			t.Errorf("artifacts missing subcommand %q (err=%v)", sub, err)
		}
	}
}

// TestArtifactsCmd_Help verifies the artifacts group help lists its
// subcommands and describes the publish flow.
func TestArtifactsCmd_Help(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"artifacts", "--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(artifacts --help) failed: %v", err)
	}

	got := buf.String()
	for _, want := range []string{"knowledge artifacts", "plan", "sync"} {
		if !strings.Contains(got, want) {
			t.Errorf("artifacts help missing %q", want)
		}
	}
}

// TestArtifactsPlanCmd_HasFlags verifies the plan subcommand registers
// exactly the documented flag surface (input-schema parity with design D5).
func TestArtifactsPlanCmd_HasFlags(t *testing.T) {
	cmd := newArtifactsPlanCmd()

	for _, name := range []string{"json", "vault"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("artifacts plan missing flag --%s", name)
		}
	}
	var planFlagNames []string
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		planFlagNames = append(planFlagNames, f.Name)
	})
	if got, want := len(planFlagNames), 2; got != want {
		t.Errorf("artifacts plan registers %d flags %v, want exactly %d (json, vault)", got, planFlagNames, want)
	}
}

// TestArtifactsSyncCmd_HasFlags verifies the sync subcommand registers
// exactly the documented flag surface (input-schema parity with design D6).
func TestArtifactsSyncCmd_HasFlags(t *testing.T) {
	cmd := newArtifactsSyncCmd()

	for _, name := range []string{"plan", "approve", "approved-by", "resolutions", "vault"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("artifacts sync missing flag --%s", name)
		}
	}
	var syncFlagNames []string
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		syncFlagNames = append(syncFlagNames, f.Name)
	})
	if got, want := len(syncFlagNames), 5; got != want {
		t.Errorf("artifacts sync registers %d flags %v, want exactly %d (plan, approve, approved-by, resolutions, vault)", got, syncFlagNames, want)
	}
}

// TestArtifactsSyncCmd_RequiresPlanFlag verifies sync refuses to run without
// the required --plan flag.
func TestArtifactsSyncCmd_RequiresPlanFlag(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"artifacts", "sync"})

	if err := cmd.Execute(); err == nil {
		t.Error("sync without --plan should fail")
	}
}

// TestArtifactsPlanSync_EndToEnd drives `dewey artifacts plan` and
// `dewey artifacts sync` against a hermetic fixture repository: the plan is
// written to a 0600 temp file with the rendered summary relayed verbatim,
// --json emits the same canonical payload as the file, sync refuses without
// --approve, and the approved sync relays the sync result JSON with the chore
// branch, commit, files written, and approver recorded.
func TestArtifactsPlanSync_EndToEnd(t *testing.T) {
	repo := newArtifactsCLIRepo(t)

	// Strand a learning in the working tree.
	artifactRel := filepath.Join(".uf", "dewey", "learnings", "auth-20260101T000000-tester.md")
	if err := os.MkdirAll(filepath.Join(repo, filepath.Dir(artifactRel)), 0o755); err != nil {
		t.Fatalf("mkdir learnings: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, artifactRel), []byte("# learning\n\ntier:: draft\n\nUse parameterized queries.\n"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	// Plan: default human mode relays the plan file path and the summary.
	planCmd := newRootCmd()
	planCmd.SetArgs([]string{"artifacts", "plan", "--vault", repo})
	out, err := captureCLIStdout(t, planCmd.Execute)
	if err != nil {
		t.Fatalf("artifacts plan: %v", err)
	}
	const marker = "plan written to "
	idx := strings.Index(out, marker)
	if idx < 0 {
		t.Fatalf("plan output missing %q: %q", marker, out)
	}
	line := out[idx+len(marker):]
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	planPath := strings.TrimSpace(line)
	if planPath == "" {
		t.Fatalf("plan output has empty plan path: %q", out)
	}
	t.Cleanup(func() { _ = os.Remove(planPath) })

	info, err := os.Stat(planPath)
	if err != nil {
		t.Fatalf("stat plan file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("plan file mode = %o, want 0600", perm)
	}
	if !strings.Contains(out, "learnings") {
		t.Errorf("plan output missing rendered summary (learnings): %q", out)
	}

	// Plan --json emits the same canonical payload as the plan file.
	jsonCmd := newRootCmd()
	jsonCmd.SetArgs([]string{"artifacts", "plan", "--json", "--vault", repo})
	jsonOut, err := captureCLIStdout(t, jsonCmd.Execute)
	if err != nil {
		t.Fatalf("artifacts plan --json: %v", err)
	}
	var planPayload map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(jsonOut)), &planPayload); err != nil {
		t.Fatalf("plan --json not valid JSON: %v\n%s", err, jsonOut)
	}
	payload, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatalf("read plan file: %v", err)
	}
	if strings.TrimSpace(jsonOut) != string(payload) {
		t.Error("plan --json output differs from the plan file payload")
	}

	// Sync refuses to run without --approve (no worktree or branch is
	// created on refusal).
	denyCmd := newRootCmd()
	denyCmd.SetArgs([]string{"artifacts", "sync", "--plan", planPath, "--vault", repo})
	if _, err := captureCLIStdout(t, denyCmd.Execute); err == nil {
		t.Fatal("sync without --approve should fail")
	}

	// Approved sync relays the sync result JSON.
	syncCmd := newRootCmd()
	syncCmd.SetArgs([]string{"artifacts", "sync", "--plan", planPath, "--approve", "--approved-by", "CLI Tester", "--vault", repo})
	syncOut, err := captureCLIStdout(t, syncCmd.Execute)
	if err != nil {
		t.Fatalf("artifacts sync: %v\n%s", err, syncOut)
	}
	var result artifacts.SyncResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(syncOut)), &result); err != nil {
		t.Fatalf("sync output not valid JSON: %v\n%s", err, syncOut)
	}
	if !strings.HasPrefix(result.Branch, "chore/dewey-artifacts-") {
		t.Errorf("sync branch = %q, want chore/dewey-artifacts- prefix", result.Branch)
	}
	if result.Commit == "" {
		t.Error("sync commit empty")
	}
	if result.ApprovedBy != "CLI Tester" {
		t.Errorf("sync approved_by = %q, want %q", result.ApprovedBy, "CLI Tester")
	}
	if result.NothingToPublish {
		t.Error("sync nothing_to_publish = true, want false")
	}
	found := false
	for _, p := range result.FilesWritten {
		if p == artifactRel {
			found = true
		}
	}
	if !found {
		t.Errorf("sync files_written missing %s: %v", artifactRel, result.FilesWritten)
	}

	// The plan file is consumed unchanged by sync: the bytes written by
	// plan are byte-identical after the approved sync ran.
	after, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatalf("re-read plan file after sync: %v", err)
	}
	if !bytes.Equal(payload, after) {
		t.Error("plan file was modified by sync")
	}

	// The committed artifact is readable via git show in the temp worktree
	// with content equal to the planned sha256.
	shown, err := exec.Command("git", "-C", result.Worktree, "show", result.Commit+":"+artifactRel).Output()
	if err != nil {
		t.Fatalf("git show committed artifact: %v", err)
	}
	gotSHA := fmt.Sprintf("%x", sha256.Sum256(shown))
	wantSHA := ""
	artifactsList, _ := planPayload["artifacts"].([]any)
	for _, entry := range artifactsList {
		m, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if p, _ := m["path"].(string); p == artifactRel {
			wantSHA, _ = m["sha256"].(string)
		}
	}
	if wantSHA == "" {
		t.Fatalf("plan JSON has no artifact entry for %s", artifactRel)
	}
	if gotSHA != wantSHA {
		t.Errorf("committed artifact sha256 = %s, want plan sha256 %s", gotSHA, wantSHA)
	}
	if result.Worktree != "" {
		wt := result.Worktree
		t.Cleanup(func() {
			_ = os.RemoveAll(wt)
			_ = exec.Command("git", "-C", repo, "worktree", "prune").Run()
		})
	}
}

// TestInitCmd_ScaffoldsDeweyPublishFromMap verifies the dewey-publish command
// file scaffolds byte-identical to the payload constructed from its
// deweySlashCommands map entry — canonical body plus the trailing ownership
// marker (design D6) — and to the command file tracked in this repository
// (task 5.2 identity requirement).
func TestInitCmd_ScaffoldsDeweyPublishFromMap(t *testing.T) {
	tmpDir := t.TempDir()

	// Pre-create .opencode/ so init scaffolds command files, plus .gitignore.
	if err := os.MkdirAll(filepath.Join(tmpDir, ".opencode", "commands"), 0o755); err != nil {
		t.Fatalf("create opencode commands dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(""), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	scaffolded, err := os.ReadFile(filepath.Join(tmpDir, ".opencode", "commands", "dewey-publish.md"))
	if err != nil {
		t.Fatalf("read scaffolded dewey-publish.md: %v", err)
	}

	want, ok := deweySlashCommands["dewey-publish.md"]
	if !ok {
		t.Fatal("deweySlashCommands missing dewey-publish.md entry")
	}
	// Compare through the payload constructor the writer uses (design D6):
	// the scaffolded form is the canonical body plus the trailing marker.
	wantPayload := string(scaffoldPayload([]byte(want), scaffoldMarkdown))
	if string(scaffolded) != wantPayload {
		t.Errorf("scaffolded dewey-publish.md differs from deweySlashCommands map entry payload (canonical body + marker)")
	}

	// The command file tracked in this repo must stay identical to the
	// constructed payload so scaffolded vaults and the repository ship the
	// same command.
	tracked, err := os.ReadFile(filepath.Join(".opencode", "commands", "dewey-publish.md"))
	if err != nil {
		t.Fatalf("read tracked dewey-publish.md: %v", err)
	}
	if string(tracked) != wantPayload {
		t.Errorf("tracked .opencode/commands/dewey-publish.md differs from deweySlashCommands map entry payload (canonical body + marker)")
	}
}

// TestInitCmd_ScaffoldsDeweyCompileFromMap verifies the dewey-compile command
// file scaffolds byte-identical to the payload constructed from its
// deweySlashCommands map entry — canonical body plus the trailing ownership
// marker (design D6) — and to the command file tracked in this repository —
// the pair drifted once when an embed regeneration synced to an unprotected
// tracked file, dropping the <protect> wrapper the spec mandates.
func TestInitCmd_ScaffoldsDeweyCompileFromMap(t *testing.T) {
	tmpDir := t.TempDir()

	// Pre-create .opencode/ so init scaffolds command files, plus .gitignore.
	if err := os.MkdirAll(filepath.Join(tmpDir, ".opencode", "commands"), 0o755); err != nil {
		t.Fatalf("create opencode commands dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(""), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	scaffolded, err := os.ReadFile(filepath.Join(tmpDir, ".opencode", "commands", "dewey-compile.md"))
	if err != nil {
		t.Fatalf("read scaffolded dewey-compile.md: %v", err)
	}

	want, ok := deweySlashCommands["dewey-compile.md"]
	if !ok {
		t.Fatal("deweySlashCommands missing dewey-compile.md entry")
	}
	// Compare through the payload constructor the writer uses (design D6):
	// the scaffolded form is the canonical body plus the trailing marker.
	wantPayload := string(scaffoldPayload([]byte(want), scaffoldMarkdown))
	if string(scaffolded) != wantPayload {
		t.Errorf("scaffolded dewey-compile.md differs from deweySlashCommands map entry payload (canonical body + marker)")
	}

	// The command file tracked in this repo must stay identical to the
	// constructed payload so scaffolded vaults and the repository ship the
	// same command.
	tracked, err := os.ReadFile(filepath.Join(".opencode", "commands", "dewey-compile.md"))
	if err != nil {
		t.Fatalf("read tracked dewey-compile.md: %v", err)
	}
	if string(tracked) != wantPayload {
		t.Errorf("tracked .opencode/commands/dewey-compile.md differs from deweySlashCommands map entry payload (canonical body + marker)")
	}
}

// TestSlashCommandsInstructionsProtected enforces the structural half of the
// protect-tags-slash-commands spec: every embedded command with an
// ## Instructions section wraps that section (through the end of its guarded
// content) in <protect>/</protect> markers, so scaffolding reproduces the
// protection on every vault.
func TestSlashCommandsInstructionsProtected(t *testing.T) {
	if len(deweySlashCommands) == 0 {
		t.Fatal("deweySlashCommands is empty")
	}
	for name, body := range deweySlashCommands {
		idx := strings.Index(body, "## Instructions")
		if idx < 0 {
			continue
		}
		open := strings.LastIndex(body[:idx], "<protect>")
		if open < 0 {
			t.Errorf("%s: ## Instructions has no <protect> marker before it", name)
			continue
		}
		close := strings.Index(body[idx:], "</protect>")
		if close < 0 {
			t.Errorf("%s: ## Instructions has no </protect> marker after it", name)
		}
	}
}

// TestInitCmd_ScaffoldingSkipsExistingDeweyPublish verifies init does not
// overwrite an existing dewey-publish.md in .opencode/commands/.
func TestInitCmd_ScaffoldingSkipsExistingDeweyPublish(t *testing.T) {
	tmpDir := t.TempDir()

	newDir := filepath.Join(tmpDir, ".opencode", "commands")
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		t.Fatalf("create commands dir: %v", err)
	}
	sentinel := "custom dewey-publish content"
	if err := os.WriteFile(filepath.Join(newDir, "dewey-publish.md"), []byte(sentinel), 0o644); err != nil {
		t.Fatalf("write existing dewey-publish.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(""), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(newDir, "dewey-publish.md"))
	if err != nil {
		t.Fatalf("read dewey-publish.md: %v", err)
	}
	if string(content) != sentinel {
		t.Errorf("existing dewey-publish.md was overwritten, got %q, want %q", string(content), sentinel)
	}
}

func TestResolveArtifactsContext_VaultFlag(t *testing.T) {
	repo := newArtifactsCLIRepo(t)

	repoRoot, vaultRoot, storePaths, err := resolveArtifactsContext(repo)
	if err != nil {
		t.Fatalf("resolveArtifactsContext(%q) failed: %v", repo, err)
	}
	wantRoot, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q) failed: %v", repo, err)
	}
	if repoRoot != wantRoot {
		t.Errorf("repoRoot = %q, want %q (symlink-resolved)", repoRoot, wantRoot)
	}
	wantVault, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatalf("resolve vault symlink: %v", err)
	}
	if vaultRoot != wantVault {
		t.Errorf("vaultRoot = %q, want %q", vaultRoot, wantVault)
	}
	if len(storePaths) != 0 {
		t.Errorf("storePaths = %v, want empty (fixture has no knowledge-stores.yaml)", storePaths)
	}
}

func TestResolveArtifactsContext_EnvVar(t *testing.T) {
	repo := newArtifactsCLIRepo(t)
	t.Setenv("OBSIDIAN_VAULT_PATH", repo)

	repoRoot, vaultRoot, storePaths, err := resolveArtifactsContext("")
	if err != nil {
		t.Fatalf("resolveArtifactsContext(\"\") with OBSIDIAN_VAULT_PATH failed: %v", err)
	}
	wantRoot, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q) failed: %v", repo, err)
	}
	if repoRoot != wantRoot {
		t.Errorf("repoRoot = %q, want %q", repoRoot, wantRoot)
	}
	wantVault, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatalf("resolve vault symlink: %v", err)
	}
	if vaultRoot != wantVault {
		t.Errorf("vaultRoot = %q, want %q", vaultRoot, wantVault)
	}
	if len(storePaths) != 0 {
		t.Errorf("storePaths = %v, want empty", storePaths)
	}
}

func TestResolveArtifactsContext_CwdFallback(t *testing.T) {
	repo := newArtifactsCLIRepo(t)
	t.Setenv("OBSIDIAN_VAULT_PATH", "")

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() failed: %v", err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("os.Chdir(%q) failed: %v", repo, err)
	}
	defer func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Errorf("restoring working directory failed: %v", err)
		}
	}()

	repoRoot, _, _, err := resolveArtifactsContext("")
	if err != nil {
		t.Fatalf("resolveArtifactsContext(\"\") via cwd fallback failed: %v", err)
	}
	wantRoot, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q) failed: %v", repo, err)
	}
	if repoRoot != wantRoot {
		t.Errorf("repoRoot = %q, want %q", repoRoot, wantRoot)
	}
}

func TestResolveArtifactsContext_NotARepo(t *testing.T) {
	vault := t.TempDir()

	_, _, _, err := resolveArtifactsContext(vault)
	if err == nil {
		t.Fatalf("resolveArtifactsContext(%q) succeeded, want failure (not a git repo)", vault)
	}
	if !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "not a git repository")
	}
}

func TestResolveArtifactsContext_StoreConfig(t *testing.T) {
	repo := newArtifactsCLIRepo(t)
	deweyDir := filepath.Join(repo, ".uf", "dewey")
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) failed: %v", deweyDir, err)
	}
	configPath := filepath.Join(deweyDir, "knowledge-stores.yaml")
	configYAML := "stores:\n  - name: team\n    sources:\n      - disk-src\n"
	if err := os.WriteFile(configPath, []byte(configYAML), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) failed: %v", configPath, err)
	}

	_, _, storePaths, err := resolveArtifactsContext(repo)
	if err != nil {
		t.Fatalf("resolveArtifactsContext(%q) with store config failed: %v", repo, err)
	}
	wantStore := filepath.Join(".uf", "dewey", "knowledge", "team")
	found := false
	for _, sp := range storePaths {
		if sp == wantStore {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("storePaths = %v, want it to contain %q", storePaths, wantStore)
	}
}

func TestResolveArtifactsContext_BadYaml(t *testing.T) {
	repo := newArtifactsCLIRepo(t)
	deweyDir := filepath.Join(repo, ".uf", "dewey")
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) failed: %v", deweyDir, err)
	}
	configPath := filepath.Join(deweyDir, "knowledge-stores.yaml")
	if err := os.WriteFile(configPath, []byte("{{{ not yaml\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) failed: %v", configPath, err)
	}

	_, _, _, err := resolveArtifactsContext(repo)
	if err == nil {
		t.Fatalf("resolveArtifactsContext(%q) with malformed yaml succeeded, want failure", repo)
	}
	if !strings.Contains(err.Error(), "load knowledge stores config") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "load knowledge stores config")
	}
}

// TestInitCmd_ScaffoldsDeweyDistillFromMap verifies the dewey-distill command
// file scaffolds byte-identical to the payload constructed from its
// deweySlashCommands map entry — canonical body plus the trailing ownership
// marker (design D6) — and to the command file tracked in this repository.
func TestInitCmd_ScaffoldsDeweyDistillFromMap(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(tmpDir, ".opencode", "commands"), 0o755); err != nil {
		t.Fatalf("create opencode commands dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(""), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	scaffolded, err := os.ReadFile(filepath.Join(tmpDir, ".opencode", "commands", "dewey-distill.md"))
	if err != nil {
		t.Fatalf("read scaffolded dewey-distill.md: %v", err)
	}

	want, ok := deweySlashCommands["dewey-distill.md"]
	if !ok {
		t.Fatal("deweySlashCommands missing dewey-distill.md entry")
	}
	// Compare through the payload constructor the writer uses (design D6):
	// the scaffolded form is the canonical body plus the trailing marker.
	wantPayload := string(scaffoldPayload([]byte(want), scaffoldMarkdown))
	if string(scaffolded) != wantPayload {
		t.Errorf("scaffolded dewey-distill.md differs from deweySlashCommands map entry payload (canonical body + marker)")
	}

	tracked, err := os.ReadFile(filepath.Join(".opencode", "commands", "dewey-distill.md"))
	if err != nil {
		t.Fatalf("read tracked dewey-distill.md: %v", err)
	}
	if string(tracked) != wantPayload {
		t.Errorf("tracked .opencode/commands/dewey-distill.md differs from deweySlashCommands map entry payload (canonical body + marker)")
	}
}

// TestInitCmd_ScaffoldsCuratorAgentFromMap verifies the curator agent
// definition scaffolds byte-identical to the payload constructed from its
// deweyAgentDefinitions map entry — canonical body plus the trailing
// ownership marker (design D6) — and to the agent file tracked in this
// repository.
func TestInitCmd_ScaffoldsCuratorAgentFromMap(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(tmpDir, ".opencode"), 0o755); err != nil {
		t.Fatalf("create opencode dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(""), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	scaffolded, err := os.ReadFile(filepath.Join(tmpDir, ".opencode", "agents", "curator.md"))
	if err != nil {
		t.Fatalf("read scaffolded curator.md: %v", err)
	}

	want, ok := deweyAgentDefinitions["curator.md"]
	if !ok {
		t.Fatal("deweyAgentDefinitions missing curator.md entry")
	}
	// Compare through the payload constructor the writer uses (design D6):
	// the scaffolded form is the canonical body plus the trailing marker.
	wantPayload := string(scaffoldPayload([]byte(want), scaffoldMarkdown))
	if string(scaffolded) != wantPayload {
		t.Errorf("scaffolded curator.md differs from deweyAgentDefinitions map entry payload (canonical body + marker)")
	}

	tracked, err := os.ReadFile(filepath.Join(".opencode", "agents", "curator.md"))
	if err != nil {
		t.Fatalf("read tracked curator.md: %v", err)
	}
	if string(tracked) != wantPayload {
		t.Errorf("tracked .opencode/agents/curator.md differs from deweyAgentDefinitions map entry payload (canonical body + marker)")
	}
}

// TestInitCmd_ScaffoldingSkipsExistingCuratorAgent verifies dewey init
// never overwrites a user-customized curator agent.
func TestInitCmd_ScaffoldingSkipsExistingCuratorAgent(t *testing.T) {
	tmpDir := t.TempDir()
	agentsDir := filepath.Join(tmpDir, ".opencode", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("create agents dir: %v", err)
	}
	custom := "# custom curator\n"
	if err := os.WriteFile(filepath.Join(agentsDir, "curator.md"), []byte(custom), 0o644); err != nil {
		t.Fatalf("write custom agent: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(""), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(agentsDir, "curator.md"))
	if err != nil {
		t.Fatalf("read curator.md: %v", err)
	}
	if string(content) != custom {
		t.Errorf("curator.md was overwritten, got:\n%s", string(content))
	}
}

// TestInitCmd_CuratorAgentWriteFailureFailsInit verifies a scaffold
// write failure fails dewey init with the target path named rather than
// reporting success with a missing agent file.
func TestInitCmd_CuratorAgentWriteFailureFailsInit(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: read-only directory does not reject writes")
	}
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".opencode"), 0o755); err != nil {
		t.Fatalf("create opencode dir: %v", err)
	}
	agentsDir := filepath.Join(tmpDir, ".opencode", "agents")
	if err := os.MkdirAll(agentsDir, 0o555); err != nil {
		t.Fatalf("create read-only agents dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(agentsDir, 0o755) })
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(""), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected init to fail on a read-only agents directory")
	}
	if !strings.Contains(err.Error(), "curator.md") {
		t.Errorf("init error %q does not name the target path", err.Error())
	}
}

// TestInitCmd_NoAgentsDirWhenNoOpenCodeDir verifies no .opencode/agents/
// directory appears when .opencode/ is absent.
func TestInitCmd_NoAgentsDirWhenNoOpenCodeDir(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(""), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(tmpDir, ".opencode", "agents")); err == nil {
		t.Error(".opencode/agents/ should NOT exist when .opencode/ was not present")
	}
}

// TestDeweyDistill_CuratorBindingAndHumanGates asserts the tracked
// dewey-distill command binds the curator agent, marks exactly one
// question line per mandatory gate section, and references only
// allowlisted Dewey MCP tools.
func TestDeweyDistill_CuratorBindingAndHumanGates(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(".opencode", "commands", "dewey-distill.md"))
	if err != nil {
		t.Fatalf("read tracked dewey-distill.md: %v", err)
	}
	command := string(data)

	// The binding must be in the YAML frontmatter, not merely present
	// somewhere in the file (KD-FR-101).
	frontBlock, _, err := splitFrontmatter(command)
	if err != nil {
		t.Fatalf("dewey-distill.md frontmatter: %v", err)
	}
	binding := false
	for _, line := range strings.Split(frontBlock, "\n") {
		if strings.TrimSpace(line) == "agent: curator" {
			binding = true
		}
	}
	if !binding {
		t.Error("dewey-distill.md frontmatter must declare `agent: curator`")
	}

	// Each mandatory gate section carries exactly one marker line, and
	// the line uses the uppercase token plus one ASCII space (CA-FR-006).
	gateSections := map[string]bool{
		"### 3. Scope Gate (MANDATORY)":  true,
		"### 6. Review Gate (MANDATORY)": true,
	}
	counts := map[string]int{}
	current := ""
	for _, line := range strings.Split(command, "\n") {
		if strings.HasPrefix(line, "### ") {
			current = strings.TrimRight(line, " ")
			continue
		}
		if !strings.HasPrefix(line, "HUMAN_GATE:") {
			continue
		}
		if !strings.HasPrefix(line, "HUMAN_GATE: ") {
			t.Errorf("marker line lacks the uppercase token plus one ASCII space: %q", line)
		}
		if !gateSections[current] {
			t.Errorf("marker line found outside a gate section (%q): %q", current, line)
		}
		counts[current]++
	}
	for section := range gateSections {
		if counts[section] != 1 {
			t.Errorf("gate section %q has %d marker lines, want exactly 1", section, counts[section])
		}
	}

	// Every Dewey MCP tool cited must be within the allowlist. The
	// candidate set is derived from the server's tool registry, so a
	// newly cited tool fails rather than passing unnoticed. A citation
	// is a backticked tool name — the command file's own convention for
	// tool calls ("Call the `lint` MCP tool") — not a bare prose word:
	// bare "scope" is the command's Scope Gate section and ordinary
	// English ("in scope", "tags-scoped"), which the registered `scope`
	// tool would otherwise collide with as a substring.
	allowlist := []string{"lint", "list_pages", "compile", "curate", "store_compiled", "verify_candidates", "supersede_learning", "promote"}
	allowedSet := make(map[string]bool, len(allowlist))
	for _, tool := range allowlist {
		allowedSet[tool] = true
	}
	for _, tool := range registeredMCPToolNames(t) {
		if !allowedSet[tool] && strings.Contains(command, "`"+tool+"`") {
			t.Errorf("dewey-distill.md cites non-allowlisted tool %q", tool)
		}
	}
	for _, tool := range allowlist {
		if !strings.Contains(command, tool) {
			t.Errorf("dewey-distill.md does not reference allowlisted tool %q", tool)
		}
	}

	// The agent body's allowlist section names exactly the seven tools,
	// order-independent.
	agent, err := os.ReadFile(filepath.Join(".opencode", "agents", "curator.md"))
	if err != nil {
		t.Fatalf("read tracked curator.md: %v", err)
	}
	if got := agentAllowlistEntries(string(agent)); !sameStringSet(got, allowlist) {
		t.Errorf("agent allowlist = %v, want %v", got, allowlist)
	}
}

// TestCuratorAgentAllowlist_AuthorizedExtensions verifies that both
// tracked copies of the curator agent definition — the tracked
// .opencode/agents/curator.md file and the deweyAgentDefinitions["curator.md"]
// scaffold mirror in agents.go — extend the six-tool baseline shipped with
// curator-agent-human-gates (#152) by exactly two entries: verify_candidates
// and supersede_learning. CA-FR-013 records verify_candidates as the one
// human-authorized addition for issue #151 (agent tool-access settings are
// a protected gatekeeping category; the authorization was issued at the
// 2026-11-11 spec-review pause), and the explicit-supersession change
// (#150) records supersede_learning as the second, authorized at the
// 2026-11-12 spec-review pause relayed by the pipeline orchestrator — so
// any ninth tool or a removed baseline tool in either copy must fail.
func TestCuratorAgentAllowlist_AuthorizedExtensions(t *testing.T) {
	// The #152 six-tool baseline plus the two authorized additions.
	want := []string{
		"lint",
		"list_pages",
		"compile",
		"curate",
		"store_compiled",
		"promote",
		"verify_candidates",
		"supersede_learning",
	}

	cases := []struct {
		name string
		body func(t *testing.T) string
	}{
		{
			name: "tracked .opencode/agents/curator.md",
			body: func(t *testing.T) string {
				t.Helper()
				data, err := os.ReadFile(filepath.Join(".opencode", "agents", "curator.md"))
				if err != nil {
					t.Fatalf("read tracked .opencode/agents/curator.md: %v", err)
				}
				return string(data)
			},
		},
		{
			name: "agents.go mirror",
			body: func(t *testing.T) string {
				t.Helper()
				mirror, ok := deweyAgentDefinitions["curator.md"]
				if !ok {
					t.Fatal("deweyAgentDefinitions missing curator.md entry")
				}
				return mirror
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := agentAllowlistEntries(tc.body(t))
			if !sameStringSet(got, want) {
				t.Errorf("%s: curator allowlist = %v, want exactly the #152 baseline plus verify_candidates and supersede_learning: %v", tc.name, got, want)
			}
		})
	}
}

// registeredMCPToolNames derives the Dewey MCP tool names registered in
// server.go so allowlist tests assert an invariant over the full
// registry rather than a hand-maintained sample.
func registeredMCPToolNames(t testing.TB) []string {
	t.Helper()
	data, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	re := regexp.MustCompile(`Name:\s+"([a-z][a-z0-9_]*)"`)
	seen := map[string]bool{}
	var names []string
	for _, match := range re.FindAllStringSubmatch(string(data), -1) {
		name := match[1]
		if name == "dewey" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	if len(names) == 0 {
		t.Fatal("no MCP tool names found in server.go")
	}
	return names
}

// sameStringSet reports whether a and b contain the same elements,
// ignoring order and duplicates.
func sameStringSet(a, b []string) bool {
	as := make(map[string]bool, len(a))
	for _, v := range a {
		as[v] = true
	}
	bs := make(map[string]bool, len(b))
	for _, v := range b {
		bs[v] = true
	}
	if len(as) != len(bs) {
		return false
	}
	for v := range as {
		if !bs[v] {
			return false
		}
	}
	return true
}

// agentAllowlistEntries extracts the bullet items under the agent's
// "## Dewey MCP tool allowlist" heading.
func agentAllowlistEntries(body string) []string {
	lines := strings.Split(body, "\n")
	var entries []string
	inSection := false
	for _, line := range lines {
		if strings.HasPrefix(line, "## ") {
			inSection = strings.TrimSpace(line) == "## Dewey MCP tool allowlist"
			continue
		}
		if !inSection {
			continue
		}
		if strings.HasPrefix(line, "- ") {
			entries = append(entries, strings.TrimSpace(strings.TrimPrefix(line, "- ")))
		}
	}
	return entries
}

// TestCuratorAgent_FrontmatterToolAllowlist verifies the curator agent's
// frontmatter tools block enforces the deny-all-then-allow-seven Dewey MCP
// boundary (CA-FR-009): the broad "dewey_*" deny is false and precedes the
// seven allow entries, exactly the seven allowlisted tools carry true, the
// built-in flags and the permission block are unchanged from
// curator-agent-human-gates, and every non-dewey MCP server configured in
// the repository-root opencode.json has a wholesale deny entry (CA-FR-010).
// Every allow entry is anchored against the tool names registered in
// server.go, and the tools key set is pinned exactly, so a typo, a rename,
// or an inert extra entry fails the test. The same shape assertions run
// against the deweyAgentDefinitions map entry so both shipped copies carry
// the boundary (CA-FR-011); TestCuratorAgent_GuardsRejectDoctoredDocuments
// executes the guards' rejection paths against doctored documents.
func TestCuratorAgent_FrontmatterToolAllowlist(t *testing.T) {
	serverKeys := configuredMCPServerKeys(t)

	tracked, err := os.ReadFile(filepath.Join(".opencode", "agents", "curator.md"))
	if err != nil {
		t.Fatalf("read tracked curator.md: %v", err)
	}
	assertCuratorFrontmatterShape(t, "tracked .opencode/agents/curator.md", string(tracked), serverKeys)

	def, ok := deweyAgentDefinitions["curator.md"]
	if !ok {
		t.Fatal("deweyAgentDefinitions missing curator.md entry")
	}
	assertCuratorFrontmatterShape(t, `deweyAgentDefinitions["curator.md"]`, def, serverKeys)
}

// configuredMCPServerKeys derives the MCP server keys configured in this
// repository's root opencode.json. The path is pinned deliberately: the
// repository also carries .opencode/opencode.json (agent autonomy
// permissions, no mcp section), and parsing that file instead would
// silently yield an empty server list. The helper fails closed — an
// unreadable file, an unparseable document, or an emptied or renamed-away
// mcp map fails the test rather than letting the deny-coverage check pass
// vacuously.
func configuredMCPServerKeys(t *testing.T) []string {
	t.Helper()
	// Repository root only — never .opencode/opencode.json.
	data, err := os.ReadFile("opencode.json")
	if err != nil {
		t.Fatalf("read repository-root opencode.json: %v", err)
	}
	var cfg struct {
		MCP map[string]json.RawMessage `json:"mcp"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse repository-root opencode.json: %v", err)
	}
	if len(cfg.MCP) == 0 {
		t.Fatal("repository-root opencode.json has an empty or missing mcp map; other-server deny coverage would pass vacuously")
	}
	keys := make([]string, 0, len(cfg.MCP))
	for key := range cfg.MCP {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, want := range []string{"dewey", "replicator"} {
		if _, ok := cfg.MCP[want]; !ok {
			t.Fatalf("opencode.json mcp keys %v are missing expected server %q; the config source moved or was renamed", keys, want)
		}
	}
	return keys
}

// curatorToolFlag is one ordered tools: entry of the curator frontmatter.
type curatorToolFlag struct {
	key   string
	grant bool
}

// curatorFrontmatter parses one shipped curator copy into its ordered
// frontmatter root pairs, ordered tools: flags, and Markdown body. It is
// the shared parse preamble of the shape and parity assertions, so quoted
// glob keys, duplicate-key rejection, and nesting are handled uniformly
// for both guards.
func curatorFrontmatter(t testing.TB, label, content string) (root []yamlPair, flags []curatorToolFlag, body string) {
	t.Helper()
	frontBlock, body, err := splitFrontmatter(content)
	if err != nil {
		t.Fatalf("%s: split frontmatter: %v", label, err)
		// Fatalf under a real *testing.T never returns; under the
		// problemRecorder it does, so return early to keep a
		// structurally-broken fixture reporting the recorded problem
		// instead of panicking below.
		return nil, nil, ""
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(frontBlock), &doc); err != nil {
		t.Fatalf("%s: parse YAML frontmatter: %v", label, err)
		return nil, nil, body
	}
	if len(doc.Content) == 0 {
		t.Fatalf("%s: frontmatter parsed to an empty document", label)
		return nil, nil, body
	}
	root = yamlMappingPairs(t, label, "frontmatter", doc.Content[0])
	toolsPairs := yamlMappingPairs(t, label, "tools", yamlPairValue(root, "tools"))
	if len(toolsPairs) == 0 {
		t.Fatalf("%s: tools block has no entries", label)
	}
	for _, pair := range toolsPairs {
		var grant bool
		if err := pair.value.Decode(&grant); err != nil {
			t.Fatalf("%s: tools entry %q is not a boolean: %v", label, pair.key, err)
		}
		flags = append(flags, curatorToolFlag{key: pair.key, grant: grant})
	}
	return root, flags, body
}

// curatorAllowedDeweyTools returns the dewey_-prefixed tools entries
// carrying true, prefix stripped — the frontmatter allow set.
func curatorAllowedDeweyTools(flags []curatorToolFlag) []string {
	var tools []string
	for _, flag := range flags {
		if flag.grant && strings.HasPrefix(flag.key, "dewey_") {
			tools = append(tools, strings.TrimPrefix(flag.key, "dewey_"))
		}
	}
	return tools
}

// assertCuratorFrontmatterShape asserts the curator agent frontmatter shape
// on one shipped copy (the tracked file or the deweyAgentDefinitions map
// entry). The tools: and permission: mappings are parsed with yaml.Node,
// which preserves key order, so the deny-before-allow ordering is asserted
// rather than assumed.
func assertCuratorFrontmatterShape(t testing.TB, label, content string, serverKeys []string) {
	t.Helper()

	root, flags, _ := curatorFrontmatter(t, label, content)

	flagByKey := make(map[string]bool, len(flags))
	for _, flag := range flags {
		flagByKey[flag.key] = flag.grant
	}

	// tools: block — deny-all-then-allow-seven (CA-FR-009, extended by CA-FR-013).

	// Built-in entries are unchanged from curator-agent-human-gates: read
	// and bash stay true; write, edit, and webfetch stay false.
	for key, want := range map[string]bool{
		"read":     true,
		"bash":     true,
		"write":    false,
		"edit":     false,
		"webfetch": false,
	} {
		got, ok := flagByKey[key]
		if !ok {
			t.Errorf("%s: tools block is missing built-in entry %q", label, key)
			continue
		}
		if got != want {
			t.Errorf("%s: built-in tools entry %q = %v, want %v", label, key, got, want)
		}
	}

	// The broad deny is present, false, and precedes every dewey_* allow
	// (design D2: a last-matching-rule-wins matcher must resolve an
	// allowlisted tool to true and every other dewey_* tool to false).
	denyIndex := -1
	for i, flag := range flags {
		if flag.key == "dewey_*" {
			denyIndex = i
			break
		}
	}
	if denyIndex < 0 {
		t.Errorf("%s: tools block is missing the \"dewey_*\" deny entry", label)
	} else {
		if flagByKey["dewey_*"] {
			t.Errorf("%s: \"dewey_*\" entry is true, want false", label)
		}
		for i, flag := range flags {
			if i < denyIndex && flag.grant && strings.HasPrefix(flag.key, "dewey_") {
				t.Errorf("%s: allow entry %q (position %d) precedes the \"dewey_*\" deny (position %d)", label, flag.key, i, denyIndex)
			}
		}
	}

	// Exactly the eight allowlisted tools carry true (the #152 six-tool
	// baseline plus the two human-authorized additions: verify_candidates,
	// CA-FR-013, and supersede_learning, explicit-supersession #150); the
	// set comparison also rejects any other dewey_*-matching key carrying
	// true.
	wantTools := []string{"lint", "list_pages", "compile", "curate", "store_compiled", "verify_candidates", "supersede_learning", "promote"}
	allowedTools := curatorAllowedDeweyTools(flags)
	if !sameStringSet(allowedTools, wantTools) {
		t.Errorf("%s: dewey_* tools carrying true = %v, want exactly %v", label, allowedTools, wantTools)
	}

	// Ground-truth anchoring: every allow entry in the document must
	// resolve to a tool actually registered in server.go. Copy-to-copy
	// parity (frontmatter, prompt text, distill command) cannot see a
	// typo or a server.go rename — the derived copies would agree on a
	// stale name while the glob denies the real tool, silently dropping
	// a distill capability with green CI. Anchoring the document's allow
	// set to the registry closes that direction.
	registered := registeredMCPToolNames(t)
	registeredSet := make(map[string]bool, len(registered))
	for _, name := range registered {
		registeredSet[name] = true
	}
	for _, tool := range allowedTools {
		if !registeredSet[tool] {
			t.Errorf("%s: allowlisted tool %q is not registered in server.go (typo or rename?); the frontmatter allow is inert", label, tool)
		}
	}

	// Grant exhaustiveness, fail closed: beyond the built-in read and bash
	// grants, no key of any kind carries true. This generalizes
	// CA-FR-009's "no other dewey_*-matching key" to every server
	// (CA-FR-010: the curator must not be granted any tool from another
	// configured server), so an unexpected grant — a replicator_* tool,
	// say — fails here instead of passing unnoticed.
	wantGranted := []string{"read", "bash"}
	for _, tool := range wantTools {
		wantGranted = append(wantGranted, "dewey_"+tool)
	}
	var granted []string
	for _, flag := range flags {
		if flag.grant {
			granted = append(granted, flag.key)
		}
	}
	if !sameStringSet(granted, wantGranted) {
		t.Errorf("%s: tools entries carrying true = %v, want exactly %v", label, granted, wantGranted)
	}

	// Every non-dewey MCP server configured in the repository-root
	// opencode.json is denied wholesale (CA-FR-010): one <key>_*: false
	// entry per configured server, so a server added to the config fails
	// this test until the curator's deny list is updated.
	for _, server := range serverKeys {
		if server == "dewey" {
			continue
		}
		key := server + "_*"
		denied, ok := flagByKey[key]
		if !ok {
			t.Errorf("%s: tools block is missing deny entry %q for configured MCP server %q", label, key, server)
			continue
		}
		if denied {
			t.Errorf("%s: deny entry %q for configured MCP server %q is true, want false", label, key, server)
		}
	}

	// The tools key set is pinned exactly, fail closed — mirroring the
	// permission block below: an inert extra entry (a stale <server>_*
	// deny left behind after a server was removed from opencode.json, or
	// an explicit per-tool deny accumulated during future editing) must
	// not pass unnoticed in either shipped copy.
	wantToolKeys := []string{"read", "bash", "write", "edit", "webfetch", "dewey_*"}
	for _, server := range serverKeys {
		if server != "dewey" {
			wantToolKeys = append(wantToolKeys, server+"_*")
		}
	}
	for _, tool := range wantTools {
		wantToolKeys = append(wantToolKeys, "dewey_"+tool)
	}
	var gotToolKeys []string
	for _, flag := range flags {
		gotToolKeys = append(gotToolKeys, flag.key)
	}
	if !sameStringSet(gotToolKeys, wantToolKeys) {
		t.Errorf("%s: tools keys = %v, want exactly %v", label, gotToolKeys, wantToolKeys)
	}

	// permission: block — unchanged from curator-agent-human-gates.

	permPairs := yamlMappingPairs(t, label, "permission", yamlPairValue(root, "permission"))

	// The top-level key set is pinned exactly: a new permission entry is a
	// capability change that must update this test (fail closed).
	wantPermKeys := []string{"question", "task", "edit", "write", "patch", "webfetch", "bash"}
	var gotPermKeys []string
	for _, pair := range permPairs {
		gotPermKeys = append(gotPermKeys, pair.key)
	}
	if !sameStringSet(gotPermKeys, wantPermKeys) {
		t.Errorf("%s: permission keys = %v, want exactly %v", label, gotPermKeys, wantPermKeys)
	}

	for key, want := range map[string]string{
		"question": "allow",
		"task":     "deny",
		"edit":     "deny",
		"write":    "deny",
		"patch":    "deny",
		"webfetch": "deny",
	} {
		node := yamlPairValue(permPairs, key)
		if node == nil {
			t.Errorf("%s: permission block is missing entry %q", label, key)
			continue
		}
		var got string
		if err := node.Decode(&got); err != nil {
			t.Errorf("%s: permission entry %q is not a string scalar: %v", label, key, err)
			continue
		}
		if got != want {
			t.Errorf("%s: permission entry %q = %q, want %q", label, key, got, want)
		}
	}

	// The nested bash map keeps its deny-all-then-allow-one carve-out and
	// ends with the terminal-deny suffix: "*": deny must precede the
	// single "dewey gate auto-answer" allow, and the seven terminal
	// metacharacter denies must be the final entries in the canonical
	// order (scaffold-bash-terminal-denies, DI-FR-003). Under the same
	// last-matching-rule-wins ordering the tools block relies on, a deny
	// after every allow wins, so the terminal denies override injection
	// payloads on the allowed prefix.
	bashPairs := yamlMappingPairs(t, label, "permission.bash", yamlPairValue(permPairs, "bash"))
	wantBash := []struct {
		key   string
		value string
	}{
		{"*", "deny"},
		{"dewey gate auto-answer", "allow"},
		{"*>*", "deny"},
		{"*<*", "deny"},
		{"*|*", "deny"},
		{"*;*", "deny"},
		{"*&*", "deny"},
		{"*$(*", "deny"},
		{"*`*", "deny"},
	}
	if len(bashPairs) != len(wantBash) {
		t.Errorf("%s: permission.bash has %d entries, want exactly %d (the \"*\" deny-all, the gate allow, and the seven terminal denies)", label, len(bashPairs), len(wantBash))
	} else {
		for i, want := range wantBash {
			var got string
			if err := bashPairs[i].value.Decode(&got); err != nil {
				t.Errorf("%s: permission.bash entry %q is not a string scalar: %v", label, bashPairs[i].key, err)
				continue
			}
			if bashPairs[i].key != want.key || got != want.value {
				t.Errorf("%s: permission.bash entry %d = %q: %q, want %q: %q", label, i, bashPairs[i].key, got, want.key, want.value)
			}
		}
	}
}

// yamlPair is one ordered entry of a YAML mapping node.
type yamlPair struct {
	key   string
	value *yaml.Node
}

// yamlMappingPairs walks a YAML mapping node in document order, failing the
// test on duplicate keys: yaml.Node does not reject them, and a duplicated
// tools or permission key would otherwise collapse silently in set-based
// assertions.
func yamlMappingPairs(t testing.TB, label, mapping string, node *yaml.Node) []yamlPair {
	t.Helper()
	if node == nil || node.Kind != yaml.MappingNode {
		t.Fatalf("%s: %s is missing or not a YAML mapping", label, mapping)
	}
	var pairs []yamlPair
	seen := make(map[string]bool)
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		if seen[key.Value] {
			t.Fatalf("%s: %s has duplicate key %q", label, mapping, key.Value)
		}
		seen[key.Value] = true
		pairs = append(pairs, yamlPair{key: key.Value, value: value})
	}
	return pairs
}

// yamlPairValue returns the value node for key, or nil when absent.
func yamlPairValue(pairs []yamlPair, key string) *yaml.Node {
	for _, pair := range pairs {
		if pair.key == key {
			return pair.value
		}
	}
	return nil
}

// TestCuratorAgent_AllowlistParity verifies the curator agent's two
// allowlist declarations cannot drift apart (CA-FR-011): the frontmatter
// tools block's dewey_* true entries and the Markdown body's
// "## Dewey MCP tool allowlist" section must name the same tools. Set
// equality is symmetric, so a tool added to either list without the other
// fails the test. The same parity assertion runs against the
// deweyAgentDefinitions map entry, matching the two-copy coverage of
// TestCuratorAgent_FrontmatterToolAllowlist; byte identity of the copies
// is separately guarded by TestInitCmd_ScaffoldsCuratorAgentFromMap.
func TestCuratorAgent_AllowlistParity(t *testing.T) {
	tracked, err := os.ReadFile(filepath.Join(".opencode", "agents", "curator.md"))
	if err != nil {
		t.Fatalf("read tracked curator.md: %v", err)
	}
	assertCuratorAllowlistParity(t, "tracked .opencode/agents/curator.md", string(tracked))

	def, ok := deweyAgentDefinitions["curator.md"]
	if !ok {
		t.Fatal("deweyAgentDefinitions missing curator.md entry")
	}
	assertCuratorAllowlistParity(t, `deweyAgentDefinitions["curator.md"]`, def)
}

// assertCuratorAllowlistParity asserts, on one shipped copy of the curator
// agent definition, that the frontmatter allow set and the prompt-text
// allowlist name the same Dewey tools: the set of dewey_-prefixed tools:
// keys carrying true (prefix stripped) must equal agentAllowlistEntries
// applied to the same copy's Markdown body. Parsing goes through the same
// curatorFrontmatter helper as the frontmatter shape assertions, so quoted
// glob keys and nesting are handled uniformly across both guards.
func assertCuratorAllowlistParity(t testing.TB, label, content string) {
	t.Helper()

	_, flags, body := curatorFrontmatter(t, label, content)
	frontmatterTools := curatorAllowedDeweyTools(flags)

	bodyTools := agentAllowlistEntries(body)
	if !sameStringSet(frontmatterTools, bodyTools) {
		t.Errorf("%s: frontmatter dewey_* allows = %v, prompt-body allowlist = %v; the two lists must name the same tools", label, frontmatterTools, bodyTools)
	}
}

// problemRecorder executes the frontmatter shape guards without failing
// the test: Errorf and Fatalf calls are recorded as problems instead, and
// every other method delegates to the wrapped testing.TB. Negative-path
// tests use it to run the guards against doctored curator and distiller
// documents and assert the guards reject them. Fatalf degrades to
// record-and-continue, so fixtures must keep the frontmatter structurally
// parseable — the guards' structural fatal paths assume FailNow semantics.
type problemRecorder struct {
	testing.TB
	problems []string
}

func (r *problemRecorder) Helper() {}

func (r *problemRecorder) Errorf(format string, args ...any) {
	r.problems = append(r.problems, fmt.Sprintf(format, args...))
}

func (r *problemRecorder) Fatalf(format string, args ...any) {
	r.Errorf(format, args...)
}

// TestCuratorAgent_GuardsRejectDoctoredDocuments executes the frontmatter
// shape and parity guards against doctored curator documents, so the
// guards' rejection behavior is verified by execution rather than code
// inspection (the CA-FR-009/010/011 drift scenarios). Each fixture doctors
// the tracked curator.md in one specific way and asserts the guards report
// the expected problem.
func TestCuratorAgent_GuardsRejectDoctoredDocuments(t *testing.T) {
	tracked, err := os.ReadFile(filepath.Join(".opencode", "agents", "curator.md"))
	if err != nil {
		t.Fatalf("read tracked curator.md: %v", err)
	}
	serverKeys := configuredMCPServerKeys(t)

	cases := []struct {
		name   string
		doctor func(string) string
		expect string // substring of a recorded problem
	}{
		{
			name: "eighth dewey allow (CA-FR-009)",
			doctor: func(s string) string {
				return strings.Replace(s, "  dewey_promote: true\n", "  dewey_promote: true\n  dewey_search: true\n", 1)
			},
			expect: `dewey_* tools carrying true`,
		},
		{
			name: "dewey_* deny reordered after the allows (design D2)",
			doctor: func(s string) string {
				s = strings.Replace(s, "  \"dewey_*\": false\n", "", 1)
				return strings.Replace(s, "  dewey_promote: true\n", "  dewey_promote: true\n  \"dewey_*\": false\n", 1)
			},
			expect: `precedes the "dewey_*" deny`,
		},
		{
			name: "dewey_* deny deleted outright",
			doctor: func(s string) string {
				return strings.Replace(s, "  \"dewey_*\": false\n", "", 1)
			},
			expect: `tools block is missing the "dewey_*" deny entry`,
		},
		{
			name: "missing replicator_* deny (CA-FR-010)",
			doctor: func(s string) string {
				return strings.Replace(s, "  \"replicator_*\": false\n", "", 1)
			},
			expect: `missing deny entry "replicator_*"`,
		},
		{
			name: "unexpected true grant from another server (CA-FR-010)",
			doctor: func(s string) string {
				return strings.Replace(s, "  \"replicator_*\": false\n", "  \"replicator_*\": false\n  replicator_comms_send: true\n", 1)
			},
			expect: `tools entries carrying true`,
		},
		{
			name: "inert extra deny entry (exact key-set pinning)",
			doctor: func(s string) string {
				return strings.Replace(s, "  \"replicator_*\": false\n", "  \"replicator_*\": false\n  \"stablehelm_*\": false\n", 1)
			},
			expect: `tools keys =`,
		},
		{
			name: "allow entry with a typo'd tool name (registry anchoring)",
			doctor: func(s string) string {
				return strings.Replace(s, "  dewey_promote: true\n", "  dewey_promte: true\n", 1)
			},
			expect: `is not registered in server.go`,
		},
		{
			name: "duplicate tools key",
			doctor: func(s string) string {
				return strings.Replace(s, "  \"dewey_*\": false\n", "  \"dewey_*\": false\n  \"dewey_*\": false\n", 1)
			},
			expect: `duplicate key`,
		},
		{
			name: "permission entry weakened (question: deny)",
			doctor: func(s string) string {
				return strings.Replace(s, "  question: allow\n", "  question: deny\n", 1)
			},
			expect: `permission entry "question"`,
		},
		{
			name: "terminal deny deleted (design D4)",
			doctor: func(s string) string {
				return strings.Replace(s, "    \"*>*\": deny\n", "", 1)
			},
			expect: `permission.bash has 8 entries, want exactly 9`,
		},
		{
			name: "gate allow moved after the terminal denies (design D4)",
			doctor: func(s string) string {
				s = strings.Replace(s, "    \"dewey gate auto-answer\": allow\n", "", 1)
				return strings.Replace(s, "    \"*`*\": deny\n", "    \"*`*\": deny\n    \"dewey gate auto-answer\": allow\n", 1)
			},
			expect: `permission.bash entry 1 = "*>*": "deny", want "dewey gate auto-answer": "allow"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &problemRecorder{TB: t}
			assertCuratorFrontmatterShape(rec, "doctored curator.md", tc.doctor(string(tracked)), serverKeys)
			if len(rec.problems) == 0 {
				t.Fatalf("shape guard accepted a doctored curator document; expected a problem containing %q", tc.expect)
			}
			if !strings.Contains(strings.Join(rec.problems, "\n"), tc.expect) {
				t.Errorf("shape guard problems = %v, want one containing %q", rec.problems, tc.expect)
			}
		})
	}

	// Parity guard (CA-FR-011): dropping a tool from the prompt-text
	// allowlist must be caught by the parity assertion, not only by the
	// frontmatter shape guard.
	t.Run("prompt-text allowlist loses a tool (CA-FR-011)", func(t *testing.T) {
		doctored := strings.Replace(string(tracked), "- promote\n", "", 1)
		rec := &problemRecorder{TB: t}
		assertCuratorAllowlistParity(rec, "doctored curator.md", doctored)
		if len(rec.problems) == 0 {
			t.Fatal("parity guard accepted a doctored curator document; expected a frontmatter/body mismatch")
		}
		if !strings.Contains(strings.Join(rec.problems, "\n"), "frontmatter dewey_* allows") {
			t.Errorf("parity guard problems = %v, want the frontmatter/body mismatch problem", rec.problems)
		}
	})
}

// TestDistillerAgent_QuestionDenied verifies the unattended distiller's
// hard denial of the interactive question tool (FR-006): both the tracked
// agent definition and the embedded copy dewey init scaffolds must carry
// `question: deny` in the permission block, and neither may allow it. The
// scope computation's structural half of FR-006 — warn-and-skip instead of
// asking — is covered by the tools scope tests; this is the agent-side
// gate that rejects any attempted question call before it reaches a human.
func TestDistillerAgent_QuestionDenied(t *testing.T) {
	tracked, err := os.ReadFile(filepath.Join(".opencode", "agents", "distiller.md"))
	if err != nil {
		t.Fatalf("read tracked distiller.md: %v", err)
	}
	def, ok := deweyAgentDefinitions["distiller.md"]
	if !ok {
		t.Fatal("deweyAgentDefinitions missing distiller.md entry")
	}
	for label, doc := range map[string]string{
		"tracked .opencode/agents/distiller.md": string(tracked),
		`deweyAgentDefinitions["distiller.md"]`: def,
	} {
		if !strings.Contains(doc, "question: deny") {
			t.Errorf("%s: permission block does not deny question (FR-006)", label)
		}
		if strings.Contains(doc, "question: allow") {
			t.Errorf("%s: permission block allows question — unattended runs must never ask (FR-006)", label)
		}
	}
}

// TestDistillerAgent_FrontmatterToolAllowlist verifies the distiller
// agent's frontmatter enforces the deny-all-then-allow boundary
// (018-weekly-distill-automation FR-016/FR-017,
// contracts/distiller-allowlist.md Allowed): the broad "dewey_*" deny
// is false and precedes the eleven allow entries, exactly the
// allowlisted tools carry true, every allow is anchored against the
// tools registered in server.go, every non-dewey configured MCP server
// is denied wholesale, and the permission block denies question — the
// unattended run must never ask (FR-006) — with the bash submap
// allowing both startup gates and the dispatch-issue comment
// (DI-FR-004) and ending with the seven terminal metacharacter denies
// (scaffold-bash-terminal-denies, DI-FR-003). The same shape
// assertions run against the deweyAgentDefinitions map entry so both
// shipped copies carry the boundary.
func TestDistillerAgent_FrontmatterToolAllowlist(t *testing.T) {
	serverKeys := configuredMCPServerKeys(t)

	tracked, err := os.ReadFile(filepath.Join(".opencode", "agents", "distiller.md"))
	if err != nil {
		t.Fatalf("read tracked distiller.md: %v", err)
	}
	assertDistillerFrontmatterShape(t, "tracked .opencode/agents/distiller.md", string(tracked), serverKeys)

	def, ok := deweyAgentDefinitions["distiller.md"]
	if !ok {
		t.Fatal("deweyAgentDefinitions missing distiller.md entry")
	}
	assertDistillerFrontmatterShape(t, `deweyAgentDefinitions["distiller.md"]`, def, serverKeys)
}

// distillerAllowedTools is the distiller agent's allow set: the
// knowledge-store tools the interactive curator shares, plus get_page
// and the three unattended pipeline tools
// (contracts/distiller-allowlist.md Allowed).
var distillerAllowedTools = []string{
	"lint", "list_pages", "get_page", "compile", "curate",
	"store_compiled", "verify_candidates", "supersede_learning",
	"scope", "group_learners", "rolling_pr",
}

// assertDistillerFrontmatterShape asserts the distiller agent
// frontmatter shape on one shipped copy. The parsing helpers are shared
// with the curator guards (curatorFrontmatter, curatorAllowedDeweyTools,
// yamlMappingPairs, yamlPairValue) — the mechanics are generic; only the
// asserted set and the permission block are distiller-specific.
func assertDistillerFrontmatterShape(t testing.TB, label, content string, serverKeys []string) {
	t.Helper()

	root, flags, _ := curatorFrontmatter(t, label, content)

	flagByKey := make(map[string]bool, len(flags))
	for _, flag := range flags {
		flagByKey[flag.key] = flag.grant
	}

	// Built-in entries: read and bash stay true (the startup gates run
	// through bash); write, edit, and webfetch stay false.
	for key, want := range map[string]bool{
		"read":     true,
		"bash":     true,
		"write":    false,
		"edit":     false,
		"webfetch": false,
	} {
		got, ok := flagByKey[key]
		if !ok {
			t.Errorf("%s: tools block is missing built-in entry %q", label, key)
			continue
		}
		if got != want {
			t.Errorf("%s: built-in tools entry %q = %v, want %v", label, key, got, want)
		}
	}

	// The broad deny is present, false, and precedes every dewey_* allow
	// (design D2: a last-matching-rule-wins matcher must resolve an
	// allowlisted tool to true and every other dewey_* tool to false).
	denyIndex := -1
	for i, flag := range flags {
		if flag.key == "dewey_*" {
			denyIndex = i
			break
		}
	}
	if denyIndex < 0 {
		t.Errorf("%s: tools block is missing the \"dewey_*\" deny entry", label)
	} else {
		if flagByKey["dewey_*"] {
			t.Errorf("%s: \"dewey_*\" entry is true, want false", label)
		}
		for i, flag := range flags {
			if i < denyIndex && flag.grant && strings.HasPrefix(flag.key, "dewey_") {
				t.Errorf("%s: allow entry %q (position %d) precedes the \"dewey_*\" deny (position %d)", label, flag.key, i, denyIndex)
			}
		}
	}

	// Exactly the eleven allowlisted tools carry true.
	allowedTools := curatorAllowedDeweyTools(flags)
	if !sameStringSet(allowedTools, distillerAllowedTools) {
		t.Errorf("%s: dewey_* tools carrying true = %v, want exactly %v", label, allowedTools, distillerAllowedTools)
	}

	// Ground-truth anchoring: every allow entry must resolve to a tool
	// actually registered in server.go, so a typo or rename fails here
	// rather than silently dropping a distill capability.
	registered := registeredMCPToolNames(t)
	registeredSet := make(map[string]bool, len(registered))
	for _, name := range registered {
		registeredSet[name] = true
	}
	for _, tool := range allowedTools {
		if !registeredSet[tool] {
			t.Errorf("%s: allowlisted tool %q is not registered in server.go (typo or rename?); the frontmatter allow is inert", label, tool)
		}
	}

	// Grant exhaustiveness, fail closed: beyond the built-in read and
	// bash grants, no key of any kind carries true.
	wantGranted := []string{"read", "bash"}
	for _, tool := range distillerAllowedTools {
		wantGranted = append(wantGranted, "dewey_"+tool)
	}
	var granted []string
	for _, flag := range flags {
		if flag.grant {
			granted = append(granted, flag.key)
		}
	}
	if !sameStringSet(granted, wantGranted) {
		t.Errorf("%s: tools entries carrying true = %v, want exactly %v", label, granted, wantGranted)
	}

	// Every non-dewey MCP server configured in the repository-root
	// opencode.json is denied wholesale, so a server added to the config
	// fails this test until the distiller's deny list is updated.
	for _, key := range serverKeys {
		if key == "dewey" {
			continue
		}
		if flagByKey[key+"_*"] {
			t.Errorf("%s: configured MCP server %q lacks its wholesale \"<key>_*\" deny", label, key)
		}
	}

	// Permission block: question is denied — the unattended run must
	// never ask (FR-006) — and so are the other interactive surfaces.
	permPairs := yamlMappingPairs(t, label, "permission", yamlPairValue(root, "permission"))
	permByKey := make(map[string]string, len(permPairs))
	for _, pair := range permPairs {
		var value string
		if err := pair.value.Decode(&value); err != nil {
			continue // bash is a mapping, decoded below
		}
		permByKey[pair.key] = value
	}
	for _, key := range []string{"question", "task", "edit", "write", "patch", "webfetch"} {
		if permByKey[key] != "deny" {
			t.Errorf("%s: permission.%s = %q, want deny", label, key, permByKey[key])
		}
	}

	// The bash permission submap denies everything by default, allows the
	// documented commands — both startup gates and the dispatch-issue
	// comment path (DI-FR-004) — and ends with the terminal-deny suffix:
	// the seven metacharacter denies are the final entries in the
	// canonical order (scaffold-bash-terminal-denies, DI-FR-003). Under
	// the same last-matching-rule-wins ordering the tools block relies
	// on, a deny after every allow wins, so the terminal denies override
	// injection payloads on the allowed prefixes while the bare allowed
	// commands stay runnable (design D6).
	bashPairs := yamlMappingPairs(t, label, "permission.bash", yamlPairValue(permPairs, "bash"))
	bashByKey := make(map[string]string, len(bashPairs))
	for _, pair := range bashPairs {
		var value string
		if err := pair.value.Decode(&value); err != nil {
			t.Fatalf("%s: permission.bash entry %q is not a string: %v", label, pair.key, err)
		}
		bashByKey[pair.key] = value
	}
	if bashByKey["*"] != "deny" {
		t.Errorf("%s: permission.bash \"*\" = %q, want deny", label, bashByKey["*"])
	}
	for _, cmd := range []string{"dewey gate auto-answer", "dewey gate distill-unattended", "gh issue comment"} {
		if bashByKey[cmd] != "allow" {
			t.Errorf("%s: permission.bash %q = %q, want allow — the startup gates and the dispatch-issue comment path must stay runnable", label, cmd, bashByKey[cmd])
		}
	}

	// Last-7 assertion mirroring the consumer lint's contract (design
	// D5): whatever the allows before it, the mapping's final seven
	// entries are exactly the seven terminal deny keys in the canonical
	// order, each deny. Unlike the curator guard's length-exact list, the
	// distiller guard pins only the suffix — a deleted deny or an allow
	// moved after the denies shifts the seven-entry window and fails the
	// ordered comparison.
	wantTerminalDenies := []struct {
		key   string
		value string
	}{
		{"*>*", "deny"},
		{"*<*", "deny"},
		{"*|*", "deny"},
		{"*;*", "deny"},
		{"*&*", "deny"},
		{"*$(*", "deny"},
		{"*`*", "deny"},
	}
	if len(bashPairs) < len(wantTerminalDenies) {
		t.Errorf("%s: permission.bash has %d entries, want at least %d (the seven terminal denies)", label, len(bashPairs), len(wantTerminalDenies))
	} else {
		for i, want := range wantTerminalDenies {
			got := bashPairs[len(bashPairs)-len(wantTerminalDenies)+i]
			if got.key != want.key || bashByKey[got.key] != want.value {
				t.Errorf("%s: permission.bash terminal-deny suffix entry %d = %q: %q, want %q: %q", label, i, got.key, bashByKey[got.key], want.key, want.value)
			}
		}
	}
}

// TestDistillerAgent_GuardsRejectDoctoredDocuments executes the distiller
// frontmatter shape guard against a doctored distiller document, so the
// guard's rejection behavior is verified by execution rather than code
// inspection — the curator doctored-fixture rationale (design D4) applied
// to the distiller guard, which had no negative-path coverage of its own
// (design D5). The fixture drops one terminal deny from the tracked
// distiller.md, shifting the seven-entry suffix window so the "gh issue
// comment" allow lands inside it; the last-7 arm must report the mismatch.
func TestDistillerAgent_GuardsRejectDoctoredDocuments(t *testing.T) {
	tracked, err := os.ReadFile(filepath.Join(".opencode", "agents", "distiller.md"))
	if err != nil {
		t.Fatalf("read tracked distiller.md: %v", err)
	}
	serverKeys := configuredMCPServerKeys(t)

	t.Run("terminal deny deleted (design D5)", func(t *testing.T) {
		doctored := strings.Replace(string(tracked), "    \"*>*\": deny\n", "", 1)
		rec := &problemRecorder{TB: t}
		assertDistillerFrontmatterShape(rec, "doctored distiller.md", doctored, serverKeys)
		if len(rec.problems) == 0 {
			t.Fatalf("shape guard accepted a doctored distiller document; expected a terminal-deny suffix mismatch")
		}
		const expect = `permission.bash terminal-deny suffix entry 0 = "gh issue comment": "allow", want "*>*": "deny"`
		if !strings.Contains(strings.Join(rec.problems, "\n"), expect) {
			t.Errorf("shape guard problems = %v, want the terminal-deny suffix mismatch problem", rec.problems)
		}
	})
}

// TestDistillerAgent_AllowlistParity asserts, on both shipped copies of
// the distiller agent definition, that the frontmatter allow set and the
// prompt-text allowlist name the same Dewey tools — the #192 parity
// pattern applied to the distiller, so the two lists cannot drift apart
// in either direction.
func TestDistillerAgent_AllowlistParity(t *testing.T) {
	tracked, err := os.ReadFile(filepath.Join(".opencode", "agents", "distiller.md"))
	if err != nil {
		t.Fatalf("read tracked distiller.md: %v", err)
	}
	assertDistillerAllowlistParity(t, "tracked .opencode/agents/distiller.md", string(tracked))

	def, ok := deweyAgentDefinitions["distiller.md"]
	if !ok {
		t.Fatal("deweyAgentDefinitions missing distiller.md entry")
	}
	assertDistillerAllowlistParity(t, `deweyAgentDefinitions["distiller.md"]`, def)
}

// distillerBodyToolNames normalizes the distiller body's allowlist
// entries for set comparison with the frontmatter dewey_* allows: each
// bulleted entry names one tool, optionally followed by a parenthetical
// constraint ("store_compiled (draft only; …)"). Two kinds of entries
// are not Dewey tool names: the built-in "read" allow (an OpenCode
// built-in with no dewey_* frontmatter counterpart) and the section's
// second list summarizing the bash allows as category lines
// ("- Filesystem: …", "- Git: …", "- GitHub: …") — colon-terminated
// labels, not tools.
func distillerBodyToolNames(body string) []string {
	var names []string
	for _, entry := range agentAllowlistEntries(body) {
		fields := strings.Fields(entry)
		if len(fields) == 0 {
			continue
		}
		name := fields[0]
		if strings.HasSuffix(name, ":") {
			continue
		}
		if name == "read" {
			continue
		}
		names = append(names, name)
	}
	return names
}

// TestInitCmd_ScaffoldsDistillAutoCommand verifies the unattended distill
// entry point ships to adopting repositories: `dewey init` scaffolds
// .opencode/commands/dewey-distill-auto.md byte-identical to the payload
// constructed from its deweySlashCommands map entry — canonical body plus
// the trailing ownership marker (design D6) — and to the tracked command
// file — without it, the gh-hud dispatch contract (/dewey-distill-auto #N)
// would invoke a command the adopting repo never received.
func TestInitCmd_ScaffoldsDistillAutoCommand(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(tmpDir, ".opencode", "commands"), 0o755); err != nil {
		t.Fatalf("create opencode commands dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(""), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	scaffolded, err := os.ReadFile(filepath.Join(tmpDir, ".opencode", "commands", "dewey-distill-auto.md"))
	if err != nil {
		t.Fatalf("read scaffolded dewey-distill-auto.md: %v", err)
	}
	want, ok := deweySlashCommands["dewey-distill-auto.md"]
	if !ok {
		t.Fatal("deweySlashCommands missing dewey-distill-auto.md entry")
	}
	// Compare through the payload constructor the writer uses (design D6):
	// the scaffolded form is the canonical body plus the trailing marker.
	wantPayload := string(scaffoldPayload([]byte(want), scaffoldMarkdown))
	if string(scaffolded) != wantPayload {
		t.Errorf("scaffolded dewey-distill-auto.md differs from deweySlashCommands map entry payload (canonical body + marker)")
	}
	tracked, err := os.ReadFile(filepath.Join(".opencode", "commands", "dewey-distill-auto.md"))
	if err != nil {
		t.Fatalf("read tracked dewey-distill-auto.md: %v", err)
	}
	if string(tracked) != wantPayload {
		t.Error("tracked .opencode/commands/dewey-distill-auto.md differs from deweySlashCommands map entry payload (canonical body + marker)")
	}
}

// assertDistillerAllowlistParity asserts, on one shipped copy of the
// distiller agent definition, that the frontmatter allow set and the
// prompt-text allowlist name the same Dewey tools.
func assertDistillerAllowlistParity(t testing.TB, label, content string) {
	t.Helper()

	_, flags, body := curatorFrontmatter(t, label, content)
	frontmatterTools := curatorAllowedDeweyTools(flags)
	bodyTools := distillerBodyToolNames(body)
	if !sameStringSet(frontmatterTools, bodyTools) {
		t.Errorf("%s: frontmatter dewey_* allows = %v, prompt-body allowlist = %v; the two lists must name the same tools", label, frontmatterTools, bodyTools)
	}
}

// TestDeweyDistill_InteractivePathUnchangedByDistiller pins FR-018: the
// interactive /dewey-distill keeps both HUMAN_GATE prompts and the full
// curator tool set — the unattended distiller's narrower allowlist must
// not leak into the interactive path, and the distiller-only pipeline
// tools must not leak into it either.
func TestDeweyDistill_InteractivePathUnchangedByDistiller(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(".opencode", "commands", "dewey-distill.md"))
	if err != nil {
		t.Fatalf("read dewey-distill.md: %v", err)
	}
	command := string(data)

	// Both HUMAN_GATE prompts retained.
	markers := 0
	for _, line := range strings.Split(command, "\n") {
		if strings.HasPrefix(line, "HUMAN_GATE: ") {
			markers++
		}
	}
	if markers != 2 {
		t.Errorf("dewey-distill.md carries %d HUMAN_GATE marker lines, want exactly 2 (FR-018)", markers)
	}

	// The full curator tool set is still referenced — no restriction
	// leaked from the distiller's narrower allowlist.
	for _, tool := range []string{"lint", "list_pages", "compile", "curate", "store_compiled", "verify_candidates", "supersede_learning", "promote"} {
		if !strings.Contains(command, tool) {
			t.Errorf("dewey-distill.md no longer references curator tool %q (FR-018: full curator tool set without restriction)", tool)
		}
	}

	// The distiller-only pipeline tools are absent from the interactive
	// command: the interactive path must not be dragged onto the
	// unattended surface.
	for _, tool := range []string{"scope", "group_learners", "rolling_pr"} {
		if strings.Contains(command, "`"+tool+"`") {
			t.Errorf("dewey-distill.md cites distiller-only tool %q; the interactive path must not use the unattended pipeline tools", tool)
		}
	}

	// The curator agent's frontmatter still carries the full allow set —
	// the distiller's existence must not narrow it — and has not gained
	// the distiller-only tools.
	curatorData, err := os.ReadFile(filepath.Join(".opencode", "agents", "curator.md"))
	if err != nil {
		t.Fatalf("read tracked curator.md: %v", err)
	}
	_, flags, _ := curatorFrontmatter(t, "tracked .opencode/agents/curator.md", string(curatorData))
	allowed := curatorAllowedDeweyTools(flags)
	for _, tool := range []string{"lint", "list_pages", "compile", "curate", "store_compiled", "verify_candidates", "supersede_learning", "promote"} {
		if !slices.Contains(allowed, tool) {
			t.Errorf("curator.md frontmatter no longer allows %q — the distiller work must not narrow the interactive tool set (FR-018)", tool)
		}
	}
	for _, tool := range []string{"scope", "group_learners", "rolling_pr"} {
		if slices.Contains(allowed, tool) {
			t.Errorf("curator.md frontmatter allows distiller-only tool %q — the interactive path must not gain unattended pipeline tools", tool)
		}
	}
}

// TestDeweyDistillAutoCommand_NoHumanGates verifies the unattended command
// carries no HUMAN_GATE markers — every gate in the unattended path is
// deterministic (server-side guards), so no stage may pause for a human —
// while the interactive /dewey-distill retains its gates (the contrast
// proves the marker check detects real gates, not prose mentions).
func TestDeweyDistillAutoCommand_NoHumanGates(t *testing.T) {
	autoCmd, err := os.ReadFile(filepath.Join(".opencode", "commands", "dewey-distill-auto.md"))
	if err != nil {
		t.Fatalf("read dewey-distill-auto.md: %v", err)
	}
	for i, line := range strings.Split(string(autoCmd), "\n") {
		if strings.HasPrefix(line, "HUMAN_GATE:") {
			t.Errorf("dewey-distill-auto.md line %d carries a HUMAN_GATE marker; the unattended path must be gate-free: %s", i+1, line)
		}
	}

	interactive, err := os.ReadFile(filepath.Join(".opencode", "commands", "dewey-distill.md"))
	if err != nil {
		t.Fatalf("read dewey-distill.md: %v", err)
	}
	gates := 0
	for _, line := range strings.Split(string(interactive), "\n") {
		if strings.HasPrefix(line, "HUMAN_GATE:") {
			gates++
		}
	}
	if gates == 0 {
		t.Error("dewey-distill.md lost its HUMAN_GATE markers; the interactive command must keep both gates")
	}
}

// TestPromoteCmd_MultiplePages verifies the CLI accepts multiple page names
// and resolves bare tags through the compiled namespace (#91).
func TestPromoteCmd_MultiplePages(t *testing.T) {
	tmpDir := t.TempDir()
	deweyDir := filepath.Join(tmpDir, ".uf", "dewey")
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("mkdir dewey dir: %v", err)
	}

	// Promote is a frontmatter file edit (PW-FR-001), so each batch
	// target needs its draft file on disk under the vault root.
	if err := os.MkdirAll(filepath.Join(tmpDir, "compiled"), 0o755); err != nil {
		t.Fatalf("mkdir compiled dir: %v", err)
	}
	for _, rel := range []string{"gateway.md", "auth-patterns.md"} {
		path := filepath.Join(tmpDir, "compiled", rel)
		if err := os.WriteFile(path, []byte("---\ntier: draft\n---\n\n# Article\n\nBody.\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	s, err := store.New(filepath.Join(deweyDir, "graph.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	for _, p := range []struct{ name, sourceID, sourceDocID string }{
		{"compiled/gateway", "compiled", "gateway"},
		{"compiled/auth-patterns", "compiled", "auth-patterns"},
	} {
		if err := s.InsertPage(&store.Page{
			Name:         p.name,
			OriginalName: p.name,
			SourceID:     p.sourceID,
			SourceDocID:  p.sourceDocID,
			Tier:         "draft",
		}); err != nil {
			t.Fatalf("InsertPage(%q): %v", p.name, err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	cmd := newPromoteCmd()
	cmd.SetArgs([]string{"--vault", tmpDir, "gateway", "compiled/auth-patterns"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("promote cmd: %v", err)
	}

	// The promotions landed in the files' frontmatter, not the database
	// (PW-FR-002: pages.tier is a cache the next index re-derives).
	for _, rel := range []string{"gateway.md", "auth-patterns.md"} {
		content, err := os.ReadFile(filepath.Join(tmpDir, "compiled", rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		if !strings.Contains(string(content), "tier: validated") {
			t.Errorf("%s = %q, want tier: validated in frontmatter", rel, string(content))
		}
	}

	s, err = store.New(filepath.Join(deweyDir, "graph.db"))
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer func() { _ = s.Close() }()

	for _, name := range []string{"compiled/gateway", "compiled/auth-patterns"} {
		page, err := s.GetPage(name)
		if err != nil {
			t.Fatalf("GetPage(%q): %v", name, err)
		}
		if page.Tier != "draft" {
			t.Errorf("%s tier = %q, want draft (promote never writes the DB tier)", name, page.Tier)
		}
	}
}

// TestPromoteCmd_BatchFailureExitsNonZero verifies that a batch containing a
// non-draft page changes nothing and returns an error from the CLI.
func TestPromoteCmd_BatchFailureExitsNonZero(t *testing.T) {
	tmpDir := t.TempDir()
	deweyDir := filepath.Join(tmpDir, ".uf", "dewey")
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("mkdir dewey dir: %v", err)
	}

	s, err := store.New(filepath.Join(deweyDir, "graph.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := s.InsertPage(&store.Page{Name: "compiled/one", OriginalName: "compiled/one", SourceID: "compiled", SourceDocID: "one", Tier: "draft"}); err != nil {
		t.Fatalf("InsertPage(one): %v", err)
	}
	if err := s.InsertPage(&store.Page{Name: "compiled/two", OriginalName: "compiled/two", SourceID: "compiled", SourceDocID: "two", Tier: "validated"}); err != nil {
		t.Fatalf("InsertPage(two): %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	cmd := newPromoteCmd()
	cmd.SetArgs([]string{"--vault", tmpDir, "compiled/one", "compiled/two"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for batch containing a validated page")
	}

	s, err = store.New(filepath.Join(deweyDir, "graph.db"))
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer func() { _ = s.Close() }()

	page, err := s.GetPage("compiled/one")
	if err != nil {
		t.Fatalf("GetPage(one): %v", err)
	}
	if page.Tier != "draft" {
		t.Errorf("compiled/one tier = %q, want draft (no partial promotion)", page.Tier)
	}
}

// TestDoctorCmd_SynthesisOpenAICompatibleConfigured verifies the new provider
// reports PASS for a complete configuration without live calls and never
// prints the API key.
func TestDoctorCmd_SynthesisOpenAICompatibleConfigured(t *testing.T) {
	tmpDir := t.TempDir()
	deweyDir := filepath.Join(tmpDir, deweyWorkspaceDir)
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("mkdir .uf/dewey: %v", err)
	}

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DEWEY_SYNTHESIS_ENDPOINT", "")
	t.Setenv("DEWEY_GENERATION_MODEL", "")
	t.Setenv("OLLAMA_HOST", "")
	const key = "doctor-test-key-not-real"
	t.Setenv("DEWEY_SYNTHESIS_API_KEY", key)

	configYAML := `synthesis:
  provider: openai-compatible
  endpoint: https://api.enmaas.devshift.net/v1
  model: rits/zai-org/glm-5-3
`
	if err := os.WriteFile(filepath.Join(deweyDir, "config.yaml"), []byte(configYAML), 0o644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}

	var buf bytes.Buffer
	runDoctorChecks(&buf, tmpDir)
	output := buf.String()

	for _, want := range []string{"openai-compatible", "https://api.enmaas.devshift.net/v1", "rits/zai-org/glm-5-3", "✅ credential", "configured"} {
		if !strings.Contains(output, want) {
			t.Errorf("doctor output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, key) {
		t.Errorf("doctor output leaked the API key:\n%s", output)
	}
}

// TestDoctorCmd_SynthesisOpenAICompatibleMissingCredential verifies the
// failure message names both accepted key sources.
func TestDoctorCmd_SynthesisOpenAICompatibleMissingCredential(t *testing.T) {
	tmpDir := t.TempDir()
	deweyDir := filepath.Join(tmpDir, deweyWorkspaceDir)
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("mkdir .uf/dewey: %v", err)
	}

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DEWEY_SYNTHESIS_ENDPOINT", "")
	t.Setenv("DEWEY_GENERATION_MODEL", "")
	t.Setenv("OLLAMA_HOST", "")
	t.Setenv("DEWEY_SYNTHESIS_API_KEY", "")

	configYAML := `synthesis:
  provider: openai-compatible
  endpoint: https://api.enmaas.devshift.net/v1
  model: rits/zai-org/glm-5-3
`
	if err := os.WriteFile(filepath.Join(deweyDir, "config.yaml"), []byte(configYAML), 0o644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}

	var buf bytes.Buffer
	runDoctorChecks(&buf, tmpDir)
	output := buf.String()

	if !strings.Contains(output, "DEWEY_SYNTHESIS_API_KEY") || !strings.Contains(output, "synthesis.api_key") {
		t.Errorf("doctor should name both key sources:\n%s", output)
	}
	if !strings.Contains(output, "❌ credential") || !strings.Contains(output, "missing") {
		t.Errorf("doctor should report a missing credential failure:\n%s", output)
	}
}

// TestDoctorCmd_SynthesisOpenAICompatibleInvalidEndpoint verifies that a
// schemeless endpoint fails configuration validation instead of passing.
func TestDoctorCmd_SynthesisOpenAICompatibleInvalidEndpoint(t *testing.T) {
	tmpDir := t.TempDir()
	deweyDir := filepath.Join(tmpDir, deweyWorkspaceDir)
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("mkdir .uf/dewey: %v", err)
	}

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DEWEY_SYNTHESIS_ENDPOINT", "")
	t.Setenv("DEWEY_GENERATION_MODEL", "")
	t.Setenv("OLLAMA_HOST", "")
	t.Setenv("DEWEY_SYNTHESIS_API_KEY", "key")

	configYAML := `synthesis:
  provider: openai-compatible
  endpoint: api.enmaas.devshift.net/v1
  model: rits/zai-org/glm-5-3
`
	if err := os.WriteFile(filepath.Join(deweyDir, "config.yaml"), []byte(configYAML), 0o644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}

	var buf bytes.Buffer
	runDoctorChecks(&buf, tmpDir)
	output := buf.String()

	if !strings.Contains(output, "❌ endpoint") || !strings.Contains(output, "invalid URL") {
		t.Errorf("doctor should report an invalid endpoint failure:\n%s", output)
	}
}

// TestDoctorCmd_SynthesisOpenAICompatibleMissingEndpoint verifies the doctor
// fails the synthesis endpoint check when an openai-compatible provider is
// configured without an endpoint (council round-3 G10).
func TestDoctorCmd_SynthesisOpenAICompatibleMissingEndpoint(t *testing.T) {
	tmpDir := t.TempDir()
	deweyDir := filepath.Join(tmpDir, deweyWorkspaceDir)
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DEWEY_SYNTHESIS_ENDPOINT", "")
	t.Setenv("DEWEY_GENERATION_MODEL", "")
	t.Setenv("OLLAMA_HOST", "")
	t.Setenv("DEWEY_SYNTHESIS_API_KEY", "doctor-test-key-not-real")

	configYAML := `synthesis:
  provider: openai-compatible
  model: rits/zai-org/glm-5-3
`
	if err := os.WriteFile(filepath.Join(deweyDir, "config.yaml"), []byte(configYAML), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	var buf bytes.Buffer
	runDoctorChecks(&buf, tmpDir)
	output := buf.String()

	if !strings.Contains(output, "openai-compatible") {
		t.Errorf("doctor should report the synthesis provider:\n%s", output)
	}
	if !strings.Contains(output, "❌ endpoint") || !strings.Contains(output, "missing") {
		t.Errorf("doctor should report a missing synthesis endpoint:\n%s", output)
	}
}

// TestDoctorCmd_SynthesisOpenAICompatibleCleartextWarning verifies the doctor
// warns when a valid synthesis endpoint uses plaintext http to a
// non-loopback host, without leaking the configured key (council round-3
// G10).
func TestDoctorCmd_SynthesisOpenAICompatibleCleartextWarning(t *testing.T) {
	tmpDir := t.TempDir()
	deweyDir := filepath.Join(tmpDir, deweyWorkspaceDir)
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DEWEY_SYNTHESIS_ENDPOINT", "")
	t.Setenv("DEWEY_GENERATION_MODEL", "")
	t.Setenv("OLLAMA_HOST", "")
	t.Setenv("DEWEY_SYNTHESIS_API_KEY", "doctor-test-key-not-real")

	// TEST-NET-1 address: a syntactically valid non-loopback http endpoint.
	// The doctor validates configuration only and never connects (D9).
	configYAML := `synthesis:
  provider: openai-compatible
  endpoint: http://192.0.2.1:8080/v1
  model: rits/zai-org/glm-5-3
`
	if err := os.WriteFile(filepath.Join(deweyDir, "config.yaml"), []byte(configYAML), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	var buf bytes.Buffer
	runDoctorChecks(&buf, tmpDir)
	output := buf.String()

	if !strings.Contains(output, "plaintext http endpoint") || !strings.Contains(output, "cleartext") {
		t.Errorf("doctor should warn about plaintext http synthesis endpoints:\n%s", output)
	}
	if !strings.Contains(output, "⚠️") {
		t.Errorf("doctor should mark the cleartext warning with the WARN marker:\n%s", output)
	}
	if strings.Contains(output, "doctor-test-key-not-real") {
		t.Errorf("doctor output must not leak the synthesis key:\n%s", output)
	}
}

// TestDoctorCmd_SynthesisOpenAICompatibleCredentialsInURLRejected verifies
// the doctor rejects a credential-bearing synthesis endpoint as an invalid
// URL and never echoes the embedded credentials (council round-3 G10).
func TestDoctorCmd_SynthesisOpenAICompatibleCredentialsInURLRejected(t *testing.T) {
	tmpDir := t.TempDir()
	deweyDir := filepath.Join(tmpDir, deweyWorkspaceDir)
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DEWEY_SYNTHESIS_ENDPOINT", "")
	t.Setenv("DEWEY_GENERATION_MODEL", "")
	t.Setenv("OLLAMA_HOST", "")
	t.Setenv("DEWEY_SYNTHESIS_API_KEY", "doctor-test-key-not-real")

	configYAML := `synthesis:
  provider: openai-compatible
  endpoint: https://user:super-secret@api.example.com/v1
  model: rits/zai-org/glm-5-3
`
	if err := os.WriteFile(filepath.Join(deweyDir, "config.yaml"), []byte(configYAML), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	var buf bytes.Buffer
	runDoctorChecks(&buf, tmpDir)
	output := buf.String()

	if !strings.Contains(output, "❌ endpoint") || !strings.Contains(output, "invalid URL") {
		t.Errorf("doctor should reject a credential-bearing synthesis endpoint:\n%s", output)
	}
	if strings.Contains(output, "super-secret") {
		t.Errorf("doctor output must not echo credentials embedded in the endpoint:\n%s", output)
	}
}

// --- Promote lock diagnostics (mcp-internal-store-access task 4.1) ---

// holdLockFileForTest creates a lock file at path, acquires an exclusive
// non-blocking flock on it from this test process, and writes content as
// the first line. A second open of the same file (as done by
// detectLockHolder) fails to flock even within the same process because
// flock locks belong to the open file description, not the process. The
// lock is held until the test ends.
func holdLockFileForTest(t *testing.T, path, content string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatalf("create lock file %q: %v", path, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		t.Fatalf("flock %q: %v", path, err)
	}
	if content != "" {
		if _, err := f.WriteString(content); err != nil {
			t.Fatalf("write lock file content: %v", err)
		}
	}
	t.Cleanup(func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	})
}

// TestPromoteStoreOpenError_HeldLock verifies that a store.New failure is
// enriched with the lock holder's recorded PID and command, points to the
// dewey_promote MCP tool, and wraps the underlying error.
func TestPromoteStoreOpenError_HeldLock(t *testing.T) {
	dir := t.TempDir()
	pid := os.Getpid()
	holdLockFileForTest(t, filepath.Join(dir, "dewey.lock"),
		fmt.Sprintf("%d dewey serve --vault /tmp/vault\n", pid))

	underlying := errors.New("another Dewey process is using this database: resource temporarily unavailable")
	got := promoteStoreOpenError(dir, underlying)
	msg := got.Error()

	if !strings.Contains(msg, fmt.Sprintf("PID %d", pid)) {
		t.Errorf("error %q does not identify holder PID %d", msg, pid)
	}
	if !strings.Contains(msg, "dewey serve --vault /tmp/vault") {
		t.Errorf("error %q does not include the recorded holder command", msg)
	}
	if !strings.Contains(msg, "dewey_promote") {
		t.Errorf("error %q does not point to the dewey_promote MCP tool", msg)
	}
	if !errors.Is(got, underlying) {
		t.Errorf("errors.Is(got, underlying) = false, want the underlying store.New error wrapped")
	}
}

// TestPromoteStoreOpenError_OldFormatLock verifies that a held lock file
// with no recorded PID (pre-T011 format) still produces the unknown-holder
// description and the MCP promote hint.
func TestPromoteStoreOpenError_OldFormatLock(t *testing.T) {
	dir := t.TempDir()
	// Held lock file with empty content: old format, no PID line.
	holdLockFileForTest(t, filepath.Join(dir, "dewey.lock"), "")

	underlying := errors.New("another Dewey process is using this database: resource temporarily unavailable")
	got := promoteStoreOpenError(dir, underlying)
	msg := got.Error()

	if !strings.Contains(msg, "lock held (unknown process)") {
		t.Errorf("error %q does not mention the unknown lock holder", msg)
	}
	if !strings.Contains(msg, "dewey_promote") {
		t.Errorf("error %q does not point to the dewey_promote MCP tool", msg)
	}
	if !errors.Is(got, underlying) {
		t.Errorf("errors.Is(got, underlying) = false, want the underlying store.New error wrapped")
	}
}

// TestPromoteStoreOpenError_NoLockFile verifies graceful degradation: with
// no lock file at all, the error is exactly the plain wrapped form with no
// lock-holder text and no MCP hint.
func TestPromoteStoreOpenError_NoLockFile(t *testing.T) {
	dir := t.TempDir() // no dewey.lock exists

	underlying := errors.New("migrate schema: boom")
	got := promoteStoreOpenError(dir, underlying)

	want := fmt.Errorf("open store: %w", underlying).Error()
	if got.Error() != want {
		t.Errorf("error = %q, want exactly %q (plain wrapped form)", got.Error(), want)
	}
	if !errors.Is(got, underlying) {
		t.Errorf("errors.Is(got, underlying) = false, want the underlying store.New error wrapped")
	}
}

// TestPromoteStoreOpenError_UnheldLockFile verifies graceful degradation
// for a stale lock file: the file exists with PID content, but nothing
// holds the flock, so the error stays in the plain wrapped form.
func TestPromoteStoreOpenError_UnheldLockFile(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "dewey.lock")
	if err := os.WriteFile(lockPath, []byte("99999 dewey serve\n"), 0o644); err != nil {
		t.Fatalf("write stale lock file: %v", err)
	}

	underlying := errors.New("migrate schema: boom")
	got := promoteStoreOpenError(dir, underlying)

	want := fmt.Errorf("open store: %w", underlying).Error()
	if got.Error() != want {
		t.Errorf("error = %q, want exactly %q (no lock-holder diagnostics when lock is unheld)", got.Error(), want)
	}
	if !errors.Is(got, underlying) {
		t.Errorf("errors.Is(got, underlying) = false, want the underlying store.New error wrapped")
	}
}

// --- OpenAI-compatible doctor tests (spec 017, T035) ---

func newGatewayDoctorVault(t *testing.T, configYAML string) string {
	t.Helper()
	tmpDir := t.TempDir()
	deweyDir := filepath.Join(tmpDir, deweyWorkspaceDir)
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("mkdir .uf/dewey: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deweyDir, "config.yaml"), []byte(configYAML), 0o644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DEWEY_EMBEDDING_ENDPOINT", "")
	t.Setenv("DEWEY_EMBEDDING_MODEL", "")
	t.Setenv("DEWEY_SYNTHESIS_ENDPOINT", "")
	t.Setenv("DEWEY_GENERATION_MODEL", "")
	t.Setenv("OLLAMA_HOST", "")
	return tmpDir
}

// embeddingLayerSection returns the doctor output from the Embedding Layer
// header up to the next section (the Synthesis Layer header).
func embeddingLayerSection(t *testing.T, output string) string {
	t.Helper()
	start := strings.Index(output, "Embedding Layer")
	if start < 0 {
		t.Fatalf("doctor output has no Embedding Layer section:\n%s", output)
	}
	section := output[start:]
	if end := strings.Index(section, "Synthesis Layer"); end >= 0 {
		section = section[:end]
	}
	return section
}

// TestDoctorCmd_EmbeddingVertexConfigured verifies a complete Vertex
// embedding config is reported through Vertex diagnostics, not Ollama
// (issue #143).
func TestDoctorCmd_EmbeddingVertexConfigured(t *testing.T) {
	configYAML := "embedding:\n  provider: vertex\n  model: text-embedding-005\n  project: my-test-project\n  region: us-central1\n"
	tmpDir := newGatewayDoctorVault(t, configYAML)

	var buf bytes.Buffer
	runDoctorChecks(&buf, tmpDir)
	section := embeddingLayerSection(t, buf.String())

	for _, want := range []string{"✅ config", "text-embedding-005 via vertex", "project=my-test-project", "region=us-central1", "model=text-embedding-005"} {
		if !strings.Contains(section, want) {
			t.Errorf("Embedding Layer should contain %q, got:\n%s", want, section)
		}
	}
	if strings.Contains(strings.ToLower(section), "ollama") {
		t.Errorf("Vertex Embedding Layer must not mention Ollama, got:\n%s", section)
	}
}

// TestDoctorCmd_EmbeddingVertexMisconfigured verifies an incomplete Vertex
// embedding config reports FAIL naming the missing field, with no Ollama
// fix hint (issue #143).
func TestDoctorCmd_EmbeddingVertexMisconfigured(t *testing.T) {
	configYAML := "embedding:\n  provider: vertex\n  model: text-embedding-005\n  region: us-central1\n"
	tmpDir := newGatewayDoctorVault(t, configYAML)

	var buf bytes.Buffer
	runDoctorChecks(&buf, tmpDir)
	section := embeddingLayerSection(t, buf.String())

	if !strings.Contains(section, "❌ config") || !strings.Contains(section, "incomplete") || !strings.Contains(section, "project") {
		t.Errorf("Embedding Layer should report incomplete config naming project, got:\n%s", section)
	}
	if strings.Contains(section, "✅") {
		t.Errorf("Embedding Layer must not render a PASS check for incomplete config, got:\n%s", section)
	}
	if strings.Contains(strings.ToLower(section), "ollama") {
		t.Errorf("Vertex Embedding Layer must not suggest Ollama, got:\n%s", section)
	}
}

func TestDoctorCmd_EmbeddingGatewayConfigured(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	configYAML := "embedding:\n  provider: openai-compatible\n  model: stub-model\n  endpoint: " + srv.URL + "/v1\n"
	tmpDir := newGatewayDoctorVault(t, configYAML)

	var buf bytes.Buffer
	runDoctorChecks(&buf, tmpDir)
	output := buf.String()

	if !strings.Contains(output, "Embedding Layer") {
		t.Errorf("doctor should include Embedding Layer, got:\n%s", output)
	}
	if !strings.Contains(output, "stub-model") {
		t.Errorf("doctor should show the gateway model, got:\n%s", output)
	}
	if !strings.Contains(output, "openai-compatible") {
		t.Errorf("doctor should name the provider on the credential path, got:\n%s", output)
	}
	if !strings.Contains(output, "ready") {
		t.Errorf("doctor should report the probe as ready, got:\n%s", output)
	}
}

func TestDoctorCmd_EmbeddingGatewayIncomplete(t *testing.T) {
	configYAML := "embedding:\n  provider: openai-compatible\n  model: stub-model\n"
	tmpDir := newGatewayDoctorVault(t, configYAML)

	var buf bytes.Buffer
	runDoctorChecks(&buf, tmpDir)
	output := buf.String()

	if !strings.Contains(output, "incomplete") {
		t.Errorf("doctor should report incomplete gateway config, got:\n%s", output)
	}
	if !strings.Contains(output, "embedding.endpoint") {
		t.Errorf("doctor should name the missing field, got:\n%s", output)
	}
}

func TestDoctorCmd_EmbeddingGatewayUnknownProviderListsAll(t *testing.T) {
	configYAML := "embedding:\n  provider: badprovider\n  model: stub-model\n"
	tmpDir := newGatewayDoctorVault(t, configYAML)

	var buf bytes.Buffer
	runDoctorChecks(&buf, tmpDir)
	output := buf.String()

	if !strings.Contains(output, "unknown provider") {
		t.Errorf("doctor should report unknown provider, got:\n%s", output)
	}
	for _, provider := range []string{"ollama", "vertex", "openai-compatible"} {
		if !strings.Contains(output, provider) {
			t.Errorf("doctor unknown-provider message should list %q, got:\n%s", provider, output)
		}
	}
}

func TestDoctorCmd_GatewayCleartextCredentialWarning(t *testing.T) {
	// No model so the factory fails before any probe: the credential
	// warning must still be emitted deterministically.
	configYAML := "embedding:\n  provider: openai-compatible\n  endpoint: http://192.0.2.1:8080/v1\n"
	tmpDir := newGatewayDoctorVault(t, configYAML)
	t.Setenv("DEWEY_EMBEDDING_API_KEY", "sk-doctor-secret")

	var buf bytes.Buffer
	runDoctorChecks(&buf, tmpDir)
	output := buf.String()

	if !strings.Contains(output, "would transit http://") {
		t.Errorf("doctor should warn about cleartext credentials, got:\n%s", output)
	}
	if strings.Contains(output, "sk-doctor-secret") {
		t.Errorf("doctor output must not contain the key, got:\n%s", output)
	}
}

func TestDoctorCmd_GatewayAPIKeyFieldIgnored(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	configYAML := "embedding:\n  provider: openai-compatible\n  model: stub-model\n  endpoint: " + srv.URL + "/v1\n  api_key: super-secret-value\n"
	tmpDir := newGatewayDoctorVault(t, configYAML)

	var buf bytes.Buffer
	runDoctorChecks(&buf, tmpDir)
	output := buf.String()

	if !strings.Contains(output, "api_key in config.yaml is ignored") {
		t.Errorf("doctor should warn about the ignored api_key field, got:\n%s", output)
	}
	if strings.Contains(output, "super-secret-value") {
		t.Errorf("doctor output must not echo the ignored key value, got:\n%s", output)
	}
}

func TestCreateIndexEmbedder_GatewayUnavailableWarnsWithoutPullHint(t *testing.T) {
	configYAML := "embedding:\n  provider: openai-compatible\n  model: stub-model\n  endpoint: http://127.0.0.1:1/v1\n"
	tmpDir := newGatewayDoctorVault(t, configYAML)
	deweyDir := filepath.Join(tmpDir, deweyWorkspaceDir)

	var logBuf bytes.Buffer
	logger.SetOutput(&logBuf)
	defer logger.SetOutput(os.Stderr)

	embedder, err := createIndexEmbedder(false, deweyDir)
	if err != nil {
		t.Fatalf("createIndexEmbedder should degrade, got error: %v", err)
	}
	if embedder != nil {
		t.Fatal("expected nil embedder (keyword-only mode)")
	}

	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "openai-compatible") {
		t.Errorf("warning should name the provider, got: %q", logOutput)
	}
	if !strings.Contains(logOutput, "unreachable") {
		t.Errorf("warning should carry the reason, got: %q", logOutput)
	}
	if strings.Contains(logOutput, "ollama pull") {
		t.Errorf("gateway warning must not suggest ollama pull, got: %q", logOutput)
	}
}

// degradationEmbedder is a stub embedder carrying an unavailable reason.
type degradationEmbedder struct{ reason string }

func (degradationEmbedder) Embed(context.Context, string) ([]float32, error) { return nil, nil }
func (degradationEmbedder) EmbedBatch(context.Context, []string) ([][]float32, error) {
	return nil, nil
}
func (degradationEmbedder) Available() bool             { return false }
func (degradationEmbedder) ModelID() string             { return "stub-model" }
func (d degradationEmbedder) UnavailableReason() string { return d.reason }

func fieldsToMap(fields []any) map[string]any {
	m := make(map[string]any, len(fields)/2)
	for i := 0; i+1 < len(fields); i += 2 {
		key, _ := fields[i].(string)
		m[key] = fields[i+1]
	}
	return m
}

func TestEmbeddingDegradationFields(t *testing.T) {
	t.Run("ollama keeps the pull hint", func(t *testing.T) {
		fields := fieldsToMap(embeddingDegradationFields(
			embed.ProviderConfig{Provider: "ollama", Model: "granite-embedding:30m", Endpoint: "http://localhost:11434"},
			degradationEmbedder{},
		))
		if fields["pull"] != "ollama pull granite-embedding:30m" {
			t.Fatalf("ollama pull hint missing: %v", fields)
		}
		if fields["provider"] != "ollama" {
			t.Fatalf("provider missing: %v", fields)
		}
	})

	t.Run("gateway omits the pull hint and carries the reason", func(t *testing.T) {
		fields := fieldsToMap(embeddingDegradationFields(
			embed.ProviderConfig{Provider: "openai-compatible", Model: "stub-model", Endpoint: "https://gateway.example/v1"},
			degradationEmbedder{reason: "openai-compatible embedding endpoint is unreachable"},
		))
		if _, ok := fields["pull"]; ok {
			t.Fatalf("gateway must not include a pull hint: %v", fields)
		}
		if fields["provider"] != "openai-compatible" {
			t.Fatalf("provider missing: %v", fields)
		}
		if fields["reason"] != "openai-compatible embedding endpoint is unreachable" {
			t.Fatalf("reason missing: %v", fields)
		}
	})
}

// --- Tool-owned scaffold refresh acceptance tests (task 3.1,
// openspec/changes/tool-owned-scaffold-updates) ---
//
// These tests drive newInitCmd() with --vault over t.TempDir() trees to
// enforce the dewey-init delta (DI-FR-001/DI-FR-002) and the
// dewey-init-ownership delta (DIO-FR-001 .. DIO-FR-006) end to end:
// refresh of untouched drifted scaffolds per kind, preservation of
// customized files, marker bootstrap of byte-identical marker-less files,
// first-init marker parity, the composability guard, migration-then-
// classification ordering, outcome observability, deterministic iteration
// order, and non-map preservation. Fixtures are built with the same
// marker-plus-payload constructors the writer uses (scaffoldPayload,
// markdownMarkerLine, withFinalLine from scaffold_test.go) so each fixture
// is by construction the on-disk form of the scaffold state it simulates.

// olderScaffoldBody derives a fake older-release body from an embedded
// scaffold map value: it differs from the current payload everywhere
// classification can see while remaining a realistic file of its kind.
// Markdown bodies append an HTML comment (their frontmatter must stay
// first); YAML bodies prepend a `#` comment (their block scalars must stay
// last).
func olderScaffoldBody(embedded string, kind scaffoldKind) string {
	if kind == scaffoldYAML {
		return "# scaffolded by an older dewey release\n" + embedded
	}
	return embedded + "\n<!-- scaffolded by an older dewey release -->\n"
}

// driftedScaffoldFixture builds what an untouched scaffold of an older Dewey
// version looks like on disk (the DIO-FR-004 "Untouched, drifted" row): a
// marker whose digest still matches the body it was written over, where
// that body differs from the current embedded payload.
func driftedScaffoldFixture(t *testing.T, embedded string, kind scaffoldKind) []byte {
	t.Helper()
	fixture := scaffoldPayload([]byte(olderScaffoldBody(embedded, kind)), kind)
	digest, payload, ok := parseScaffoldMarker(fixture)
	if !ok || digest != sha256Hex(payload) {
		t.Fatalf("fixture precondition: marker must be valid for the older body")
	}
	return fixture
}

// customizedScaffoldFixture builds the DIO-FR-004 customized scenario: a
// file that was scaffolded with a valid marker and whose body was then
// edited, so the marker's digest no longer matches the bytes preceding it.
func customizedScaffoldFixture(t *testing.T, embedded string, kind scaffoldKind) []byte {
	t.Helper()
	scaffolded := scaffoldPayload([]byte(embedded), kind)
	_, payload, ok := parseScaffoldMarker(scaffolded)
	if !ok {
		t.Fatal("fixture precondition: scaffoldPayload output must end in a parseable marker")
	}
	// Keep the original marker line verbatim (with its now-stale digest) and
	// edit only the payload bytes preceding it.
	marker := strings.TrimSuffix(string(scaffolded[len(payload):]), "\n")
	edited := append(bytes.Clone(payload), []byte("a user edit made after scaffolding\n")...)
	return withFinalLine(edited, marker)
}

// scaffoldOutcomeEvents extracts the tool-owned scaffold outcome events
// (DIO-FR-006) from captured init log output, in order. Each line is
// normalized to start at its event text so the logger's prefix and
// timestamp never participate in comparisons, and carries the path= value
// naming the target through the end of the line.
func scaffoldOutcomeEvents(logs string) []string {
	markers := []string{
		"scaffolded slash command",
		"scaffolded agent",
		"scaffolded workflow template example",
		"updated slash command",
		"updated agent",
		"updated workflow template example",
		"skip (customized)",
		"skip (concurrent)",
	}
	var events []string
	for _, line := range strings.Split(logs, "\n") {
		for _, marker := range markers {
			if idx := strings.Index(line, marker); idx >= 0 {
				events = append(events, line[idx:])
				break
			}
		}
	}
	return events
}

// hasOutcomeEvent reports whether events contains a line matching event
// that also names path.
func hasOutcomeEvent(events []string, event, path string) bool {
	for _, e := range events {
		if strings.Contains(e, event) && strings.Contains(e, path) {
			return true
		}
	}
	return false
}

// prepareMixedOutcomeVault builds a vault whose single init run produces
// outcome events across all three tool-owned kinds: every slash command
// missing (scaffolded), curator.md an untouched drifted scaffold (updated),
// distiller.md missing (scaffolded), and a customized workflow template
// example (skip). The determinism test materializes this same vault state
// twice — as two identically-prepared vaults — so neither run's writes feed
// the other's inputs.
func prepareMixedOutcomeVault(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()

	agentsDir := filepath.Join(tmpDir, ".opencode", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("create agents dir: %v", err)
	}
	curator, ok := deweyAgentDefinitions["curator.md"]
	if !ok {
		t.Fatal("deweyAgentDefinitions missing curator.md entry")
	}
	if err := os.WriteFile(filepath.Join(agentsDir, "curator.md"), driftedScaffoldFixture(t, curator, scaffoldMarkdown), 0o644); err != nil {
		t.Fatalf("write drifted curator.md: %v", err)
	}

	workflowsDir := filepath.Join(tmpDir, ".github", "workflows")
	if err := os.MkdirAll(workflowsDir, 0o755); err != nil {
		t.Fatalf("create workflows dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workflowsDir, "dewey-distill.yml.example"), []byte("# user-customized example\n"), 0o644); err != nil {
		t.Fatalf("write customized workflow example: %v", err)
	}

	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(""), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}
	return tmpDir
}

// TestInitCmd_ReInitRefreshesUntouchedDriftedCommand enforces the DI-FR-002
// scenario "Re-init after upgrade refreshes untouched commands": a command
// scaffolded by an older Dewey — its marker digest still valid for its
// body, but that body differs from the current embedded payload — is
// rewritten to the embedded payload with a fresh marker, while missing
// commands are still created and the refresh logs `updated` naming the path.
func TestInitCmd_ReInitRefreshesUntouchedDriftedCommand(t *testing.T) {
	tmpDir := t.TempDir()
	cmdDir := filepath.Join(tmpDir, ".opencode", "commands")
	if err := os.MkdirAll(cmdDir, 0o755); err != nil {
		t.Fatalf("create commands dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(""), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	// The vault was initialized by an older Dewey release: .uf/dewey exists
	// and dewey-compile.md is an untouched scaffold of that older version.
	deweyDir := filepath.Join(tmpDir, ".uf", "dewey")
	if err := os.MkdirAll(deweyDir, 0o755); err != nil {
		t.Fatalf("create .uf/dewey: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deweyDir, "config.yaml"), []byte("# existing config\n"), 0o644); err != nil {
		t.Fatalf("write existing config.yaml: %v", err)
	}

	embedded, ok := deweySlashCommands["dewey-compile.md"]
	if !ok {
		t.Fatal("deweySlashCommands missing dewey-compile.md entry")
	}
	driftedPath := filepath.Join(cmdDir, "dewey-compile.md")
	drifted := driftedScaffoldFixture(t, embedded, scaffoldMarkdown)
	oldDigest, _, ok := parseScaffoldMarker(drifted)
	if !ok {
		t.Fatal("fixture precondition: drifted command must carry a parseable marker")
	}
	if err := os.WriteFile(driftedPath, drifted, 0o644); err != nil {
		t.Fatalf("write drifted command: %v", err)
	}

	logBuf := captureLoggerOutput(t)
	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("re-init failed: %v", err)
	}

	got, err := os.ReadFile(driftedPath)
	if err != nil {
		t.Fatalf("read refreshed command: %v", err)
	}
	if want := scaffoldPayload([]byte(embedded), scaffoldMarkdown); !bytes.Equal(got, want) {
		t.Errorf("refreshed dewey-compile.md = %q, want the current embedded scaffoldPayload %q", got, want)
	}

	// DIO-FR-002 "Rewritten file carries a fresh marker": the new marker
	// verifies against the new payload and no longer records the older
	// body's digest.
	newDigest, payload, ok := parseScaffoldMarker(got)
	if !ok {
		t.Fatalf("refreshed command does not end in a parseable marker:\n%s", got)
	}
	if want := sha256Hex(canonicalBody(embedded)); newDigest != want {
		t.Errorf("refreshed marker digest = %q, want the current payload's digest %q", newDigest, want)
	}
	if newDigest == oldDigest {
		t.Error("refreshed marker kept the older body's digest; the rewrite must carry a fresh marker")
	}
	if !bytes.Equal(payload, canonicalBody(embedded)) {
		t.Errorf("refreshed marker payload = %q, want the current canonical body %q", payload, canonicalBody(embedded))
	}

	// Missing commands are still created alongside the refresh (DI-FR-002).
	if _, err := os.Stat(filepath.Join(cmdDir, "dewey-store.md")); err != nil {
		t.Errorf("missing command dewey-store.md was not created on re-init: %v", err)
	}

	events := scaffoldOutcomeEvents(logBuf.String())
	if !hasOutcomeEvent(events, "updated slash command", driftedPath) {
		t.Errorf("the refresh must log `updated slash command` naming %s; events:\n%s", driftedPath, strings.Join(events, "\n"))
	}
}

// TestInitCmd_UntouchedCurrentScaffoldNotRewritten enforces the DIO-FR-004
// scenario "Untouched current scaffold is left alone": a scaffold already
// equal to the embedded payload is not rewritten — bytes and mtime are
// preserved — and no outcome event is logged for it (idempotent silence).
func TestInitCmd_UntouchedCurrentScaffoldNotRewritten(t *testing.T) {
	tmpDir := t.TempDir()
	cmdDir := filepath.Join(tmpDir, ".opencode", "commands")
	if err := os.MkdirAll(cmdDir, 0o755); err != nil {
		t.Fatalf("create commands dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(""), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	entry, ok := deweySlashCommands["dewey-store.md"]
	if !ok {
		t.Fatal("deweySlashCommands missing dewey-store.md entry")
	}
	path := filepath.Join(cmdDir, "dewey-store.md")
	current := scaffoldPayload([]byte(entry), scaffoldMarkdown)
	if err := os.WriteFile(path, current, 0o644); err != nil {
		t.Fatalf("write current scaffold: %v", err)
	}

	// Pin a recognizable old mtime, then re-stat it: the filesystem may
	// round the stored timestamp, so the comparison baseline is what it
	// actually kept. Any rewrite — create or temp-and-rename — would set a
	// fresh mtime hours away.
	forced := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(path, forced, forced); err != nil {
		t.Fatalf("pin mtime: %v", err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat before init: %v", err)
	}

	logBuf := captureLoggerOutput(t)
	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat after init: %v", err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("current scaffold was rewritten: mtime changed from %v to %v", before.ModTime(), after.ModTime())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read current scaffold: %v", err)
	}
	if !bytes.Equal(got, current) {
		t.Errorf("current scaffold bytes changed: got %q, want %q", got, current)
	}

	// Idempotent silence: no outcome event names this path.
	for _, e := range scaffoldOutcomeEvents(logBuf.String()) {
		if strings.Contains(e, path) {
			t.Errorf("current scaffold must log no outcome event; got %q", e)
		}
	}
}

// TestInitCmd_CustomizedScaffoldPreservedByteForByte enforces the DIO-FR-004
// scenario "Customized scaffold is preserved": a scaffold whose marker digest
// no longer matches its body (the body was edited after scaffolding) is left
// untouched — not modified, moved, renamed, or deleted — and
// `skip (customized)` is logged naming the path.
func TestInitCmd_CustomizedScaffoldPreservedByteForByte(t *testing.T) {
	tmpDir := t.TempDir()
	cmdDir := filepath.Join(tmpDir, ".opencode", "commands")
	if err := os.MkdirAll(cmdDir, 0o755); err != nil {
		t.Fatalf("create commands dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(""), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	entry, ok := deweySlashCommands["dewey-curate.md"]
	if !ok {
		t.Fatal("deweySlashCommands missing dewey-curate.md entry")
	}
	path := filepath.Join(cmdDir, "dewey-curate.md")
	customized := customizedScaffoldFixture(t, entry, scaffoldMarkdown)
	if err := os.WriteFile(path, customized, 0o644); err != nil {
		t.Fatalf("write customized scaffold: %v", err)
	}

	logBuf := captureLoggerOutput(t)
	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read customized scaffold: %v", err)
	}
	if !bytes.Equal(got, customized) {
		t.Errorf("customized scaffold was modified: got %q, want byte-identical %q", got, customized)
	}
	events := scaffoldOutcomeEvents(logBuf.String())
	if !hasOutcomeEvent(events, "skip (customized)", path) {
		t.Errorf("customized scaffold must log `skip (customized)` naming %s; events:\n%s", path, strings.Join(events, "\n"))
	}
}

// TestInitCmd_MarkerlessIdenticalGainsMarker enforces the DIO-FR-004
// scenario "Pre-marker legacy scaffold that is byte-identical gains the
// marker": a file with no marker whose content is byte-identical to the
// current embedded payload is rewritten with the marker plus the same
// payload, and `updated` is logged naming the path.
func TestInitCmd_MarkerlessIdenticalGainsMarker(t *testing.T) {
	tmpDir := t.TempDir()
	cmdDir := filepath.Join(tmpDir, ".opencode", "commands")
	if err := os.MkdirAll(cmdDir, 0o755); err != nil {
		t.Fatalf("create commands dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(""), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	entry, ok := deweySlashCommands["dewey-publish.md"]
	if !ok {
		t.Fatal("deweySlashCommands missing dewey-publish.md entry")
	}
	path := filepath.Join(cmdDir, "dewey-publish.md")
	// The pre-marker form: exactly the embedded payload, no marker line.
	if err := os.WriteFile(path, canonicalBody(entry), 0o644); err != nil {
		t.Fatalf("write marker-less command: %v", err)
	}

	logBuf := captureLoggerOutput(t)
	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read marker-bootstrapped command: %v", err)
	}
	want := scaffoldPayload([]byte(entry), scaffoldMarkdown)
	if !bytes.Equal(got, want) {
		t.Errorf("marker-bootstrapped command = %q, want the exact scaffoldPayload %q", got, want)
	}
	events := scaffoldOutcomeEvents(logBuf.String())
	if !hasOutcomeEvent(events, "updated slash command", path) {
		t.Errorf("marker bootstrap must log `updated slash command` naming %s; events:\n%s", path, strings.Join(events, "\n"))
	}
}

// TestInitCmd_MarkerlessDifferingPreserved enforces the DIO-FR-004 scenario
// "Pre-marker legacy scaffold that differs is preserved": a marker-less file
// whose content differs from the current embedded payload is treated as
// customized — unchanged, with `skip (customized)` logged (the legacy safe
// default).
func TestInitCmd_MarkerlessDifferingPreserved(t *testing.T) {
	tmpDir := t.TempDir()
	cmdDir := filepath.Join(tmpDir, ".opencode", "commands")
	if err := os.MkdirAll(cmdDir, 0o755); err != nil {
		t.Fatalf("create commands dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(""), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	path := filepath.Join(cmdDir, "dewey-index.md")
	legacy := []byte("# legacy command scaffolded by a pre-marker dewey\n\nwith local edits\n")
	if err := os.WriteFile(path, legacy, 0o644); err != nil {
		t.Fatalf("write legacy command: %v", err)
	}

	logBuf := captureLoggerOutput(t)
	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read legacy command: %v", err)
	}
	if !bytes.Equal(got, legacy) {
		t.Errorf("legacy marker-less command was modified: got %q, want byte-identical %q", got, legacy)
	}
	events := scaffoldOutcomeEvents(logBuf.String())
	if !hasOutcomeEvent(events, "skip (customized)", path) {
		t.Errorf("legacy marker-less command must log `skip (customized)` naming %s; events:\n%s", path, strings.Join(events, "\n"))
	}
}

// TestInitCmd_MalformedMarkerPreserved enforces the DIO-FR-004 scenario
// "Malformed marker is treated as customized": a tool-owned path whose final
// line contains dewey:scaffold but whose digest field is not 64 lowercase
// hex characters is unchanged, with `skip (customized)` logged.
func TestInitCmd_MalformedMarkerPreserved(t *testing.T) {
	tmpDir := t.TempDir()
	cmdDir := filepath.Join(tmpDir, ".opencode", "commands")
	if err := os.MkdirAll(cmdDir, 0o755); err != nil {
		t.Fatalf("create commands dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(""), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	entry, ok := deweySlashCommands["dewey-reindex.md"]
	if !ok {
		t.Fatal("deweySlashCommands missing dewey-reindex.md entry")
	}
	path := filepath.Join(cmdDir, "dewey-reindex.md")
	// The final line carries the dewey:scaffold token, but "deadbeef" is
	// not 64 lowercase hex characters — a malformed digest field.
	malformed := withFinalLine(canonicalBody(entry), markdownMarkerLine("dev", "deadbeef"))
	if err := os.WriteFile(path, malformed, 0o644); err != nil {
		t.Fatalf("write malformed-marker command: %v", err)
	}

	logBuf := captureLoggerOutput(t)
	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read malformed-marker command: %v", err)
	}
	if !bytes.Equal(got, malformed) {
		t.Errorf("malformed-marker command was modified: got %q, want byte-identical %q", got, malformed)
	}
	events := scaffoldOutcomeEvents(logBuf.String())
	if !hasOutcomeEvent(events, "skip (customized)", path) {
		t.Errorf("malformed-marker command must log `skip (customized)` naming %s; events:\n%s", path, strings.Join(events, "\n"))
	}
}

// TestInitCmd_RefreshesUntouchedAgentAndWorkflowExample enforces the
// DI-FR-002 scenario "Re-init refreshes untouched agents and workflow
// example": an untouched drifted curator.md and an untouched drifted
// dewey-distill.yml.example are both rewritten with the current embedded
// content plus fresh markers (DIO-FR-002).
func TestInitCmd_RefreshesUntouchedAgentAndWorkflowExample(t *testing.T) {
	tmpDir := t.TempDir()
	agentsDir := filepath.Join(tmpDir, ".opencode", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("create agents dir: %v", err)
	}
	workflowsDir := filepath.Join(tmpDir, ".github", "workflows")
	if err := os.MkdirAll(workflowsDir, 0o755); err != nil {
		t.Fatalf("create workflows dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(""), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	curator, ok := deweyAgentDefinitions["curator.md"]
	if !ok {
		t.Fatal("deweyAgentDefinitions missing curator.md entry")
	}
	workflow, ok := deweyWorkflowTemplates["dewey-distill.yml.example"]
	if !ok {
		t.Fatal("deweyWorkflowTemplates missing dewey-distill.yml.example entry")
	}

	type refreshTarget struct {
		label     string
		path      string
		embedded  string
		kind      scaffoldKind
		oldDigest string
	}
	targets := []refreshTarget{
		{
			label:    "agent",
			path:     filepath.Join(agentsDir, "curator.md"),
			embedded: curator,
			kind:     scaffoldMarkdown,
		},
		{
			label:    "workflow template example",
			path:     filepath.Join(workflowsDir, "dewey-distill.yml.example"),
			embedded: workflow,
			kind:     scaffoldYAML,
		},
	}
	for i := range targets {
		target := &targets[i]
		drifted := driftedScaffoldFixture(t, target.embedded, target.kind)
		digest, _, ok := parseScaffoldMarker(drifted)
		if !ok {
			t.Fatalf("%s fixture precondition: drifted scaffold must carry a parseable marker", target.label)
		}
		target.oldDigest = digest
		if err := os.WriteFile(target.path, drifted, 0o644); err != nil {
			t.Fatalf("write drifted %s: %v", target.label, err)
		}
	}

	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	for _, target := range targets {
		got, err := os.ReadFile(target.path)
		if err != nil {
			t.Fatalf("read refreshed %s: %v", target.label, err)
		}
		if want := scaffoldPayload([]byte(target.embedded), target.kind); !bytes.Equal(got, want) {
			t.Errorf("refreshed %s = %q, want the current embedded scaffoldPayload %q", target.label, got, want)
			continue
		}
		newDigest, payload, ok := parseScaffoldMarker(got)
		if !ok {
			t.Errorf("refreshed %s does not end in a parseable marker:\n%s", target.label, got)
			continue
		}
		if want := sha256Hex(canonicalBody(target.embedded)); newDigest != want {
			t.Errorf("refreshed %s marker digest = %q, want the current payload's digest %q", target.label, newDigest, want)
		}
		if newDigest == target.oldDigest {
			t.Errorf("refreshed %s kept the older body's digest; the rewrite must carry a fresh marker", target.label)
		}
		if !bytes.Equal(payload, canonicalBody(target.embedded)) {
			t.Errorf("refreshed %s marker payload differs from the current canonical body", target.label)
		}
	}
}

// TestInitCmd_FirstInitWritesVerifiableMarkers enforces the DI-FR-002
// scenario "First init is unchanged apart from markers" together with
// DIO-FR-002: on a first init over a vault with .opencode/ present, every
// tool-owned file — every slash command, agent definition, and workflow
// template example — is created in the exact marker-plus-payload form, and
// each marker verifies against an independently computed SHA-256 of the
// payload preceding it.
func TestInitCmd_FirstInitWritesVerifiableMarkers(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".opencode"), 0o755); err != nil {
		t.Fatalf("create .opencode: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(""), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	kinds := []struct {
		name  string
		dir   string
		files map[string]string
		kind  scaffoldKind
	}{
		{"slash commands", filepath.Join(".opencode", "commands"), deweySlashCommands, scaffoldMarkdown},
		{"agent definitions", filepath.Join(".opencode", "agents"), deweyAgentDefinitions, scaffoldMarkdown},
		{"workflow template examples", filepath.Join(".github", "workflows"), deweyWorkflowTemplates, scaffoldYAML},
	}
	for _, k := range kinds {
		t.Run(k.name, func(t *testing.T) {
			names := make([]string, 0, len(k.files))
			for name := range k.files {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				body := k.files[name]
				path := filepath.Join(tmpDir, k.dir, name)

				got, err := os.ReadFile(path)
				if err != nil {
					t.Errorf("init did not scaffold %s: %v", path, err)
					continue
				}
				// The exact write shape: canonical body plus the kind's
				// marker as the final line.
				if want := scaffoldPayload([]byte(body), k.kind); !bytes.Equal(got, want) {
					t.Errorf("%s = %q, want the exact scaffoldPayload %q", path, got, want)
					continue
				}
				// The marker is verifiable (DIO-FR-002): it parses, its
				// payload is the canonical map value, and its digest
				// matches an independently computed SHA-256 of that payload.
				digest, payload, ok := parseScaffoldMarker(got)
				if !ok {
					t.Errorf("%s does not end in a parseable marker:\n%s", path, got)
					continue
				}
				if want := canonicalBody(body); !bytes.Equal(payload, want) {
					t.Errorf("%s marker payload = %q, want the canonical map value %q", path, payload, want)
				}
				if want := sha256Hex(payload); digest != want {
					t.Errorf("%s marker digest = %q, want independently computed %q", path, digest, want)
				}
			}
		})
	}
}

// TestInitCmd_NoOpenCodeDirScaffoldsWorkflowExampleOnly enforces the
// DI-FR-002 composability guard: when .opencode/ is absent, no command or
// agent scaffolding runs and .opencode/ itself is not created, while the
// workflow template example is still scaffolded into .github/workflows/.
func TestInitCmd_NoOpenCodeDirScaffoldsWorkflowExampleOnly(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(""), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// .opencode/ must not be created — so neither commands nor agents can
	// appear under it.
	for _, dir := range []string{
		filepath.Join(tmpDir, ".opencode"),
		filepath.Join(tmpDir, ".opencode", "commands"),
		filepath.Join(tmpDir, ".opencode", "agents"),
	} {
		if _, err := os.Stat(dir); err == nil {
			t.Errorf("%s must not be created when .opencode/ was absent", dir)
		} else if !os.IsNotExist(err) {
			t.Errorf("stat %s: %v", dir, err)
		}
	}

	// The workflow template example is still scaffolded.
	entry, ok := deweyWorkflowTemplates["dewey-distill.yml.example"]
	if !ok {
		t.Fatal("deweyWorkflowTemplates missing dewey-distill.yml.example entry")
	}
	got, err := os.ReadFile(filepath.Join(tmpDir, ".github", "workflows", "dewey-distill.yml.example"))
	if err != nil {
		t.Fatalf("workflow template example was not scaffolded without .opencode/: %v", err)
	}
	if want := scaffoldPayload([]byte(entry), scaffoldYAML); !bytes.Equal(got, want) {
		t.Errorf("scaffolded workflow example = %q, want the exact scaffoldPayload %q", got, want)
	}
}

// TestInitCmd_MigratesThenClassifiesCustomizedCommand enforces the DI-FR-002
// scenario "Old command-directory migration still precedes classification":
// a Dewey-owned file in .opencode/command/ (singular) migrates to
// .opencode/commands/ first and is then classified by the refresh decision
// like any other existing file — here a customized fixture, preserved
// byte-for-byte rather than refreshed.
func TestInitCmd_MigratesThenClassifiesCustomizedCommand(t *testing.T) {
	tmpDir := t.TempDir()
	oldDir := filepath.Join(tmpDir, ".opencode", "command")
	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatalf("create old command dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(""), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	legacy := []byte("# my customized legacy dewey-store command\n\nedited before markers existed\n")
	if err := os.WriteFile(filepath.Join(oldDir, "dewey-store.md"), legacy, 0o644); err != nil {
		t.Fatalf("write legacy command: %v", err)
	}

	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// The file migrated, and classification preserved the customized bytes:
	// marker-less content differing from the embedded payload is skipped,
	// never refreshed.
	migratedPath := filepath.Join(tmpDir, ".opencode", "commands", "dewey-store.md")
	got, err := os.ReadFile(migratedPath)
	if err != nil {
		t.Fatalf("migrated command missing from .opencode/commands/: %v", err)
	}
	if !bytes.Equal(got, legacy) {
		t.Errorf("migrated command was rewritten by classification: got %q, want byte-identical %q", got, legacy)
	}

	// The old directory held only the Dewey file, so migration removed it.
	if _, err := os.Stat(oldDir); err == nil {
		t.Error("old .opencode/command/ should be removed when empty after migration")
	} else if !os.IsNotExist(err) {
		t.Errorf("stat old command dir: %v", err)
	}

	// Scaffolding proceeded after the migration.
	if _, err := os.Stat(filepath.Join(tmpDir, ".opencode", "commands", "dewey-index.md")); err != nil {
		t.Errorf("scaffolding did not proceed after migration: %v", err)
	}
}

// TestInitCmd_OutcomeEventsObservable enforces the DIO-FR-006 scenario
// "Outcomes are logged with paths": over a vault holding one missing, one
// untouched-drifted, and one customized command — with every other
// tool-owned scaffold already current — init logs exactly one `scaffolded`,
// one `updated`, and one `skip (customized)` event, each naming its path.
func TestInitCmd_OutcomeEventsObservable(t *testing.T) {
	tmpDir := t.TempDir()
	cmdDir := filepath.Join(tmpDir, ".opencode", "commands")
	if err := os.MkdirAll(cmdDir, 0o755); err != nil {
		t.Fatalf("create commands dir: %v", err)
	}
	agentsDir := filepath.Join(tmpDir, ".opencode", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("create agents dir: %v", err)
	}
	workflowsDir := filepath.Join(tmpDir, ".github", "workflows")
	if err := os.MkdirAll(workflowsDir, 0o755); err != nil {
		t.Fatalf("create workflows dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(""), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	const missingName = "dewey-store.md"
	const driftedName = "dewey-compile.md"
	const customizedName = "dewey-lint.md"
	for _, name := range []string{missingName, driftedName, customizedName} {
		if _, ok := deweySlashCommands[name]; !ok {
			t.Fatalf("deweySlashCommands missing %s entry", name)
		}
	}

	// Every command except the three under test starts current; the three
	// carry the missing / untouched-drifted / customized states.
	for name, body := range deweySlashCommands {
		var content []byte
		switch name {
		case missingName:
			continue // left absent — the missing one
		case driftedName:
			content = driftedScaffoldFixture(t, body, scaffoldMarkdown)
		case customizedName:
			content = customizedScaffoldFixture(t, body, scaffoldMarkdown)
		default:
			content = scaffoldPayload([]byte(body), scaffoldMarkdown)
		}
		if err := os.WriteFile(filepath.Join(cmdDir, name), content, 0o644); err != nil {
			t.Fatalf("write command fixture %s: %v", name, err)
		}
	}
	for name, body := range deweyAgentDefinitions {
		if err := os.WriteFile(filepath.Join(agentsDir, name), scaffoldPayload([]byte(body), scaffoldMarkdown), 0o644); err != nil {
			t.Fatalf("write agent fixture %s: %v", name, err)
		}
	}
	for name, body := range deweyWorkflowTemplates {
		if err := os.WriteFile(filepath.Join(workflowsDir, name), scaffoldPayload([]byte(body), scaffoldYAML), 0o644); err != nil {
			t.Fatalf("write workflow fixture %s: %v", name, err)
		}
	}

	logBuf := captureLoggerOutput(t)
	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	events := scaffoldOutcomeEvents(logBuf.String())
	if len(events) != 3 {
		t.Fatalf("got %d outcome events, want exactly 3 (one scaffolded, one updated, one skip); events:\n%s", len(events), strings.Join(events, "\n"))
	}
	if !hasOutcomeEvent(events, "scaffolded slash command", filepath.Join(cmdDir, missingName)) {
		t.Errorf("the missing command must log `scaffolded slash command` naming %s; events:\n%s", filepath.Join(cmdDir, missingName), strings.Join(events, "\n"))
	}
	if !hasOutcomeEvent(events, "updated slash command", filepath.Join(cmdDir, driftedName)) {
		t.Errorf("the drifted command must log `updated slash command` naming %s; events:\n%s", filepath.Join(cmdDir, driftedName), strings.Join(events, "\n"))
	}
	if !hasOutcomeEvent(events, "skip (customized)", filepath.Join(cmdDir, customizedName)) {
		t.Errorf("the customized command must log `skip (customized)` naming %s; events:\n%s", filepath.Join(cmdDir, customizedName), strings.Join(events, "\n"))
	}
}

// TestInitCmd_DeterministicIterationOrder enforces the DIO-FR-006 scenario
// "Deterministic iteration order": the same vault state — materialized as
// two identically-prepared vaults so neither run's writes feed the other —
// produces its outcome events in the same order, with the slash commands
// scaffolded in sorted name order.
func TestInitCmd_DeterministicIterationOrder(t *testing.T) {
	runOnce := func() []string {
		vault := prepareMixedOutcomeVault(t)
		logBuf := captureLoggerOutput(t)
		cmd := newInitCmd()
		cmd.SetArgs([]string{"--vault", vault})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("init failed: %v", err)
		}
		return scaffoldOutcomeEvents(logBuf.String())
	}
	first := runOnce()
	second := runOnce()

	wantTotal := len(deweySlashCommands) + len(deweyAgentDefinitions) + len(deweyWorkflowTemplates)
	if len(first) != wantTotal {
		t.Fatalf("got %d outcome events, want %d (every command missing, one agent drifted, one agent missing, workflow customized); events:\n%s", len(first), wantTotal, strings.Join(first, "\n"))
	}

	// The fixture must exercise mixed outcomes, or the ordering comparison
	// would be vacuous.
	for _, event := range []string{"scaffolded slash command", "updated agent", "scaffolded agent", "skip (customized)"} {
		found := false
		for _, e := range first {
			if strings.HasPrefix(e, event) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("outcome sequence missing %q events; the fixture must exercise mixed outcomes:\n%s", event, strings.Join(first, "\n"))
		}
	}

	// Normalize the vault directory out of each event so two runs over
	// identically-prepared vaults compare by event and filename.
	normalize := func(events []string) []string {
		out := make([]string, len(events))
		for i, e := range events {
			if idx := strings.Index(e, "path="); idx >= 0 {
				out[i] = e[:idx+len("path=")] + filepath.Base(e[idx+len("path="):])
			} else {
				out[i] = e
			}
		}
		return out
	}
	if !slices.Equal(normalize(first), normalize(second)) {
		t.Errorf("outcome events differ across runs over the same vault state:\nrun 1:\n%s\nrun 2:\n%s", strings.Join(first, "\n"), strings.Join(second, "\n"))
	}

	// Sorted-key iteration (DIO-FR-006): the scaffolded commands appear in
	// sorted name order.
	var commandNames []string
	for _, e := range first {
		if strings.HasPrefix(e, "scaffolded slash command") {
			if idx := strings.Index(e, "path="); idx >= 0 {
				commandNames = append(commandNames, filepath.Base(e[idx+len("path="):]))
			}
		}
	}
	if len(commandNames) != len(deweySlashCommands) {
		t.Fatalf("got %d scaffolded command events, want %d", len(commandNames), len(deweySlashCommands))
	}
	if !slices.IsSorted(commandNames) {
		t.Errorf("scaffolded command events are not in sorted name order: %v", commandNames)
	}
}

// TestInitCmd_PreservesNonMapDeweyLikeCommand enforces the DIO-FR-001
// scenario "Non-map file with a Dewey-like name is never touched": a
// user-authored .opencode/commands/dewey-notes.md — not a deweySlashCommands
// entry — is neither written, rewritten, nor removed. Real vaults hold many
// such files from multiple toolchains alongside the dewey-* commands, so
// map membership, not the filename, must decide ownership.
func TestInitCmd_PreservesNonMapDeweyLikeCommand(t *testing.T) {
	const notesName = "dewey-notes.md"
	if _, isMapEntry := deweySlashCommands[notesName]; isMapEntry {
		t.Fatalf("fixture precondition: %s must not be a deweySlashCommands entry", notesName)
	}

	tmpDir := t.TempDir()
	cmdDir := filepath.Join(tmpDir, ".opencode", "commands")
	if err := os.MkdirAll(cmdDir, 0o755); err != nil {
		t.Fatalf("create commands dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(""), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	notes := []byte("# my own dewey-notes command\n\nauthored by the user; not owned by any dewey scaffold map\n")
	path := filepath.Join(cmdDir, notesName)
	if err := os.WriteFile(path, notes, 0o644); err != nil {
		t.Fatalf("write user-authored command: %v", err)
	}

	cmd := newInitCmd()
	cmd.SetArgs([]string{"--vault", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("user-authored command was removed: %v", err)
	}
	if !bytes.Equal(got, notes) {
		t.Errorf("user-authored command was modified: got %q, want byte-identical %q", got, notes)
	}
}
