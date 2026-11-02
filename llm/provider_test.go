package llm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unbound-force/dewey/v3/embed"
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

	// Isolate env vars to ensure only compile_model affects the result.
	t.Setenv("DEWEY_SYNTHESIS_ENDPOINT", "")
	t.Setenv("DEWEY_EMBEDDING_ENDPOINT", "")
	t.Setenv("OLLAMA_HOST", "")

	cfg := ReadSynthesisConfig(dir)
	if cfg.Provider != "ollama" {
		t.Errorf("Provider = %q, want ollama", cfg.Provider)
	}
	if cfg.Model != "llama3.2:3b" {
		t.Errorf("Model = %q, want llama3.2:3b", cfg.Model)
	}
	if cfg.Endpoint != embed.DefaultOllamaEndpoint {
		t.Errorf("Endpoint = %q, want %q (default, since only compile_model is set)", cfg.Endpoint, embed.DefaultOllamaEndpoint)
	}
}

func TestReadSynthesisConfig_EnvFallback(t *testing.T) {
	dir := t.TempDir()
	// Isolate from real global config.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	// Isolate endpoint env vars.
	t.Setenv("DEWEY_SYNTHESIS_ENDPOINT", "")
	t.Setenv("DEWEY_EMBEDDING_ENDPOINT", "")
	t.Setenv("OLLAMA_HOST", "")
	// No config file.
	t.Setenv("DEWEY_GENERATION_MODEL", "mistral:latest")

	cfg := ReadSynthesisConfig(dir)
	if cfg.Provider != "ollama" {
		t.Errorf("Provider = %q, want ollama", cfg.Provider)
	}
	if cfg.Model != "mistral:latest" {
		t.Errorf("Model = %q, want mistral:latest (from env)", cfg.Model)
	}
	if cfg.Endpoint != embed.DefaultOllamaEndpoint {
		t.Errorf("Endpoint = %q, want %q (default, since no synthesis env var is set)", cfg.Endpoint, embed.DefaultOllamaEndpoint)
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

// --- Synthesis Endpoint Resolution Tests ---

func TestResolveSynthesisEndpoint_OverridesAll(t *testing.T) {
	t.Setenv("DEWEY_SYNTHESIS_ENDPOINT", "http://synthesis:9999")
	t.Setenv("OLLAMA_HOST", "http://ollama:2222")
	t.Setenv("DEWEY_EMBEDDING_ENDPOINT", "http://embedding:3333")

	got := ResolveSynthesisEndpoint()
	if got != "http://synthesis:9999" {
		t.Errorf("ResolveSynthesisEndpoint() = %q, want %q", got, "http://synthesis:9999")
	}
}

func TestResolveSynthesisEndpoint_FallsBackToOllamaHost(t *testing.T) {
	t.Setenv("DEWEY_SYNTHESIS_ENDPOINT", "")
	t.Setenv("OLLAMA_HOST", "http://host.docker.internal:11435")
	t.Setenv("DEWEY_EMBEDDING_ENDPOINT", "http://embedding:3333")

	got := ResolveSynthesisEndpoint()
	if got != "http://host.docker.internal:11435" {
		t.Errorf("ResolveSynthesisEndpoint() = %q, want %q", got, "http://host.docker.internal:11435")
	}
}

func TestResolveSynthesisEndpoint_WinsOverOllamaHost(t *testing.T) {
	t.Setenv("DEWEY_SYNTHESIS_ENDPOINT", "http://synthesis:1111")
	t.Setenv("OLLAMA_HOST", "http://ollama:2222")

	got := ResolveSynthesisEndpoint()
	if got != "http://synthesis:1111" {
		t.Errorf("ResolveSynthesisEndpoint() = %q, want %q (DEWEY_SYNTHESIS_ENDPOINT should take precedence)", got, "http://synthesis:1111")
	}
}

func TestResolveSynthesisEndpoint_DefaultsWhenNothingSet(t *testing.T) {
	t.Setenv("DEWEY_SYNTHESIS_ENDPOINT", "")
	t.Setenv("OLLAMA_HOST", "")
	t.Setenv("DEWEY_EMBEDDING_ENDPOINT", "")

	got := ResolveSynthesisEndpoint()
	if got != embed.DefaultOllamaEndpoint {
		t.Errorf("ResolveSynthesisEndpoint() = %q, want %q (default)", got, embed.DefaultOllamaEndpoint)
	}
}

func TestResolveSynthesisEndpoint_EmbeddingEndpointDoesNotAffect(t *testing.T) {
	t.Setenv("DEWEY_SYNTHESIS_ENDPOINT", "")
	t.Setenv("OLLAMA_HOST", "")
	t.Setenv("DEWEY_EMBEDDING_ENDPOINT", "http://embedding-only:5555")

	got := ResolveSynthesisEndpoint()
	if got != embed.DefaultOllamaEndpoint {
		t.Errorf("ResolveSynthesisEndpoint() = %q, want %q (DEWEY_EMBEDDING_ENDPOINT must NOT affect synthesis)", got, embed.DefaultOllamaEndpoint)
	}
}

func TestResolveSynthesisEndpoint_NormalizesNoScheme(t *testing.T) {
	t.Setenv("DEWEY_SYNTHESIS_ENDPOINT", "0.0.0.0:11434")
	t.Setenv("OLLAMA_HOST", "")

	got := ResolveSynthesisEndpoint()
	want := "http://0.0.0.0:11434"
	if got != want {
		t.Errorf("ResolveSynthesisEndpoint() = %q, want %q (should prepend http://)", got, want)
	}
}

func TestResolveSynthesisEndpoint_PreservesHTTPS(t *testing.T) {
	t.Setenv("DEWEY_SYNTHESIS_ENDPOINT", "https://synthesis.internal:11434")
	t.Setenv("OLLAMA_HOST", "")

	got := ResolveSynthesisEndpoint()
	if got != "https://synthesis.internal:11434" {
		t.Errorf("ResolveSynthesisEndpoint() = %q, want %q (HTTPS should be preserved)", got, "https://synthesis.internal:11434")
	}
}

func TestResolveSynthesisEndpoint_EmptyTreatedAsUnset(t *testing.T) {
	t.Setenv("DEWEY_SYNTHESIS_ENDPOINT", "")
	t.Setenv("OLLAMA_HOST", "http://fallback:11434")

	got := ResolveSynthesisEndpoint()
	if got != "http://fallback:11434" {
		t.Errorf("ResolveSynthesisEndpoint() = %q, want %q (empty DEWEY_SYNTHESIS_ENDPOINT should fall back to OLLAMA_HOST)", got, "http://fallback:11434")
	}
}

func TestResolveSynthesisEndpoint_OllamaHostNoSchemeNormalized(t *testing.T) {
	t.Setenv("DEWEY_SYNTHESIS_ENDPOINT", "")
	t.Setenv("OLLAMA_HOST", "0.0.0.0:11434")

	got := ResolveSynthesisEndpoint()
	want := "http://0.0.0.0:11434"
	if got != want {
		t.Errorf("ResolveSynthesisEndpoint() = %q, want %q (OLLAMA_HOST without scheme should be normalized)", got, want)
	}
}

func TestReadSynthesisConfig_ConfigYAMLWinsOverSynthesisEndpoint(t *testing.T) {
	dir := t.TempDir()
	configYAML := `synthesis:
  provider: ollama
  model: llama3.2:3b
  endpoint: http://config-host:11434
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("DEWEY_SYNTHESIS_ENDPOINT", "http://env-host:11434")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg := ReadSynthesisConfig(dir)
	if cfg.Endpoint != "http://config-host:11434" {
		t.Errorf("ReadSynthesisConfig().Endpoint = %q, want %q (config.yaml should win over DEWEY_SYNTHESIS_ENDPOINT)",
			cfg.Endpoint, "http://config-host:11434")
	}
}

// --- OpenAI-compatible provider factory and configuration tests ---

func TestNewSynthesizerFromConfig_OpenAICompatible(t *testing.T) {
	clearSynthesisKeyEnv(t)
	cfg := ProviderConfig{
		Provider: "openai-compatible",
		Model:    "enmaas/rits/zai-org/glm-5-3",
		Endpoint: "https://gateway.example/v1",
	}

	s, err := NewSynthesizerFromConfig(cfg)
	if err != nil {
		t.Fatalf("factory construction must succeed with no API key: %v", err)
	}
	if _, ok := s.(*OpenAICompatibleSynthesizer); !ok {
		t.Fatalf("expected *OpenAICompatibleSynthesizer, got %T", s)
	}
	if s.ModelID() != "enmaas/rits/zai-org/glm-5-3" {
		t.Errorf("ModelID = %q", s.ModelID())
	}
}

func TestNewSynthesizerFromConfig_OpenAICompatibleMissingFields(t *testing.T) {
	clearSynthesisKeyEnv(t)

	_, err := NewSynthesizerFromConfig(ProviderConfig{
		Provider: "openai-compatible",
		Model:    "model",
	})
	if err == nil || !strings.Contains(err.Error(), "synthesis.endpoint") {
		t.Fatalf("missing endpoint must name synthesis.endpoint: %v", err)
	}

	_, err = NewSynthesizerFromConfig(ProviderConfig{
		Provider: "openai-compatible",
		Endpoint: "https://gateway.example/v1",
	})
	if err == nil || !strings.Contains(err.Error(), "synthesis.model") {
		t.Fatalf("missing model must name synthesis.model: %v", err)
	}
}

func TestNewSynthesizerFromConfig_UnknownProviderListsAll(t *testing.T) {
	_, err := NewSynthesizerFromConfig(ProviderConfig{Provider: "unsupported"})
	if err == nil {
		t.Fatal("expected error")
	}
	for _, provider := range []string{"ollama", "vertex", "openai-compatible"} {
		if !strings.Contains(err.Error(), provider) {
			t.Fatalf("unknown-provider error must list %q: %v", provider, err)
		}
	}
}

func TestReadSynthesisConfig_GatewayConfigFirst(t *testing.T) {
	dir := t.TempDir()
	configYAML := `synthesis:
  provider: openai-compatible
  model: gateway-model
  endpoint: http://config-host:8080/v1
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("DEWEY_SYNTHESIS_ENDPOINT", "http://env-host:9090/v1")
	t.Setenv("OLLAMA_HOST", "http://ollama-host:11434")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg := ReadSynthesisConfig(dir)
	if cfg.Provider != "openai-compatible" {
		t.Fatalf("Provider = %q", cfg.Provider)
	}
	if cfg.Endpoint != "http://config-host:8080/v1" {
		t.Fatalf("config.yaml must win for synthesis (config-first): %q", cfg.Endpoint)
	}
}

func TestReadSynthesisConfig_GatewayEnvEndpointFallback(t *testing.T) {
	dir := t.TempDir()
	configYAML := `synthesis:
  provider: openai-compatible
  model: gateway-model
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("DEWEY_SYNTHESIS_ENDPOINT", "http://env-host:9090/v1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg := ReadSynthesisConfig(dir)
	if cfg.Endpoint != "http://env-host:9090/v1" {
		t.Fatalf("gateway endpoint should fall back to DEWEY_SYNTHESIS_ENDPOINT: %q", cfg.Endpoint)
	}
}

func TestReadSynthesisConfig_GatewayDoesNotUseOllamaFallbacks(t *testing.T) {
	dir := t.TempDir()
	configYAML := `synthesis:
  provider: openai-compatible
  model: gateway-model
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("DEWEY_SYNTHESIS_ENDPOINT", "")
	t.Setenv("OLLAMA_HOST", "http://ollama-host:11434")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg := ReadSynthesisConfig(dir)
	if cfg.Endpoint != "" {
		t.Fatalf("gateway endpoint must not fall back to OLLAMA_HOST or localhost: %q", cfg.Endpoint)
	}
}

// TestReadSynthesisConfig_GatewayGlobalConfigEnvFallback exercises the
// global-config branch of resolveGatewaySynthesisConfig: a gateway
// provider declared only in the global config.yaml still gets the
// DEWEY_SYNTHESIS_ENDPOINT fallback (code review L-15).
func TestReadSynthesisConfig_GatewayGlobalConfigEnvFallback(t *testing.T) {
	vaultDir := t.TempDir()

	globalDir := filepath.Join(t.TempDir(), "dewey")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	globalYAML := `synthesis:
  provider: openai-compatible
  model: gateway-model
`
	if err := os.WriteFile(filepath.Join(globalDir, "config.yaml"), []byte(globalYAML), 0644); err != nil {
		t.Fatalf("write global config: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Dir(globalDir))
	t.Setenv("DEWEY_SYNTHESIS_ENDPOINT", "http://env-host:9090/v1")

	cfg := ReadSynthesisConfig(vaultDir)
	if cfg.Provider != "openai-compatible" {
		t.Fatalf("Provider = %q, want openai-compatible (from global)", cfg.Provider)
	}
	if cfg.Endpoint != "http://env-host:9090/v1" {
		t.Fatalf("global gateway endpoint should fall back to DEWEY_SYNTHESIS_ENDPOINT: %q", cfg.Endpoint)
	}
}

// TestHasAPIKeyField_VaultAndGlobal covers both config sources for the
// ignored-key warning probe (FR-024), including the per-vault and global
// branches (code review HIGH-2, L-15).
func TestHasAPIKeyField_VaultAndGlobal(t *testing.T) {
	vaultDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if HasAPIKeyField(vaultDir) {
		t.Fatal("no config anywhere: want false")
	}

	vaultYAML := `synthesis:
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
	globalYAML := `synthesis:
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
