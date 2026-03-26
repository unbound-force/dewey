package source

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSourcesConfig_ValidDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sources.yaml")
	content := `sources:
  - id: disk-local
    type: disk
    name: local
    config:
      path: "."
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
	if configs[0].ID != "disk-local" {
		t.Errorf("id = %q, want %q", configs[0].ID, "disk-local")
	}
	if configs[0].Type != "disk" {
		t.Errorf("type = %q, want %q", configs[0].Type, "disk")
	}
}

func TestLoadSourcesConfig_ValidGitHub(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sources.yaml")
	content := `sources:
  - id: github-gaze
    type: github
    name: gaze
    refresh_interval: daily
    config:
      org: unbound-force
      repos:
        - gaze
        - website
      content:
        - issues
        - pulls
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
	if configs[0].RefreshInterval != "daily" {
		t.Errorf("refresh_interval = %q, want %q", configs[0].RefreshInterval, "daily")
	}
}

func TestLoadSourcesConfig_ValidWeb(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sources.yaml")
	content := `sources:
  - id: web-go-stdlib
    type: web
    name: go-stdlib
    refresh_interval: weekly
    config:
      urls:
        - https://pkg.go.dev/std
      depth: 2
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
}

func TestLoadSourcesConfig_MissingFile(t *testing.T) {
	configs, err := LoadSourcesConfig("/nonexistent/path/sources.yaml")
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if configs != nil {
		t.Errorf("expected nil configs for missing file, got %v", configs)
	}
}

func TestLoadSourcesConfig_MissingID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sources.yaml")
	content := `sources:
  - type: disk
    name: local
    config:
      path: "."
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadSourcesConfig(path)
	if err == nil {
		t.Fatal("expected error for missing id")
	}
}

func TestLoadSourcesConfig_MissingType(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sources.yaml")
	content := `sources:
  - id: test
    name: local
    config:
      path: "."
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadSourcesConfig(path)
	if err == nil {
		t.Fatal("expected error for missing type")
	}
}

func TestLoadSourcesConfig_UnknownType(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sources.yaml")
	content := `sources:
  - id: test
    type: ftp
    name: local
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadSourcesConfig(path)
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
}

func TestLoadSourcesConfig_GitHubMissingOrg(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sources.yaml")
	content := `sources:
  - id: github-test
    type: github
    name: test
    config:
      repos:
        - repo1
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadSourcesConfig(path)
	if err == nil {
		t.Fatal("expected error for github source missing org")
	}
}

func TestParseRefreshInterval(t *testing.T) {
	tests := []struct {
		input string
		want  string // duration string for comparison
		err   bool
	}{
		{"daily", "24h0m0s", false},
		{"weekly", "168h0m0s", false},
		{"hourly", "1h0m0s", false},
		{"1h", "1h0m0s", false},
		{"30m", "30m0s", false},
		{"", "0s", false},
		{"invalid", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			d, err := ParseRefreshInterval(tt.input)
			if tt.err {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if d.String() != tt.want {
				t.Errorf("ParseRefreshInterval(%q) = %v, want %v", tt.input, d, tt.want)
			}
		})
	}
}

func TestSaveSourcesConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sources.yaml")

	configs := []SourceConfig{
		{
			ID:   "disk-local",
			Type: "disk",
			Name: "local",
			Config: map[string]any{
				"path": ".",
			},
		},
	}

	if err := SaveSourcesConfig(path, configs); err != nil {
		t.Fatalf("SaveSourcesConfig: %v", err)
	}

	// Verify we can read it back.
	loaded, err := LoadSourcesConfig(path)
	if err != nil {
		t.Fatalf("LoadSourcesConfig after save: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 config, got %d", len(loaded))
	}
	if loaded[0].ID != "disk-local" {
		t.Errorf("id = %q, want %q", loaded[0].ID, "disk-local")
	}
}

// --- validateSourceConfig Tests ---

func TestValidateSourceConfig_ValidDisk(t *testing.T) {
	src := &SourceConfig{
		ID:   "disk-local",
		Type: "disk",
		Name: "local",
		Config: map[string]any{
			"path": "/vault",
		},
	}

	if err := validateSourceConfig(src); err != nil {
		t.Fatalf("validateSourceConfig returned unexpected error: %v", err)
	}
}

func TestValidateSourceConfig_DiskDefaultsPath(t *testing.T) {
	src := &SourceConfig{
		ID:     "disk-local",
		Type:   "disk",
		Name:   "local",
		Config: nil,
	}

	if err := validateSourceConfig(src); err != nil {
		t.Fatalf("validateSourceConfig: %v", err)
	}

	// When Config is nil, validateSourceConfig should set a default path.
	if src.Config == nil {
		t.Fatal("Config should be set to default after validation")
	}
	path, ok := src.Config["path"]
	if !ok {
		t.Fatal("Config should contain 'path' key after validation")
	}
	if path != "." {
		t.Errorf("Config[path] = %q, want %q", path, ".")
	}
}

func TestValidateSourceConfig_MissingID(t *testing.T) {
	src := &SourceConfig{
		Type: "disk",
		Name: "local",
	}

	err := validateSourceConfig(src)
	if err == nil {
		t.Fatal("expected error for missing id")
	}
	if !strings.Contains(err.Error(), "missing required field: id") {
		t.Errorf("error = %q, want to contain 'missing required field: id'", err.Error())
	}
}

func TestValidateSourceConfig_MissingType(t *testing.T) {
	src := &SourceConfig{
		ID:   "test",
		Name: "local",
	}

	err := validateSourceConfig(src)
	if err == nil {
		t.Fatal("expected error for missing type")
	}
	if !strings.Contains(err.Error(), "missing required field: type") {
		t.Errorf("error = %q, want to contain 'missing required field: type'", err.Error())
	}
}

func TestValidateSourceConfig_MissingName(t *testing.T) {
	src := &SourceConfig{
		ID:   "test",
		Type: "disk",
	}

	err := validateSourceConfig(src)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	if !strings.Contains(err.Error(), "missing required field: name") {
		t.Errorf("error = %q, want to contain 'missing required field: name'", err.Error())
	}
}

func TestValidateSourceConfig_UnknownType(t *testing.T) {
	src := &SourceConfig{
		ID:   "test",
		Type: "ftp",
		Name: "bad",
	}

	err := validateSourceConfig(src)
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
	if !strings.Contains(err.Error(), "unknown source type: ftp") {
		t.Errorf("error = %q, want to contain 'unknown source type: ftp'", err.Error())
	}
}

func TestValidateSourceConfig_ValidGitHub(t *testing.T) {
	src := &SourceConfig{
		ID:   "github-test",
		Type: "github",
		Name: "test",
		Config: map[string]any{
			"org":   "myorg",
			"repos": []any{"repo1"},
		},
	}

	if err := validateSourceConfig(src); err != nil {
		t.Fatalf("validateSourceConfig returned unexpected error: %v", err)
	}
}

func TestValidateSourceConfig_GitHubMissingConfig(t *testing.T) {
	src := &SourceConfig{
		ID:     "github-test",
		Type:   "github",
		Name:   "test",
		Config: nil,
	}

	err := validateSourceConfig(src)
	if err == nil {
		t.Fatal("expected error for github source with nil config")
	}
	if !strings.Contains(err.Error(), "github source requires config") {
		t.Errorf("error = %q, want to contain 'github source requires config'", err.Error())
	}
}

func TestValidateSourceConfig_GitHubMissingOrg(t *testing.T) {
	src := &SourceConfig{
		ID:   "github-test",
		Type: "github",
		Name: "test",
		Config: map[string]any{
			"repos": []any{"repo1"},
		},
	}

	err := validateSourceConfig(src)
	if err == nil {
		t.Fatal("expected error for github source missing org")
	}
	if !strings.Contains(err.Error(), "requires 'org'") {
		t.Errorf("error = %q, want to contain \"requires 'org'\"", err.Error())
	}
}

func TestValidateSourceConfig_GitHubMissingRepos(t *testing.T) {
	src := &SourceConfig{
		ID:   "github-test",
		Type: "github",
		Name: "test",
		Config: map[string]any{
			"org": "myorg",
		},
	}

	err := validateSourceConfig(src)
	if err == nil {
		t.Fatal("expected error for github source missing repos")
	}
	if !strings.Contains(err.Error(), "requires 'repos'") {
		t.Errorf("error = %q, want to contain \"requires 'repos'\"", err.Error())
	}
}

func TestValidateSourceConfig_ValidWeb(t *testing.T) {
	src := &SourceConfig{
		ID:   "web-docs",
		Type: "web",
		Name: "docs",
		Config: map[string]any{
			"urls": []any{"https://example.com"},
		},
	}

	if err := validateSourceConfig(src); err != nil {
		t.Fatalf("validateSourceConfig returned unexpected error: %v", err)
	}
}

func TestValidateSourceConfig_WebMissingConfig(t *testing.T) {
	src := &SourceConfig{
		ID:     "web-docs",
		Type:   "web",
		Name:   "docs",
		Config: nil,
	}

	err := validateSourceConfig(src)
	if err == nil {
		t.Fatal("expected error for web source with nil config")
	}
	if !strings.Contains(err.Error(), "web source requires config") {
		t.Errorf("error = %q, want to contain 'web source requires config'", err.Error())
	}
}

func TestValidateSourceConfig_WebMissingURLs(t *testing.T) {
	src := &SourceConfig{
		ID:   "web-docs",
		Type: "web",
		Name: "docs",
		Config: map[string]any{
			"depth": 2,
		},
	}

	err := validateSourceConfig(src)
	if err == nil {
		t.Fatal("expected error for web source missing urls")
	}
	if !strings.Contains(err.Error(), "requires 'urls'") {
		t.Errorf("error = %q, want to contain \"requires 'urls'\"", err.Error())
	}
}
