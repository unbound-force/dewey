package embed

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveOllamaEndpoint_DeweyEndpointOverridesAll(t *testing.T) {
	t.Setenv("DEWEY_EMBEDDING_ENDPOINT", "http://custom:9999")
	t.Setenv("OLLAMA_HOST", "")

	got := ResolveOllamaEndpoint()
	if got != "http://custom:9999" {
		t.Errorf("ResolveOllamaEndpoint() = %q, want %q", got, "http://custom:9999")
	}
}

func TestResolveOllamaEndpoint_FallsBackToOllamaHost(t *testing.T) {
	t.Setenv("DEWEY_EMBEDDING_ENDPOINT", "")
	t.Setenv("OLLAMA_HOST", "http://host.docker.internal:11435")

	got := ResolveOllamaEndpoint()
	if got != "http://host.docker.internal:11435" {
		t.Errorf("ResolveOllamaEndpoint() = %q, want %q", got, "http://host.docker.internal:11435")
	}
}

func TestResolveOllamaEndpoint_DeweyEndpointWinsOverOllamaHost(t *testing.T) {
	t.Setenv("DEWEY_EMBEDDING_ENDPOINT", "http://dewey:1111")
	t.Setenv("OLLAMA_HOST", "http://ollama:2222")

	got := ResolveOllamaEndpoint()
	if got != "http://dewey:1111" {
		t.Errorf("ResolveOllamaEndpoint() = %q, want %q (DEWEY_EMBEDDING_ENDPOINT should take precedence)", got, "http://dewey:1111")
	}
}

func TestResolveOllamaEndpoint_DefaultsWhenNothingSet(t *testing.T) {
	t.Setenv("DEWEY_EMBEDDING_ENDPOINT", "")
	t.Setenv("OLLAMA_HOST", "")

	got := ResolveOllamaEndpoint()
	if got != DefaultOllamaEndpoint {
		t.Errorf("ResolveOllamaEndpoint() = %q, want %q (default)", got, DefaultOllamaEndpoint)
	}
}

func TestResolveOllamaEndpoint_NormalizesOllamaHostWithoutScheme(t *testing.T) {
	t.Setenv("DEWEY_EMBEDDING_ENDPOINT", "")
	t.Setenv("OLLAMA_HOST", "0.0.0.0:11434")

	got := ResolveOllamaEndpoint()
	want := "http://0.0.0.0:11434"
	if got != want {
		t.Errorf("ResolveOllamaEndpoint() = %q, want %q (should prepend http://)", got, want)
	}
}

func TestResolveOllamaEndpoint_PreservesHTTPSOnOllamaHost(t *testing.T) {
	t.Setenv("DEWEY_EMBEDDING_ENDPOINT", "")
	t.Setenv("OLLAMA_HOST", "https://ollama.internal:11434")

	got := ResolveOllamaEndpoint()
	if got != "https://ollama.internal:11434" {
		t.Errorf("ResolveOllamaEndpoint() = %q, want %q (HTTPS should be preserved)", got, "https://ollama.internal:11434")
	}
}

func TestResolveOllamaEndpoint_EmptyOllamaHostTreatedAsUnset(t *testing.T) {
	t.Setenv("DEWEY_EMBEDDING_ENDPOINT", "")
	t.Setenv("OLLAMA_HOST", "")

	got := ResolveOllamaEndpoint()
	if got != DefaultOllamaEndpoint {
		t.Errorf("ResolveOllamaEndpoint() = %q, want %q (empty OLLAMA_HOST treated as unset)", got, DefaultOllamaEndpoint)
	}
}

func TestResolveOllamaEndpoint_NormalizesDeweyEndpointWithoutScheme(t *testing.T) {
	t.Setenv("DEWEY_EMBEDDING_ENDPOINT", "myhost:11434")
	t.Setenv("OLLAMA_HOST", "")

	got := ResolveOllamaEndpoint()
	want := "http://myhost:11434"
	if got != want {
		t.Errorf("ResolveOllamaEndpoint() = %q, want %q (should prepend http:// to DEWEY_EMBEDDING_ENDPOINT)", got, want)
	}
}

// TestReadEmbeddingConfig_ConfigYAMLWinsOverOllamaHost verifies that
// config.yaml embedding.endpoint takes precedence over OLLAMA_HOST when
// DEWEY_EMBEDDING_ENDPOINT is not set.
func TestReadEmbeddingConfig_ConfigYAMLWinsOverOllamaHost(t *testing.T) {
	dir := t.TempDir()
	configYAML := "embedding:\n  model: test-model\n  endpoint: http://config-host:11434\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(configYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("OLLAMA_HOST", "http://env-host:11435")
	t.Setenv("DEWEY_EMBEDDING_ENDPOINT", "")

	cfg := ReadEmbeddingConfig(dir)
	if cfg.Endpoint != "http://config-host:11434" {
		t.Errorf("ReadEmbeddingConfig().Endpoint = %q, want %q (config.yaml should win over OLLAMA_HOST)",
			cfg.Endpoint, "http://config-host:11434")
	}
}

// TestReadEmbeddingConfig_MaxChunkCharsDefault verifies that MaxChunkChars
// defaults to DefaultMaxChunkChars when no config file or env var sets it.
func TestReadEmbeddingConfig_MaxChunkCharsDefault(t *testing.T) {
	dir := t.TempDir()
	// No config.yaml, no DEWEY_CHUNK_MAX_CHARS env var.
	t.Setenv("DEWEY_CHUNK_MAX_CHARS", "")
	t.Setenv("DEWEY_EMBEDDING_ENDPOINT", "")
	t.Setenv("OLLAMA_HOST", "")

	cfg := ReadEmbeddingConfig(dir)
	if cfg.MaxChunkChars != DefaultMaxChunkChars {
		t.Errorf("ReadEmbeddingConfig().MaxChunkChars = %d, want %d (default)",
			cfg.MaxChunkChars, DefaultMaxChunkChars)
	}
}

// TestReadEmbeddingConfig_MaxChunkCharsFromConfig verifies that
// config.yaml embedding.max_chunk_chars is respected.
func TestReadEmbeddingConfig_MaxChunkCharsFromConfig(t *testing.T) {
	dir := t.TempDir()
	configYAML := "embedding:\n  model: test-model\n  max_chunk_chars: 4096\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(configYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("DEWEY_CHUNK_MAX_CHARS", "")
	t.Setenv("DEWEY_EMBEDDING_ENDPOINT", "")
	t.Setenv("OLLAMA_HOST", "")

	cfg := ReadEmbeddingConfig(dir)
	if cfg.MaxChunkChars != 4096 {
		t.Errorf("ReadEmbeddingConfig().MaxChunkChars = %d, want %d (from config.yaml)",
			cfg.MaxChunkChars, 4096)
	}
}

// TestReadEmbeddingConfig_MaxChunkCharsEnvOverridesConfig verifies that
// DEWEY_CHUNK_MAX_CHARS env var takes precedence over config.yaml.
func TestReadEmbeddingConfig_MaxChunkCharsEnvOverridesConfig(t *testing.T) {
	dir := t.TempDir()
	configYAML := "embedding:\n  model: test-model\n  max_chunk_chars: 4096\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(configYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("DEWEY_CHUNK_MAX_CHARS", "2048")
	t.Setenv("DEWEY_EMBEDDING_ENDPOINT", "")
	t.Setenv("OLLAMA_HOST", "")

	cfg := ReadEmbeddingConfig(dir)
	if cfg.MaxChunkChars != 2048 {
		t.Errorf("ReadEmbeddingConfig().MaxChunkChars = %d, want %d (env var should override config.yaml)",
			cfg.MaxChunkChars, 2048)
	}
}

// TestReadEmbeddingConfig_MaxChunkCharsEnvInvalidFallsBack verifies that
// an invalid (non-numeric) DEWEY_CHUNK_MAX_CHARS falls back to the default.
func TestReadEmbeddingConfig_MaxChunkCharsEnvInvalidFallsBack(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DEWEY_CHUNK_MAX_CHARS", "abc")
	t.Setenv("DEWEY_EMBEDDING_ENDPOINT", "")
	t.Setenv("OLLAMA_HOST", "")

	cfg := ReadEmbeddingConfig(dir)
	if cfg.MaxChunkChars != DefaultMaxChunkChars {
		t.Errorf("ReadEmbeddingConfig().MaxChunkChars = %d, want %d (invalid env var should fall back to default)",
			cfg.MaxChunkChars, DefaultMaxChunkChars)
	}
}

// TestReadEmbeddingConfig_MaxChunkCharsEnvZeroFallsBack verifies that
// DEWEY_CHUNK_MAX_CHARS=0 falls back to the default (zero is not a valid chunk size).
func TestReadEmbeddingConfig_MaxChunkCharsEnvZeroFallsBack(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DEWEY_CHUNK_MAX_CHARS", "0")
	t.Setenv("DEWEY_EMBEDDING_ENDPOINT", "")
	t.Setenv("OLLAMA_HOST", "")

	cfg := ReadEmbeddingConfig(dir)
	if cfg.MaxChunkChars != DefaultMaxChunkChars {
		t.Errorf("ReadEmbeddingConfig().MaxChunkChars = %d, want %d (zero env var should fall back to default)",
			cfg.MaxChunkChars, DefaultMaxChunkChars)
	}
}

// TestReadEmbeddingConfig_MaxChunkCharsEnvNegativeFallsBack verifies that
// a negative DEWEY_CHUNK_MAX_CHARS falls back to the default.
func TestReadEmbeddingConfig_MaxChunkCharsEnvNegativeFallsBack(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DEWEY_CHUNK_MAX_CHARS", "-5")
	t.Setenv("DEWEY_EMBEDDING_ENDPOINT", "")
	t.Setenv("OLLAMA_HOST", "")

	cfg := ReadEmbeddingConfig(dir)
	if cfg.MaxChunkChars != DefaultMaxChunkChars {
		t.Errorf("ReadEmbeddingConfig().MaxChunkChars = %d, want %d (negative env var should fall back to default)",
			cfg.MaxChunkChars, DefaultMaxChunkChars)
	}
}
