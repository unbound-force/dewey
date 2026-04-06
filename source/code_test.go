package source

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	// Blank import to ensure the Go chunker is registered via init().
	// Without this, chunker.ForExtension(".go") returns (nil, false)
	// and no files are processed.
	_ "github.com/unbound-force/dewey/chunker"
)

// validGoSource is a minimal Go source file with exported declarations
// that the Go chunker can extract blocks from.
const validGoSource = `package example

// Config holds application configuration.
type Config struct {
	Port int
	Host string
}

// NewConfig creates a Config with default values.
func NewConfig() *Config {
	return &Config{Port: 8080, Host: "localhost"}
}

func unexported() {}
`

// testGoSource is a Go test file that should be skipped by CodeSource.
const testGoSource = `package example

import "testing"

func TestSomething(t *testing.T) {
	t.Log("test")
}
`

// syntaxErrorGoSource is a Go file with invalid syntax.
const syntaxErrorGoSource = `package example

func Broken( {
`

// createCodeTestDir creates a temporary directory with Go source files
// for testing CodeSource. Returns the directory path.
func createCodeTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	files := map[string]string{
		"main.go":           validGoSource,
		"main_test.go":      testGoSource,
		"cmd/serve.go":      validGoSource,
		"internal/util.go":  validGoSource,
		"vendor/dep/dep.go": validGoSource,
		"README.md":         "# README\nNot a Go file.",
	}

	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	return dir
}

func TestCodeSource_List(t *testing.T) {
	dir := createCodeTestDir(t)
	cs := NewCodeSource("code-test", "test", dir, []string{"go"})

	docs, err := cs.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	// Should find main.go, cmd/serve.go, internal/util.go, vendor/dep/dep.go.
	// Should NOT find main_test.go (test file) or README.md (not Go).
	if len(docs) < 1 {
		t.Fatal("expected at least 1 document, got 0")
	}

	// Build a set of document IDs for easy lookup.
	docIDs := make(map[string]bool)
	for _, doc := range docs {
		docIDs[doc.ID] = true
	}

	// main.go should be included.
	if !docIDs["main.go"] {
		t.Error("expected main.go to be included")
	}

	// main_test.go should NOT be included (test file).
	if docIDs["main_test.go"] {
		t.Error("expected main_test.go to be excluded (test file)")
	}

	// README.md should NOT be included (not a Go file).
	if docIDs["README.md"] {
		t.Error("expected README.md to be excluded (not a Go file)")
	}

	// Verify document content is markdown-formatted.
	for _, doc := range docs {
		if !strings.HasPrefix(doc.Content, "# ") {
			t.Errorf("document %q content should start with '# ', got %q",
				doc.ID, doc.Content[:min(50, len(doc.Content))])
		}
		if !strings.Contains(doc.Content, "## ") {
			t.Errorf("document %q content should contain '## ' headings", doc.ID)
		}
	}

	// Verify content hashes are set.
	for _, doc := range docs {
		if doc.ContentHash == "" {
			t.Errorf("document %q has empty content hash", doc.ID)
		}
		if doc.SourceID != "code-test" {
			t.Errorf("document %q source_id = %q, want %q", doc.ID, doc.SourceID, "code-test")
		}
	}

	// Verify properties are set.
	for _, doc := range docs {
		if doc.Properties == nil {
			t.Errorf("document %q has nil properties", doc.ID)
			continue
		}
		if doc.Properties["language"] != "go" {
			t.Errorf("document %q language = %v, want %q", doc.ID, doc.Properties["language"], "go")
		}
		if doc.Properties["file_path"] == nil {
			t.Errorf("document %q missing file_path property", doc.ID)
		}
		bc, ok := doc.Properties["block_count"].(int)
		if !ok || bc < 1 {
			t.Errorf("document %q block_count = %v, want >= 1", doc.ID, doc.Properties["block_count"])
		}
	}
}

func TestCodeSource_ListSkipsTestFiles(t *testing.T) {
	dir := t.TempDir()

	// Create only test files and one regular file.
	files := map[string]string{
		"foo.go":      validGoSource,
		"foo_test.go": testGoSource,
		"bar_test.go": testGoSource,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	cs := NewCodeSource("code-test", "test", dir, []string{"go"})
	docs, err := cs.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	// Should only include foo.go, not the test files.
	if len(docs) != 1 {
		t.Fatalf("expected 1 document, got %d", len(docs))
	}
	if docs[0].ID != "foo.go" {
		t.Errorf("expected foo.go, got %q", docs[0].ID)
	}
}

func TestCodeSource_ListSkipsSyntaxErrors(t *testing.T) {
	dir := t.TempDir()

	files := map[string]string{
		"good.go":   validGoSource,
		"broken.go": syntaxErrorGoSource,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	cs := NewCodeSource("code-test", "test", dir, []string{"go"})
	docs, err := cs.List()
	if err != nil {
		t.Fatalf("List should not fail due to syntax errors: %v", err)
	}

	// Should include good.go but skip broken.go.
	if len(docs) != 1 {
		t.Fatalf("expected 1 document (broken.go skipped), got %d", len(docs))
	}
	if docs[0].ID != "good.go" {
		t.Errorf("expected good.go, got %q", docs[0].ID)
	}
}

func TestCodeSource_ListRespectsInclude(t *testing.T) {
	dir := createCodeTestDir(t)
	cs := NewCodeSource("code-test", "test", dir, []string{"go"},
		WithCodeInclude([]string{"cmd/"}),
	)

	docs, err := cs.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	// Only cmd/serve.go should be included.
	if len(docs) != 1 {
		t.Fatalf("expected 1 document (only cmd/), got %d", len(docs))
	}
	if docs[0].ID != "cmd/serve.go" {
		t.Errorf("expected cmd/serve.go, got %q", docs[0].ID)
	}
}

func TestCodeSource_ListRespectsExclude(t *testing.T) {
	dir := createCodeTestDir(t)
	cs := NewCodeSource("code-test", "test", dir, []string{"go"},
		WithCodeExclude([]string{"vendor/"}),
	)

	docs, err := cs.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	// Build a set of document IDs.
	docIDs := make(map[string]bool)
	for _, doc := range docs {
		docIDs[doc.ID] = true
	}

	// vendor/dep/dep.go should be excluded.
	if docIDs["vendor/dep/dep.go"] {
		t.Error("expected vendor/dep/dep.go to be excluded by exclude pattern")
	}

	// Other files should still be included.
	if !docIDs["main.go"] {
		t.Error("expected main.go to be included")
	}
	if !docIDs["cmd/serve.go"] {
		t.Error("expected cmd/serve.go to be included")
	}
}

func TestCodeSource_RespectsGitignore(t *testing.T) {
	dir := t.TempDir()

	// Create a .gitignore that excludes the build/ directory.
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("build/\n"), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	files := map[string]string{
		"main.go":      validGoSource,
		"build/gen.go": validGoSource,
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	cs := NewCodeSource("code-test", "test", dir, []string{"go"})
	docs, err := cs.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	docIDs := make(map[string]bool)
	for _, doc := range docs {
		docIDs[doc.ID] = true
	}

	if !docIDs["main.go"] {
		t.Error("expected main.go to be included")
	}
	if docIDs["build/gen.go"] {
		t.Error("expected build/gen.go to be excluded by .gitignore")
	}
}

func TestCodeSource_RecursiveFalse(t *testing.T) {
	dir := createCodeTestDir(t)
	cs := NewCodeSource("code-test", "test", dir, []string{"go"},
		WithCodeRecursive(false),
	)

	docs, err := cs.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	// Only main.go (root level) should be returned.
	// cmd/serve.go, internal/util.go, vendor/dep/dep.go are in subdirs.
	docIDs := make(map[string]bool)
	for _, doc := range docs {
		docIDs[doc.ID] = true
	}

	if !docIDs["main.go"] {
		t.Error("expected main.go to be included (root level)")
	}
	if docIDs["cmd/serve.go"] {
		t.Error("expected cmd/serve.go to be excluded (subdirectory)")
	}
	if docIDs["internal/util.go"] {
		t.Error("expected internal/util.go to be excluded (subdirectory)")
	}

	if len(docs) != 1 {
		t.Errorf("expected 1 document, got %d", len(docs))
		for _, doc := range docs {
			t.Logf("  included: %s", doc.ID)
		}
	}
}

func TestCodeSource_UnsupportedLanguage(t *testing.T) {
	dir := t.TempDir()

	// Create a Go file that should still be found via the "go" language.
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(validGoSource), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// "typescript" is not registered — should log a warning but not error.
	cs := NewCodeSource("code-test", "test", dir, []string{"typescript"})
	docs, err := cs.List()
	if err != nil {
		t.Fatalf("List should not error for unsupported language: %v", err)
	}

	// No documents should be returned since typescript has no chunker
	// and we only configured typescript (not go).
	if len(docs) != 0 {
		t.Errorf("expected 0 documents for unsupported language, got %d", len(docs))
	}
}

func TestCodeSource_Meta(t *testing.T) {
	cs := NewCodeSource("code-test", "test", "/tmp/test", []string{"go"})
	meta := cs.Meta()

	if meta.ID != "code-test" {
		t.Errorf("id = %q, want %q", meta.ID, "code-test")
	}
	if meta.Type != "code" {
		t.Errorf("type = %q, want %q", meta.Type, "code")
	}
	if meta.Name != "test" {
		t.Errorf("name = %q, want %q", meta.Name, "test")
	}
	if meta.Status != "active" {
		t.Errorf("status = %q, want %q", meta.Status, "active")
	}
}

func TestCodeSource_Fetch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(validGoSource), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cs := NewCodeSource("code-test", "test", dir, []string{"go"})
	doc, err := cs.Fetch("main.go")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if doc.ID != "main.go" {
		t.Errorf("ID = %q, want %q", doc.ID, "main.go")
	}
	if doc.Content == "" {
		t.Error("Content should not be empty")
	}
	if doc.ContentHash == "" {
		t.Error("ContentHash should not be empty")
	}
	if !strings.Contains(doc.Content, "## type Config") {
		t.Error("Content should contain type Config heading")
	}
	if !strings.Contains(doc.Content, "## func NewConfig") {
		t.Error("Content should contain func NewConfig heading")
	}
}

func TestCodeSource_Diff_NewFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(validGoSource), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cs := NewCodeSource("code-test", "test", dir, []string{"go"})

	// No stored hashes — all files should be "added".
	changes, err := cs.Diff()
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}

	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Type != ChangeAdded {
		t.Errorf("Type = %q, want %q", changes[0].Type, ChangeAdded)
	}
	if changes[0].ID != "main.go" {
		t.Errorf("ID = %q, want %q", changes[0].ID, "main.go")
	}
}

func TestCodeSource_Diff_DeletedFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(validGoSource), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cs := NewCodeSource("code-test", "test", dir, []string{"go"})

	// Set stored hashes including a file that no longer exists.
	cs.SetStoredHashes(map[string]string{
		"main.go":    computeHash("old content"),
		"deleted.go": computeHash("deleted content"),
	})

	changes, err := cs.Diff()
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}

	changeMap := make(map[string]ChangeType)
	for _, c := range changes {
		changeMap[c.ID] = c.Type
	}

	if changeMap["deleted.go"] != ChangeDeleted {
		t.Errorf("deleted.go should be ChangeDeleted, got %q", changeMap["deleted.go"])
	}
}

func TestCreateCodeSource_ValidConfig(t *testing.T) {
	cfg := SourceConfig{
		ID:   "code-test",
		Type: "code",
		Name: "test",
		Config: map[string]any{
			"path":      "/some/project",
			"languages": []any{"go"},
			"include":   []any{"cmd/", "internal/"},
			"exclude":   []any{"vendor/"},
			"ignore":    []any{"generated_*.go"},
			"recursive": true,
		},
	}

	src := createCodeSource(cfg, "/fallback/base")
	if src == nil {
		t.Fatal("createCodeSource returned nil for valid config")
	}

	cs, ok := src.(*CodeSource)
	if !ok {
		t.Fatalf("expected *CodeSource, got %T", src)
	}
	if cs.id != "code-test" {
		t.Errorf("id = %q, want %q", cs.id, "code-test")
	}
	if cs.name != "test" {
		t.Errorf("name = %q, want %q", cs.name, "test")
	}
	if cs.basePath != "/some/project" {
		t.Errorf("basePath = %q, want %q", cs.basePath, "/some/project")
	}
	if len(cs.languages) != 1 || cs.languages[0] != "go" {
		t.Errorf("languages = %v, want [go]", cs.languages)
	}
	if len(cs.include) != 2 {
		t.Errorf("include count = %d, want 2", len(cs.include))
	}
	if len(cs.exclude) != 1 {
		t.Errorf("exclude count = %d, want 1", len(cs.exclude))
	}
	if len(cs.ignorePatterns) != 1 {
		t.Errorf("ignorePatterns count = %d, want 1", len(cs.ignorePatterns))
	}
	if !cs.recursive {
		t.Error("recursive should be true")
	}
}

func TestCreateCodeSource_DotPathFallsBackToBasePath(t *testing.T) {
	cfg := SourceConfig{
		ID:   "code-test",
		Type: "code",
		Name: "test",
		Config: map[string]any{
			"path":      ".",
			"languages": []any{"go"},
		},
	}

	src := createCodeSource(cfg, "/fallback/base")
	cs, ok := src.(*CodeSource)
	if !ok {
		t.Fatal("createCodeSource did not return *CodeSource")
	}
	if cs.basePath != "/fallback/base" {
		t.Errorf("basePath = %q, want %q (dot path should resolve to basePath)",
			cs.basePath, "/fallback/base")
	}
}

func TestValidateCodeSourceConfig_Valid(t *testing.T) {
	src := &SourceConfig{
		ID:   "code-test",
		Type: "code",
		Name: "test",
		Config: map[string]any{
			"path":      "/some/project",
			"languages": []any{"go"},
		},
	}

	if err := validateSourceConfig(src); err != nil {
		t.Fatalf("validateSourceConfig returned unexpected error: %v", err)
	}
}

func TestValidateCodeSourceConfig_MissingConfig(t *testing.T) {
	src := &SourceConfig{
		ID:     "code-test",
		Type:   "code",
		Name:   "test",
		Config: nil,
	}

	err := validateSourceConfig(src)
	if err == nil {
		t.Fatal("expected error for code source with nil config")
	}
	if !strings.Contains(err.Error(), "code source requires config") {
		t.Errorf("error = %q, want to contain 'code source requires config'", err.Error())
	}
}

func TestValidateCodeSourceConfig_MissingPath(t *testing.T) {
	src := &SourceConfig{
		ID:   "code-test",
		Type: "code",
		Name: "test",
		Config: map[string]any{
			"languages": []any{"go"},
		},
	}

	err := validateSourceConfig(src)
	if err == nil {
		t.Fatal("expected error for code source missing path")
	}
	if !strings.Contains(err.Error(), "requires 'path'") {
		t.Errorf("error = %q, want to contain \"requires 'path'\"", err.Error())
	}
}

func TestValidateCodeSourceConfig_MissingLanguages(t *testing.T) {
	src := &SourceConfig{
		ID:   "code-test",
		Type: "code",
		Name: "test",
		Config: map[string]any{
			"path": "/some/project",
		},
	}

	err := validateSourceConfig(src)
	if err == nil {
		t.Fatal("expected error for code source missing languages")
	}
	if !strings.Contains(err.Error(), "requires 'languages'") {
		t.Errorf("error = %q, want to contain \"requires 'languages'\"", err.Error())
	}
}

func TestValidateCodeSourceConfig_EmptyLanguages(t *testing.T) {
	src := &SourceConfig{
		ID:   "code-test",
		Type: "code",
		Name: "test",
		Config: map[string]any{
			"path":      "/some/project",
			"languages": []any{},
		},
	}

	err := validateSourceConfig(src)
	if err == nil {
		t.Fatal("expected error for code source with empty languages")
	}
	if !strings.Contains(err.Error(), "requires 'languages'") {
		t.Errorf("error = %q, want to contain \"requires 'languages'\"", err.Error())
	}
}

func TestValidateCodeSourceConfig_EmptyPath(t *testing.T) {
	src := &SourceConfig{
		ID:   "code-test",
		Type: "code",
		Name: "test",
		Config: map[string]any{
			"path":      "",
			"languages": []any{"go"},
		},
	}

	err := validateSourceConfig(src)
	if err == nil {
		t.Fatal("expected error for code source with empty path")
	}
	if !strings.Contains(err.Error(), "requires 'path'") {
		t.Errorf("error = %q, want to contain \"requires 'path'\"", err.Error())
	}
}

func TestCodeSource_FormatBlocksAsMarkdown(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(validGoSource), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cs := NewCodeSource("code-test", "test", dir, []string{"go"})
	docs, err := cs.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("expected 1 document, got %d", len(docs))
	}

	content := docs[0].Content

	// Verify the document starts with a level-1 heading.
	if !strings.HasPrefix(content, "# main.go\n") {
		t.Errorf("content should start with '# main.go\\n', got %q",
			content[:min(30, len(content))])
	}

	// Verify it contains level-2 headings for declarations.
	if !strings.Contains(content, "## type Config") {
		t.Error("content should contain '## type Config'")
	}
	if !strings.Contains(content, "## func NewConfig") {
		t.Error("content should contain '## func NewConfig'")
	}

	// Verify it does NOT contain unexported function.
	if strings.Contains(content, "unexported") {
		t.Error("content should not contain unexported function")
	}
}

func TestCodeSource_IncludeExcludeCombined(t *testing.T) {
	dir := t.TempDir()

	// Create files in different directories.
	files := map[string]string{
		"cmd/main.go":       validGoSource,
		"cmd/util.go":       validGoSource,
		"internal/core.go":  validGoSource,
		"vendor/dep/dep.go": validGoSource,
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	// Include cmd/ and internal/, but exclude vendor/.
	cs := NewCodeSource("code-test", "test", dir, []string{"go"},
		WithCodeInclude([]string{"cmd/", "internal/"}),
		WithCodeExclude([]string{"vendor/"}),
	)

	docs, err := cs.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	docIDs := make(map[string]bool)
	for _, doc := range docs {
		docIDs[doc.ID] = true
	}

	if !docIDs["cmd/main.go"] {
		t.Error("expected cmd/main.go to be included")
	}
	if !docIDs["cmd/util.go"] {
		t.Error("expected cmd/util.go to be included")
	}
	if !docIDs["internal/core.go"] {
		t.Error("expected internal/core.go to be included")
	}
	if docIDs["vendor/dep/dep.go"] {
		t.Error("expected vendor/dep/dep.go to be excluded")
	}
}

// TestLoadSourcesConfig_ValidCode verifies that a code source config
// can be loaded from YAML and passes validation.
func TestLoadSourcesConfig_ValidCode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sources.yaml")
	content := `sources:
  - id: code-replicator
    type: code
    name: replicator
    config:
      path: "../replicator"
      languages:
        - go
      include:
        - "cmd/"
        - "internal/"
      exclude:
        - "vendor/"
      recursive: true
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	configs, err := LoadSourcesConfig(path)
	if err != nil {
		t.Fatalf("LoadSourcesConfig: %v", err)
	}
	if len(configs) != 1 {
		t.Fatalf("expected 1 config, got %d", len(configs))
	}
	if configs[0].ID != "code-replicator" {
		t.Errorf("id = %q, want %q", configs[0].ID, "code-replicator")
	}
	if configs[0].Type != "code" {
		t.Errorf("type = %q, want %q", configs[0].Type, "code")
	}
}
