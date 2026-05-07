package llm

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
// relevant to synthesis configuration.
type configFile struct {
	Synthesis struct {
		Provider string `yaml:"provider"`
		Model    string `yaml:"model"`
		Endpoint string `yaml:"endpoint"`
		Project  string `yaml:"project"`
		Region   string `yaml:"region"`
	} `yaml:"synthesis"`

	// Legacy field for backward compatibility.
	CompileModel string `yaml:"compile_model"`
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
		logger.Warn("failed to parse config.yaml, using defaults", "path", filepath.Join(dir, "config.yaml"), "err", err)
		return nil
	}
	return &cfg
}

// ReadSynthesisConfig reads the synthesis provider configuration.
//
// Config precedence (highest to lowest):
//  1. Per-vault config: {deweyDir}/config.yaml (synthesis section)
//  2. Per-vault legacy: {deweyDir}/config.yaml (compile_model field)
//  3. Global config: ~/.config/dewey/config.yaml
//  4. Environment variable: DEWEY_GENERATION_MODEL
//  5. Zero config (no synthesizer — prompt-only mode)
//
// Environment variables do not override config file values for synthesis
// (unlike embedding), preserving backward compatibility with the legacy
// compile_model path.
func ReadSynthesisConfig(deweyDir string) ProviderConfig {
	cfg := readConfigFile(deweyDir)

	// Check per-vault synthesis section.
	if cfg != nil && (cfg.Synthesis.Provider != "" || cfg.Synthesis.Model != "") {
		return ProviderConfig{
			Provider: cfg.Synthesis.Provider,
			Model:    cfg.Synthesis.Model,
			Endpoint: cfg.Synthesis.Endpoint,
			Project:  cfg.Synthesis.Project,
			Region:   cfg.Synthesis.Region,
		}
	}

	// Check per-vault legacy compile_model.
	if cfg != nil && cfg.CompileModel != "" {
		endpoint := os.Getenv("DEWEY_EMBEDDING_ENDPOINT")
		if endpoint == "" {
			endpoint = "http://localhost:11434"
		}
		return ProviderConfig{
			Provider: "ollama",
			Model:    cfg.CompileModel,
			Endpoint: endpoint,
		}
	}

	// Check global config.
	globalCfg := readConfigFile(globalConfigDir())
	if globalCfg != nil && (globalCfg.Synthesis.Provider != "" || globalCfg.Synthesis.Model != "") {
		return ProviderConfig{
			Provider: globalCfg.Synthesis.Provider,
			Model:    globalCfg.Synthesis.Model,
			Endpoint: globalCfg.Synthesis.Endpoint,
			Project:  globalCfg.Synthesis.Project,
			Region:   globalCfg.Synthesis.Region,
		}
	}

	return synthConfigFromEnv()
}

// synthConfigFromEnv builds a synthesis config from environment variables.
func synthConfigFromEnv() ProviderConfig {
	model := os.Getenv("DEWEY_GENERATION_MODEL")
	if model == "" {
		return ProviderConfig{}
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
