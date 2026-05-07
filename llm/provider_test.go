package llm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewSynthesizerFromConfig_Ollama(t *testing.T) {
	cfg := ProviderConfig{
		Provider: "ollama",
		Model:    "llama3.2:3b",
		Endpoint: "http://localhost:11434",
	}
	s, err := NewSynthesizerFromConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.ModelID() != "llama3.2:3b" {
		t.Errorf("ModelID = %q, want llama3.2:3b", s.ModelID())
	}
}

func TestNewSynthesizerFromConfig_EmptyDefaultsToOllama(t *testing.T) {
	cfg := ProviderConfig{
		Model: "llama3.2:3b",
	}
	s, err := NewSynthesizerFromConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := s.(*OllamaSynthesizer); !ok {
		t.Errorf("expected *OllamaSynthesizer, got %T", s)
	}
}

func TestNewSynthesizerFromConfig_Vertex(t *testing.T) {
	cfg := ProviderConfig{
		Provider: "vertex",
		Model:    "claude-sonnet-4-6",
		Project:  "my-project",
		Region:   "us-east5",
	}
	s, err := NewSynthesizerFromConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.ModelID() != "claude-sonnet-4-6" {
		t.Errorf("ModelID = %q, want claude-sonnet-4-6", s.ModelID())
	}
}

func TestNewSynthesizerFromConfig_VertexMissingProject(t *testing.T) {
	cfg := ProviderConfig{
		Provider: "vertex",
		Model:    "claude-sonnet-4-6",
		Region:   "us-east5",
	}
	_, err := NewSynthesizerFromConfig(cfg)
	if err == nil {
		t.Fatal("expected error for missing project")
	}
	if !strings.Contains(err.Error(), "project") {
		t.Errorf("error = %q, want to contain 'project'", err.Error())
	}
}

func TestNewSynthesizerFromConfig_VertexMissingRegion(t *testing.T) {
	cfg := ProviderConfig{
		Provider: "vertex",
		Model:    "claude-sonnet-4-6",
		Project:  "my-project",
	}
	_, err := NewSynthesizerFromConfig(cfg)
	if err == nil {
		t.Fatal("expected error for missing region")
	}
	if !strings.Contains(err.Error(), "region") {
		t.Errorf("error = %q, want to contain 'region'", err.Error())
	}
}

func TestNewSynthesizerFromConfig_UnknownProvider(t *testing.T) {
	cfg := ProviderConfig{
		Provider: "unsupported",
	}
	_, err := NewSynthesizerFromConfig(cfg)
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("error = %q, want to contain 'unsupported'", err.Error())
	}
}

func TestReadSynthesisConfig_FromFile(t *testing.T) {
	dir := t.TempDir()
	configYAML := `synthesis:
  provider: vertex
  model: claude-sonnet-4-6
  project: my-project
  region: us-east5
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg := ReadSynthesisConfig(dir)
	if cfg.Provider != "vertex" {
		t.Errorf("Provider = %q, want vertex", cfg.Provider)
	}
	if cfg.Model != "claude-sonnet-4-6" {
		t.Errorf("Model = %q, want claude-sonnet-4-6", cfg.Model)
	}
	if cfg.Project != "my-project" {
		t.Errorf("Project = %q, want my-project", cfg.Project)
	}
}

func TestReadSynthesisConfig_BackwardCompatible(t *testing.T) {
	dir := t.TempDir()
	configYAML := `compile_model: llama3.2:3b
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg := ReadSynthesisConfig(dir)
	if cfg.Provider != "ollama" {
		t.Errorf("Provider = %q, want ollama", cfg.Provider)
	}
	if cfg.Model != "llama3.2:3b" {
		t.Errorf("Model = %q, want llama3.2:3b", cfg.Model)
	}
}

func TestReadSynthesisConfig_EnvFallback(t *testing.T) {
	dir := t.TempDir()
	// Isolate from real global config.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	// No config file.
	t.Setenv("DEWEY_GENERATION_MODEL", "mistral:latest")

	cfg := ReadSynthesisConfig(dir)
	if cfg.Provider != "ollama" {
		t.Errorf("Provider = %q, want ollama", cfg.Provider)
	}
	if cfg.Model != "mistral:latest" {
		t.Errorf("Model = %q, want mistral:latest (from env)", cfg.Model)
	}
}

func TestReadSynthesisConfig_NoFileNoEnv(t *testing.T) {
	dir := t.TempDir()
	// Isolate from real global config.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	// No config, no env — should return zero config.
	cfg := ReadSynthesisConfig(dir)
	if cfg.Model != "" {
		t.Errorf("Model = %q, want empty (no config)", cfg.Model)
	}
}

func TestReadSynthesisConfig_GlobalFallback(t *testing.T) {
	vaultDir := t.TempDir()

	globalDir := filepath.Join(t.TempDir(), "dewey")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	globalYAML := `synthesis:
  provider: vertex
  model: claude-sonnet-4-6
  project: global-project
  region: us-east5
`
	if err := os.WriteFile(filepath.Join(globalDir, "config.yaml"), []byte(globalYAML), 0644); err != nil {
		t.Fatalf("write global config: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Dir(globalDir))

	cfg := ReadSynthesisConfig(vaultDir)
	if cfg.Provider != "vertex" {
		t.Errorf("Provider = %q, want vertex (from global)", cfg.Provider)
	}
	if cfg.Model != "claude-sonnet-4-6" {
		t.Errorf("Model = %q, want claude-sonnet-4-6", cfg.Model)
	}
}

func TestReadSynthesisConfig_VaultOverridesGlobal(t *testing.T) {
	vaultDir := t.TempDir()
	vaultYAML := `synthesis:
  provider: ollama
  model: llama3.2:3b
`
	if err := os.WriteFile(filepath.Join(vaultDir, "config.yaml"), []byte(vaultYAML), 0644); err != nil {
		t.Fatalf("write vault config: %v", err)
	}

	globalDir := filepath.Join(t.TempDir(), "dewey")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	globalYAML := `synthesis:
  provider: vertex
  model: claude-sonnet-4-6
  project: global-project
  region: us-east5
`
	if err := os.WriteFile(filepath.Join(globalDir, "config.yaml"), []byte(globalYAML), 0644); err != nil {
		t.Fatalf("write global config: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Dir(globalDir))

	cfg := ReadSynthesisConfig(vaultDir)
	if cfg.Provider != "ollama" {
		t.Errorf("Provider = %q, want ollama (vault overrides global)", cfg.Provider)
	}
	if cfg.Model != "llama3.2:3b" {
		t.Errorf("Model = %q, want llama3.2:3b", cfg.Model)
	}
}
