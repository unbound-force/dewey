package ignore

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// T005: TestNewMatcher_* tests
// ---------------------------------------------------------------------------

func TestNewMatcher_MissingGitignore(t *testing.T) {
	m, err := NewMatcher("/nonexistent/path/.gitignore", nil)
	if err != nil {
		t.Fatalf("expected no error for missing gitignore, got: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil Matcher when gitignore is missing")
	}
}

func TestNewMatcher_EmptyGitignore(t *testing.T) {
	dir := t.TempDir()
	gitignorePath := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte(""), 0644); err != nil {
		t.Fatalf("failed to write empty .gitignore: %v", err)
	}

	m, err := NewMatcher(gitignorePath, nil)
	if err != nil {
		t.Fatalf("expected no error for empty gitignore, got: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil Matcher for empty gitignore")
	}
	if len(m.patterns) != 0 {
		t.Errorf("expected 0 patterns from empty gitignore, got %d", len(m.patterns))
	}
}

func TestNewMatcher_MalformedGlob(t *testing.T) {
	dir := t.TempDir()
	gitignorePath := filepath.Join(dir, ".gitignore")

	// [invalid is a malformed glob (unclosed bracket).
	// *.log is a valid pattern that should still be parsed.
	content := "[invalid\n*.log\n"
	if err := os.WriteFile(gitignorePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write .gitignore: %v", err)
	}

	m, err := NewMatcher(gitignorePath, nil)
	if err != nil {
		t.Fatalf("expected no error despite malformed glob, got: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil Matcher despite malformed glob")
	}

	// The malformed pattern should be skipped, but *.log should work.
	if !m.ShouldSkip("debug.log", false) {
		t.Error("expected *.log pattern to match debug.log after malformed glob was skipped")
	}
}

func TestNewMatcher_ReadError(t *testing.T) {
	// Point gitignorePath at a directory instead of a file.
	// os.ReadFile on a directory returns an error, which NewMatcher
	// should handle gracefully (same as missing file).
	dir := t.TempDir()

	m, err := NewMatcher(dir, nil)
	if err != nil {
		t.Fatalf("expected no error when gitignorePath is a directory, got: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil Matcher when gitignorePath is a directory")
	}
}

// ---------------------------------------------------------------------------
// T006: TestShouldSkip_* tests
// ---------------------------------------------------------------------------

func TestShouldSkip_DirectoryPattern(t *testing.T) {
	// Pattern "node_modules/" has dirOnly=true, so it should match
	// directories named "node_modules" but not files.
	m, err := NewMatcher("/nonexistent", []string{"node_modules/"})
	if err != nil {
		t.Fatalf("NewMatcher failed: %v", err)
	}

	if !m.ShouldSkip("node_modules", true) {
		t.Error("expected ShouldSkip(\"node_modules\", true) = true for dirOnly pattern")
	}
	if m.ShouldSkip("node_modules", false) {
		t.Error("expected ShouldSkip(\"node_modules\", false) = false for dirOnly pattern")
	}
}

func TestShouldSkip_FileGlob(t *testing.T) {
	m, err := NewMatcher("/nonexistent", []string{"*.log"})
	if err != nil {
		t.Fatalf("NewMatcher failed: %v", err)
	}

	tests := []struct {
		name   string
		isDir  bool
		expect bool
	}{
		{"debug.log", false, true},
		{"readme.md", false, false},
		{"error.log", false, true},
		{"app.log", true, true}, // glob matches dirs too (no dirOnly flag)
	}

	for _, tt := range tests {
		got := m.ShouldSkip(tt.name, tt.isDir)
		if got != tt.expect {
			t.Errorf("ShouldSkip(%q, %v) = %v, want %v", tt.name, tt.isDir, got, tt.expect)
		}
	}
}

func TestShouldSkip_Negation(t *testing.T) {
	// Patterns: "*.md" then "!important.md"
	// Last-match-wins: *.md matches all .md files, but !important.md
	// negates the match for important.md specifically.
	m, err := NewMatcher("/nonexistent", []string{"*.md", "!important.md"})
	if err != nil {
		t.Fatalf("NewMatcher failed: %v", err)
	}

	if !m.ShouldSkip("readme.md", false) {
		t.Error("expected ShouldSkip(\"readme.md\", false) = true (matched by *.md)")
	}
	if m.ShouldSkip("important.md", false) {
		t.Error("expected ShouldSkip(\"important.md\", false) = false (negated by !important.md)")
	}
}

func TestShouldSkip_HiddenDir(t *testing.T) {
	// No patterns at all — only the hardcoded hidden-directory baseline.
	m, err := NewMatcher("/nonexistent", nil)
	if err != nil {
		t.Fatalf("NewMatcher failed: %v", err)
	}

	// Hidden directories are always skipped (hardcoded baseline).
	if !m.ShouldSkip(".git", true) {
		t.Error("expected ShouldSkip(\".git\", true) = true (hidden directory baseline)")
	}
	if !m.ShouldSkip(".hg", true) {
		t.Error("expected ShouldSkip(\".hg\", true) = true (hidden directory baseline)")
	}

	// Hidden files are NOT skipped by the baseline — only directories.
	if m.ShouldSkip(".env", false) {
		t.Error("expected ShouldSkip(\".env\", false) = false (hidden file, not directory)")
	}
	if m.ShouldSkip(".gitignore", false) {
		t.Error("expected ShouldSkip(\".gitignore\", false) = false (hidden file, not directory)")
	}
}

func TestShouldSkip_PlainName(t *testing.T) {
	// Pattern "vendor" (no trailing slash, no glob) — exact string match
	// against any entry named "vendor", regardless of type.
	m, err := NewMatcher("/nonexistent", []string{"vendor"})
	if err != nil {
		t.Fatalf("NewMatcher failed: %v", err)
	}

	if !m.ShouldSkip("vendor", true) {
		t.Error("expected ShouldSkip(\"vendor\", true) = true (plain name matches directory)")
	}
	if !m.ShouldSkip("vendor", false) {
		t.Error("expected ShouldSkip(\"vendor\", false) = true (plain name matches file)")
	}
}

func TestShouldSkip_NoMatch(t *testing.T) {
	// No patterns — only the hidden-directory baseline applies.
	m, err := NewMatcher("/nonexistent", nil)
	if err != nil {
		t.Fatalf("NewMatcher failed: %v", err)
	}

	if m.ShouldSkip("src", true) {
		t.Error("expected ShouldSkip(\"src\", true) = false (no patterns, not hidden)")
	}
	if m.ShouldSkip("main.go", false) {
		t.Error("expected ShouldSkip(\"main.go\", false) = false (no patterns)")
	}
}

// ---------------------------------------------------------------------------
// T007: TestShouldSkipPath_* tests
// ---------------------------------------------------------------------------

func TestShouldSkipPath_NestedIgnored(t *testing.T) {
	m, err := NewMatcher("/nonexistent", []string{"node_modules/"})
	if err != nil {
		t.Fatalf("NewMatcher failed: %v", err)
	}

	// A file nested inside an ignored directory should be skipped.
	if !m.ShouldSkipPath("node_modules/pkg/README.md") {
		t.Error("expected ShouldSkipPath(\"node_modules/pkg/README.md\") = true")
	}

	// The directory itself at root level should also be skipped.
	if !m.ShouldSkipPath("node_modules/index.js") {
		t.Error("expected ShouldSkipPath(\"node_modules/index.js\") = true")
	}
}

func TestShouldSkipPath_HiddenInPath(t *testing.T) {
	// No patterns — hidden directory baseline applies to path components.
	m, err := NewMatcher("/nonexistent", nil)
	if err != nil {
		t.Fatalf("NewMatcher failed: %v", err)
	}

	if !m.ShouldSkipPath(".git/config") {
		t.Error("expected ShouldSkipPath(\".git/config\") = true (hidden dir in path)")
	}
	if !m.ShouldSkipPath(".hg/store/data") {
		t.Error("expected ShouldSkipPath(\".hg/store/data\") = true (hidden dir in path)")
	}
}

func TestShouldSkipPath_CleanPath(t *testing.T) {
	m, err := NewMatcher("/nonexistent", nil)
	if err != nil {
		t.Fatalf("NewMatcher failed: %v", err)
	}

	if m.ShouldSkipPath("docs/guide.md") {
		t.Error("expected ShouldSkipPath(\"docs/guide.md\") = false (clean path)")
	}
}

func TestShouldSkipPath_RootFile(t *testing.T) {
	m, err := NewMatcher("/nonexistent", nil)
	if err != nil {
		t.Fatalf("NewMatcher failed: %v", err)
	}

	if m.ShouldSkipPath("README.md") {
		t.Error("expected ShouldSkipPath(\"README.md\") = false (root file, no patterns)")
	}
}

// ---------------------------------------------------------------------------
// T008: TestShouldSkip_UnionMerge test
// ---------------------------------------------------------------------------

func TestShouldSkip_UnionMerge(t *testing.T) {
	dir := t.TempDir()
	gitignorePath := filepath.Join(dir, ".gitignore")

	// .gitignore contains node_modules/ and a negation for drafts.
	// Extra patterns contain "drafts".
	// The negation in .gitignore should NOT un-ignore the extra pattern
	// because gitignore patterns are evaluated first, then extras are
	// appended — so the extra "drafts" pattern comes AFTER "!drafts"
	// in the pattern list, and last-match-wins means "drafts" wins.
	gitignoreContent := "node_modules/\n!drafts\n"
	if err := os.WriteFile(gitignorePath, []byte(gitignoreContent), 0644); err != nil {
		t.Fatalf("failed to write .gitignore: %v", err)
	}

	m, err := NewMatcher(gitignorePath, []string{"drafts"})
	if err != nil {
		t.Fatalf("NewMatcher failed: %v", err)
	}

	// node_modules from .gitignore should be skipped.
	if !m.ShouldSkip("node_modules", true) {
		t.Error("expected ShouldSkip(\"node_modules\", true) = true (from .gitignore)")
	}

	// drafts from extra patterns should be skipped — the extra "drafts"
	// pattern appears after the gitignore "!drafts" negation, so
	// last-match-wins means the extra pattern takes precedence.
	if !m.ShouldSkip("drafts", true) {
		t.Error("expected ShouldSkip(\"drafts\", true) = true (extra pattern overrides gitignore negation)")
	}
	if !m.ShouldSkip("drafts", false) {
		t.Error("expected ShouldSkip(\"drafts\", false) = true (extra pattern applies to files too)")
	}
}
