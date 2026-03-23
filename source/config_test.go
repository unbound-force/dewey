package source

import (
	"os"
	"path/filepath"
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
