package embed

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewEmbedderFromConfig_Ollama(t *testing.T) {
	cfg := ProviderConfig{
		Provider: "ollama",
		Model:    "granite-embedding:30m",
		Endpoint: "http://localhost:11434",
	}
	e, err := NewEmbedderFromConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e.ModelID() != "granite-embedding:30m" {
		t.Errorf("ModelID = %q, want granite-embedding:30m", e.ModelID())
	}
}

func TestNewEmbedderFromConfig_EmptyDefaultsToOllama(t *testing.T) {
	cfg := ProviderConfig{
		Model: "granite-embedding:30m",
	}
	e, err := NewEmbedderFromConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := e.(*OllamaEmbedder); !ok {
		t.Errorf("expected *OllamaEmbedder, got %T", e)
	}
}

func TestNewEmbedderFromConfig_Vertex(t *testing.T) {
	cfg := ProviderConfig{
		Provider: "vertex",
		Model:    "text-embedding-005",
		Project:  "my-project",
		Region:   "us-central1",
	}
	e, err := NewEmbedderFromConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e.ModelID() != "text-embedding-005" {
		t.Errorf("ModelID = %q, want text-embedding-005", e.ModelID())
	}
}

func TestNewEmbedderFromConfig_VertexMissingProject(t *testing.T) {
	cfg := ProviderConfig{
		Provider: "vertex",
		Model:    "text-embedding-005",
		Region:   "us-central1",
	}
	_, err := NewEmbedderFromConfig(cfg)
	if err == nil {
		t.Fatal("expected error for missing project")
	}
	if !strings.Contains(err.Error(), "project") {
		t.Errorf("error = %q, want to contain 'project'", err.Error())
	}
}

func TestNewEmbedderFromConfig_VertexMissingRegion(t *testing.T) {
	cfg := ProviderConfig{
		Provider: "vertex",
		Model:    "text-embedding-005",
		Project:  "my-project",
	}
	_, err := NewEmbedderFromConfig(cfg)
	if err == nil {
		t.Fatal("expected error for missing region")
	}
	if !strings.Contains(err.Error(), "region") {
		t.Errorf("error = %q, want to contain 'region'", err.Error())
	}
}

func TestNewEmbedderFromConfig_UnknownProvider(t *testing.T) {
	cfg := ProviderConfig{
		Provider: "unsupported",
	}
	_, err := NewEmbedderFromConfig(cfg)
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("error = %q, want to contain 'unsupported'", err.Error())
	}
}

func TestReadEmbeddingConfig_FromFile(t *testing.T) {
	dir := t.TempDir()
	configYAML := `embedding:
  provider: vertex
  model: text-embedding-005
  project: my-project
  region: us-central1
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg := ReadEmbeddingConfig(dir)
	if cfg.Provider != "vertex" {
		t.Errorf("Provider = %q, want vertex", cfg.Provider)
	}
	if cfg.Model != "text-embedding-005" {
		t.Errorf("Model = %q, want text-embedding-005", cfg.Model)
	}
	if cfg.Project != "my-project" {
		t.Errorf("Project = %q, want my-project", cfg.Project)
	}
}

func TestReadEmbeddingConfig_BackwardCompatible(t *testing.T) {
	dir := t.TempDir()
	// Existing config format without provider field.
	configYAML := `embedding:
  model: granite-embedding:30m
  endpoint: http://localhost:11434
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg := ReadEmbeddingConfig(dir)
	if cfg.Provider != "ollama" {
		t.Errorf("Provider = %q, want ollama (default)", cfg.Provider)
	}
	if cfg.Model != "granite-embedding:30m" {
		t.Errorf("Model = %q, want granite-embedding:30m", cfg.Model)
	}
}

func TestReadEmbeddingConfig_EnvOverride(t *testing.T) {
	dir := t.TempDir()
	configYAML := `embedding:
  model: granite-embedding:30m
  endpoint: http://localhost:11434
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("DEWEY_EMBEDDING_MODEL", "override-model")
	t.Setenv("DEWEY_EMBEDDING_ENDPOINT", "http://override:9999")

	cfg := ReadEmbeddingConfig(dir)
	if cfg.Model != "override-model" {
		t.Errorf("Model = %q, want override-model (from env)", cfg.Model)
	}
	if cfg.Endpoint != "http://override:9999" {
		t.Errorf("Endpoint = %q, want http://override:9999 (from env)", cfg.Endpoint)
	}
}

func TestReadEmbeddingConfig_NoFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	// No config file — should fall back to env/defaults.
	cfg := ReadEmbeddingConfig(dir)
	if cfg.Provider != "ollama" {
		t.Errorf("Provider = %q, want ollama", cfg.Provider)
	}
	if cfg.Model != "granite-embedding:30m" {
		t.Errorf("Model = %q, want granite-embedding:30m (default)", cfg.Model)
	}
}

func TestReadEmbeddingConfig_GlobalFallback(t *testing.T) {
	// Per-vault dir has no config.
	vaultDir := t.TempDir()

	// Global config has vertex provider.
	globalDir := filepath.Join(t.TempDir(), "dewey")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	globalYAML := `embedding:
  provider: vertex
  model: text-embedding-005
  project: global-project
  region: us-central1
`
	if err := os.WriteFile(filepath.Join(globalDir, "config.yaml"), []byte(globalYAML), 0644); err != nil {
		t.Fatalf("write global config: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Dir(globalDir))

	cfg := ReadEmbeddingConfig(vaultDir)
	if cfg.Provider != "vertex" {
		t.Errorf("Provider = %q, want vertex (from global)", cfg.Provider)
	}
	if cfg.Project != "global-project" {
		t.Errorf("Project = %q, want global-project", cfg.Project)
	}
}

func TestReadEmbeddingConfig_VaultOverridesGlobal(t *testing.T) {
	// Vault config with ollama.
	vaultDir := t.TempDir()
	vaultYAML := `embedding:
  provider: ollama
  model: vault-model
`
	if err := os.WriteFile(filepath.Join(vaultDir, "config.yaml"), []byte(vaultYAML), 0644); err != nil {
		t.Fatalf("write vault config: %v", err)
	}

	// Global config with vertex — should be ignored.
	globalDir := filepath.Join(t.TempDir(), "dewey")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	globalYAML := `embedding:
  provider: vertex
  model: global-model
  project: global-project
  region: us-central1
`
	if err := os.WriteFile(filepath.Join(globalDir, "config.yaml"), []byte(globalYAML), 0644); err != nil {
		t.Fatalf("write global config: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Dir(globalDir))

	cfg := ReadEmbeddingConfig(vaultDir)
	if cfg.Provider != "ollama" {
		t.Errorf("Provider = %q, want ollama (vault overrides global)", cfg.Provider)
	}
	if cfg.Model != "vault-model" {
		t.Errorf("Model = %q, want vault-model", cfg.Model)
	}
}
