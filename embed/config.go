package embed

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultOllamaEndpoint is the default Ollama API endpoint.
// All code referencing the default endpoint should use this constant
// instead of hardcoding the URL string.
const DefaultOllamaEndpoint = "http://localhost:11434"

// ResolveOllamaEndpoint resolves the Ollama endpoint from environment
// variables with the following precedence (highest to lowest):
//  1. DEWEY_EMBEDDING_ENDPOINT (app-specific override)
//  2. OLLAMA_HOST (ecosystem standard)
//  3. DefaultOllamaEndpoint constant
//
// If the resolved value has no URL scheme (no "://"), "http://" is prepended.
// Empty strings are treated as unset.
func ResolveOllamaEndpoint() string {
	if ep := os.Getenv("DEWEY_EMBEDDING_ENDPOINT"); ep != "" {
		return normalizeEndpoint(ep)
	}
	return resolveOllamaHostFallback()
}

// resolveOllamaHostFallback resolves the Ollama endpoint from the
// ecosystem-standard OLLAMA_HOST env var, falling back to the default.
// This is used when DEWEY_EMBEDDING_ENDPOINT is not set and config.yaml
// does not provide an endpoint.
func resolveOllamaHostFallback() string {
	if host := os.Getenv("OLLAMA_HOST"); host != "" {
		return normalizeEndpoint(host)
	}
	return DefaultOllamaEndpoint
}

// normalizeEndpoint prepends "http://" if the endpoint has no URL scheme.
func normalizeEndpoint(ep string) string {
	if !strings.Contains(ep, "://") {
		return "http://" + ep
	}
	return ep
}

// globalConfigDir returns the path to the global dewey config directory.
// Uses $XDG_CONFIG_HOME/dewey if set, otherwise ~/.config/dewey.
func globalConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "dewey")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "dewey")
}

// configFile represents the structure of config.yaml
// relevant to embedding configuration.
type configFile struct {
	Embedding struct {
		Provider string `yaml:"provider"`
		Model    string `yaml:"model"`
		Endpoint string `yaml:"endpoint"`
		Project  string `yaml:"project"`
		Region   string `yaml:"region"`
	} `yaml:"embedding"`
}

// readConfigFile attempts to parse a config.yaml at the given directory.
// Returns nil if the file doesn't exist or can't be parsed.
func readConfigFile(dir string) *configFile {
	if dir == "" {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		return nil
	}
	var cfg configFile
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		embedLogger.Warn("failed to parse config.yaml, using defaults", "path", filepath.Join(dir, "config.yaml"), "err", err)
		return nil
	}
	return &cfg
}

// ReadEmbeddingConfig reads the embedding provider configuration.
//
// Endpoint precedence (highest to lowest):
//  1. DEWEY_EMBEDDING_ENDPOINT env var (app-specific override — always wins)
//  2. Per-vault config: {deweyDir}/config.yaml embedding.endpoint
//  3. Global config: ~/.config/dewey/config.yaml embedding.endpoint
//  4. OLLAMA_HOST env var (ecosystem standard fallback)
//  5. DefaultOllamaEndpoint constant
func ReadEmbeddingConfig(deweyDir string) ProviderConfig {
	// Try per-vault config first, then global.
	cfg := readConfigFile(deweyDir)
	if cfg == nil || (cfg.Embedding.Provider == "" && cfg.Embedding.Model == "") {
		cfg = readConfigFile(globalConfigDir())
	}

	if cfg == nil {
		return embedConfigFromEnv()
	}

	pc := ProviderConfig{
		Provider: cfg.Embedding.Provider,
		Model:    cfg.Embedding.Model,
		Endpoint: cfg.Embedding.Endpoint,
		Project:  cfg.Embedding.Project,
		Region:   cfg.Embedding.Region,
	}

	// Environment variables override config file values (existing behavior).
	if envModel := os.Getenv("DEWEY_EMBEDDING_MODEL"); envModel != "" {
		pc.Model = envModel
	}
	// DEWEY_EMBEDDING_ENDPOINT always overrides config.yaml.
	if envEndpoint := os.Getenv("DEWEY_EMBEDDING_ENDPOINT"); envEndpoint != "" {
		pc.Endpoint = envEndpoint
	}

	// If no endpoint was set by config.yaml or DEWEY_EMBEDDING_ENDPOINT,
	// fall back to the OLLAMA_HOST env var chain (OLLAMA_HOST > default).
	// DEWEY_EMBEDDING_ENDPOINT was already checked above, so skip it here.
	if pc.Endpoint == "" {
		pc.Endpoint = resolveOllamaHostFallback()
	}

	// Default provider to ollama if not specified.
	if pc.Provider == "" {
		pc.Provider = "ollama"
	}

	return pc
}

// embedConfigFromEnv builds an embedding config from environment variables.
// Uses ResolveOllamaEndpoint() for the endpoint fallback chain.
func embedConfigFromEnv() ProviderConfig {
	model := os.Getenv("DEWEY_EMBEDDING_MODEL")
	if model == "" {
		model = "granite-embedding:30m"
	}
	return ProviderConfig{
		Provider: "ollama",
		Model:    model,
		Endpoint: ResolveOllamaEndpoint(),
	}
}
