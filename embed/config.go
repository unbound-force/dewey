package embed

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

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
		return nil
	}
	return &cfg
}

// ReadEmbeddingConfig reads the embedding provider configuration.
//
// Config precedence (highest to lowest):
//  1. Environment variables: DEWEY_EMBEDDING_MODEL, DEWEY_EMBEDDING_ENDPOINT
//  2. Per-vault config: {deweyDir}/config.yaml
//  3. Global config: ~/.config/dewey/config.yaml
//  4. Defaults: provider=ollama, model=granite-embedding:30m, endpoint=localhost:11434
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
	if envEndpoint := os.Getenv("DEWEY_EMBEDDING_ENDPOINT"); envEndpoint != "" {
		pc.Endpoint = envEndpoint
	}

	// Default provider to ollama if not specified.
	if pc.Provider == "" {
		pc.Provider = "ollama"
	}

	return pc
}

// embedConfigFromEnv builds an embedding config from environment variables.
func embedConfigFromEnv() ProviderConfig {
	model := os.Getenv("DEWEY_EMBEDDING_MODEL")
	if model == "" {
		model = "granite-embedding:30m"
	}
	endpoint := os.Getenv("DEWEY_EMBEDDING_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:11434"
	}
	return ProviderConfig{
		Provider: "ollama",
		Model:    model,
		Endpoint: endpoint,
	}
}
