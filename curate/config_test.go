package curate

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadKnowledgeStoresConfig_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "knowledge-stores.yaml")

	content := `stores:
  - name: team-decisions
    sources: [disk-local, github-org]
    path: /tmp/knowledge/team
    curate_on_index: true
    curation_interval: 30m
  - name: architecture
    sources: [disk-docs]
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	stores, err := LoadKnowledgeStoresConfig(path)
	if err != nil {
		t.Fatalf("LoadKnowledgeStoresConfig: %v", err)
	}
	if len(stores) != 2 {
		t.Fatalf("got %d stores, want 2", len(stores))
	}

	// First store: explicit values.
	if stores[0].Name != "team-decisions" {
		t.Errorf("store[0].Name = %q, want %q", stores[0].Name, "team-decisions")
	}
	if len(stores[0].Sources) != 2 {
		t.Errorf("store[0].Sources len = %d, want 2", len(stores[0].Sources))
	}
	if stores[0].Path != "/tmp/knowledge/team" {
		t.Errorf("store[0].Path = %q, want %q", stores[0].Path, "/tmp/knowledge/team")
	}
	if !stores[0].CurateOnIndex {
		t.Error("store[0].CurateOnIndex = false, want true")
	}
	if stores[0].CurationInterval != "30m" {
		t.Errorf("store[0].CurationInterval = %q, want %q", stores[0].CurationInterval, "30m")
	}

	// Second store: defaults applied.
	if stores[1].Name != "architecture" {
		t.Errorf("store[1].Name = %q, want %q", stores[1].Name, "architecture")
	}
	if stores[1].CurationInterval != "10m" {
		t.Errorf("store[1].CurationInterval = %q, want %q (default)", stores[1].CurationInterval, "10m")
	}
	if stores[1].CurateOnIndex {
		t.Error("store[1].CurateOnIndex = true, want false (default)")
	}
}

func TestLoadKnowledgeStoresConfig_MissingFile(t *testing.T) {
	stores, err := LoadKnowledgeStoresConfig("/nonexistent/path/knowledge-stores.yaml")
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if stores != nil {
		t.Fatalf("expected nil stores for missing file, got: %v", stores)
	}
}

func TestLoadKnowledgeStoresConfig_MalformedYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "knowledge-stores.yaml")

	content := `stores:
  - name: [invalid yaml structure
    sources: {broken`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	_, err := LoadKnowledgeStoresConfig(path)
	if err == nil {
		t.Fatal("expected error for malformed YAML, got nil")
	}
}

func TestLoadKnowledgeStoresConfig_DuplicateNames(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "knowledge-stores.yaml")

	content := `stores:
  - name: decisions
    sources: [disk-local]
  - name: decisions
    sources: [disk-docs]
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	_, err := LoadKnowledgeStoresConfig(path)
	if err == nil {
		t.Fatal("expected error for duplicate store names, got nil")
	}
}

func TestLoadKnowledgeStoresConfig_EmptyName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "knowledge-stores.yaml")

	content := `stores:
  - name: ""
    sources: [disk-local]
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	_, err := LoadKnowledgeStoresConfig(path)
	if err == nil {
		t.Fatal("expected error for empty store name, got nil")
	}
}

func TestLoadKnowledgeStoresConfig_EmptySources(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "knowledge-stores.yaml")

	content := `stores:
  - name: empty-store
    sources: []
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	// Empty sources should not error — just warn.
	stores, err := LoadKnowledgeStoresConfig(path)
	if err != nil {
		t.Fatalf("expected no error for empty sources, got: %v", err)
	}
	if len(stores) != 1 {
		t.Fatalf("got %d stores, want 1", len(stores))
	}
	if len(stores[0].Sources) != 0 {
		t.Errorf("store[0].Sources len = %d, want 0", len(stores[0].Sources))
	}
}

func TestLoadKnowledgeStoresConfig_InvalidInterval(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "knowledge-stores.yaml")

	content := `stores:
  - name: bad-interval
    sources: [disk-local]
    curation_interval: "not-a-duration"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	_, err := LoadKnowledgeStoresConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid curation interval, got nil")
	}
}

func TestLoadKnowledgeStoresConfig_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "knowledge-stores.yaml")

	// An empty file or one with only comments should return empty stores.
	content := `# just a comment
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	stores, err := LoadKnowledgeStoresConfig(path)
	if err != nil {
		t.Fatalf("expected no error for empty config, got: %v", err)
	}
	if len(stores) != 0 {
		t.Fatalf("got %d stores, want 0", len(stores))
	}
}

func TestResolveStorePath_Empty(t *testing.T) {
	cfg := StoreConfig{Name: "team-decisions"}
	got := ResolveStorePath(cfg, "/vault")
	want := filepath.Join("/vault", ".uf", "dewey", "knowledge", "team-decisions")
	if got != want {
		t.Errorf("ResolveStorePath(empty) = %q, want %q", got, want)
	}
}

func TestResolveStorePath_Absolute(t *testing.T) {
	cfg := StoreConfig{Name: "team", Path: "/absolute/path/knowledge"}
	got := ResolveStorePath(cfg, "/vault")
	if got != "/absolute/path/knowledge" {
		t.Errorf("ResolveStorePath(absolute) = %q, want %q", got, "/absolute/path/knowledge")
	}
}

func TestResolveStorePath_Relative(t *testing.T) {
	cfg := StoreConfig{Name: "team", Path: "custom/knowledge"}
	got := ResolveStorePath(cfg, "/vault")
	want := filepath.Join("/vault", "custom", "knowledge")
	if got != want {
		t.Errorf("ResolveStorePath(relative) = %q, want %q", got, want)
	}
}

func TestParseCurationInterval_Empty(t *testing.T) {
	d, err := ParseCurationInterval("")
	if err != nil {
		t.Fatalf("ParseCurationInterval empty: %v", err)
	}
	if d != 10*time.Minute {
		t.Errorf("ParseCurationInterval empty = %v, want %v", d, 10*time.Minute)
	}
}

func TestParseCurationInterval_Valid(t *testing.T) {
	tests := []struct {
		input string
		want  time.Duration
	}{
		{"10m", 10 * time.Minute},
		{"1h", time.Hour},
		{"30s", 30 * time.Second},
		{"daily", 24 * time.Hour},
		{"hourly", time.Hour},
	}
	for _, tt := range tests {
		d, err := ParseCurationInterval(tt.input)
		if err != nil {
			t.Errorf("ParseCurationInterval(%q): %v", tt.input, err)
			continue
		}
		if d != tt.want {
			t.Errorf("ParseCurationInterval(%q) = %v, want %v", tt.input, d, tt.want)
		}
	}
}

func TestParseCurationInterval_Invalid(t *testing.T) {
	_, err := ParseCurationInterval("not-a-duration")
	if err == nil {
		t.Fatal("expected error for invalid duration, got nil")
	}
}

func TestValidateConfig_MissingSource(t *testing.T) {
	stores := []StoreConfig{
		{Name: "test", Sources: []string{"disk-local", "unknown-source"}},
	}
	sourceIDs := []string{"disk-local", "github-org"}

	warnings := ValidateConfig(stores, sourceIDs)
	if len(warnings) != 1 {
		t.Fatalf("got %d warnings, want 1", len(warnings))
	}
	if warnings[0] == "" {
		t.Error("expected non-empty warning message")
	}
}

func TestValidateConfig_AllSourcesExist(t *testing.T) {
	stores := []StoreConfig{
		{Name: "test", Sources: []string{"disk-local", "github-org"}},
	}
	sourceIDs := []string{"disk-local", "github-org"}

	warnings := ValidateConfig(stores, sourceIDs)
	if len(warnings) != 0 {
		t.Errorf("got %d warnings, want 0: %v", len(warnings), warnings)
	}
}
