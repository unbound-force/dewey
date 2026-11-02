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

// --- OpenAI-compatible provider factory and configuration tests ---

func TestNewEmbedderFromConfig_OpenAICompatible(t *testing.T) {
	clearEmbeddingKeyEnv(t)
	cfg := ProviderConfig{
		Provider: "openai-compatible",
		Model:    "openrouter/amazon/titan-embed-text-v2",
		Endpoint: "https://openrouter.ai/api/v1",
	}

	e, err := NewEmbedderFromConfig(cfg)
	if err != nil {
		t.Fatalf("factory construction must succeed with no API key: %v", err)
	}
	if _, ok := e.(*OpenAICompatibleEmbedder); !ok {
		t.Fatalf("expected *OpenAICompatibleEmbedder, got %T", e)
	}
	if e.ModelID() != "openrouter/amazon/titan-embed-text-v2" {
		t.Errorf("ModelID = %q", e.ModelID())
	}
}

func TestNewEmbedderFromConfig_OpenAICompatibleMissingFields(t *testing.T) {
	clearEmbeddingKeyEnv(t)

	_, err := NewEmbedderFromConfig(ProviderConfig{
		Provider: "openai-compatible",
		Model:    "model",
	})
	if err == nil || !strings.Contains(err.Error(), "embedding.endpoint") {
		t.Fatalf("missing endpoint must name embedding.endpoint: %v", err)
	}

	_, err = NewEmbedderFromConfig(ProviderConfig{
		Provider: "openai-compatible",
		Endpoint: "https://gateway.example/v1",
	})
	if err == nil || !strings.Contains(err.Error(), "embedding.model") {
		t.Fatalf("missing model must name embedding.model: %v", err)
	}
}

func TestNewEmbedderFromConfig_UnknownProviderListsAll(t *testing.T) {
	_, err := NewEmbedderFromConfig(ProviderConfig{Provider: "unsupported"})
	if err == nil {
		t.Fatal("expected error")
	}
	for _, provider := range []string{"ollama", "vertex", "openai-compatible"} {
		if !strings.Contains(err.Error(), provider) {
			t.Fatalf("unknown-provider error must list %q: %v", provider, err)
		}
	}
}

func TestReadEmbeddingConfig_GatewayEnvFirst(t *testing.T) {
	dir := t.TempDir()
	configYAML := `embedding:
  provider: openai-compatible
  model: gateway-model
  endpoint: http://config-host:8080/v1
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("DEWEY_EMBEDDING_ENDPOINT", "http://env-host:9090/v1")
	t.Setenv("OLLAMA_HOST", "http://ollama-host:11434")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg := ReadEmbeddingConfig(dir)
	if cfg.Provider != "openai-compatible" {
		t.Fatalf("Provider = %q", cfg.Provider)
	}
	if cfg.Endpoint != "http://env-host:9090/v1" {
		t.Fatalf("DEWEY_EMBEDDING_ENDPOINT must win for embedding (env-first): %q", cfg.Endpoint)
	}
}

func TestReadEmbeddingConfig_GatewayDoesNotUseOllamaFallbacks(t *testing.T) {
	dir := t.TempDir()
	configYAML := `embedding:
  provider: openai-compatible
  model: gateway-model
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("DEWEY_EMBEDDING_ENDPOINT", "")
	t.Setenv("OLLAMA_HOST", "http://ollama-host:11434")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg := ReadEmbeddingConfig(dir)
	if cfg.Endpoint != "" {
		t.Fatalf("gateway endpoint must not fall back to OLLAMA_HOST or localhost: %q", cfg.Endpoint)
	}
}

func TestReadEmbeddingConfig_GatewayFromGlobalConfig(t *testing.T) {
	vaultDir := t.TempDir()

	globalDir := filepath.Join(t.TempDir(), "dewey")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	globalYAML := `embedding:
  provider: openai-compatible
  model: global-gateway-model
  endpoint: https://global-gateway.example/v1
`
	if err := os.WriteFile(filepath.Join(globalDir, "config.yaml"), []byte(globalYAML), 0644); err != nil {
		t.Fatalf("write global config: %v", err)
	}
	t.Setenv("DEWEY_EMBEDDING_ENDPOINT", "")
	t.Setenv("OLLAMA_HOST", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Dir(globalDir))

	cfg := ReadEmbeddingConfig(vaultDir)
	if cfg.Provider != "openai-compatible" {
		t.Fatalf("Provider = %q, want openai-compatible (from global)", cfg.Provider)
	}
	if cfg.Endpoint != "https://global-gateway.example/v1" {
		t.Fatalf("Endpoint = %q, want the global gateway endpoint", cfg.Endpoint)
	}
}

// TestHasAPIKeyField_VaultAndGlobal covers both config sources for the
// ignored-key warning probe (FR-024), including the global true branch
// (code review L-15).
func TestHasAPIKeyField_VaultAndGlobal(t *testing.T) {
	vaultDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if HasAPIKeyField(vaultDir) {
		t.Fatal("no config anywhere: want false")
	}

	vaultYAML := `embedding:
  api_key: vault-secret
`
	if err := os.WriteFile(filepath.Join(vaultDir, "config.yaml"), []byte(vaultYAML), 0644); err != nil {
		t.Fatalf("write vault config: %v", err)
	}
	if !HasAPIKeyField(vaultDir) {
		t.Fatal("vault api_key: want true")
	}

	emptyVault := t.TempDir()
	globalDir := filepath.Join(t.TempDir(), "dewey")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	globalYAML := `embedding:
  api_key: global-secret
`
	if err := os.WriteFile(filepath.Join(globalDir, "config.yaml"), []byte(globalYAML), 0644); err != nil {
		t.Fatalf("write global config: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Dir(globalDir))
	if !HasAPIKeyField(emptyVault) {
		t.Fatal("global api_key with empty vault: want true")
	}
}
