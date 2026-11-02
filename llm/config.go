package llm

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/unbound-force/dewey/v3/embed"
	"gopkg.in/yaml.v3"
)

// ResolveSynthesisEndpoint resolves the Ollama endpoint for synthesis
// from environment variables with the following precedence (highest to lowest):
//  1. DEWEY_SYNTHESIS_ENDPOINT (app-specific override)
//  2. OLLAMA_HOST (ecosystem standard)
//  3. embed.DefaultOllamaEndpoint constant
//
// If the resolved value has no URL scheme (no "://"), "http://" is prepended.
// Empty strings are treated as unset.
func ResolveSynthesisEndpoint() string {
	if ep := os.Getenv("DEWEY_SYNTHESIS_ENDPOINT"); ep != "" {
		return normalizeSynthesisEndpoint(ep)
	}
	if host := os.Getenv("OLLAMA_HOST"); host != "" {
		return normalizeSynthesisEndpoint(host)
	}
	return embed.DefaultOllamaEndpoint
}

// normalizeSynthesisEndpoint prepends "http://" if the endpoint has no URL scheme.
func normalizeSynthesisEndpoint(ep string) string {
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
// relevant to synthesis configuration.
type configFile struct {
	Synthesis struct {
		Provider string `yaml:"provider"`
		Model    string `yaml:"model"`
		Endpoint string `yaml:"endpoint"`
		Project  string `yaml:"project"`
		Region   string `yaml:"region"`
		// APIKey is parsed only to warn that it is ignored: keys are
		// environment-only (FR-006, FR-024). It never reaches ProviderConfig.
		APIKey string `yaml:"api_key"`
	} `yaml:"synthesis"`

	// Legacy field for backward compatibility.
	CompileModel string `yaml:"compile_model"`
}

// HasAPIKeyField reports whether a synthesis.api_key field appears in the
// per-vault or global config.yaml. The field is ignored by design
// (FR-006); startup and dewey doctor warn about it (FR-024).
//
// This same-schema probe is deliberately mirrored in
// embed.HasAPIKeyField rather than shared through a common package:
// spec 016's configuration-independence decision keeps llm and embed
// decoupled, and each function reads a different YAML section. The
// duplicated config read is the accepted interim trade-off.
func HasAPIKeyField(deweyDir string) bool {
	if cfg := readConfigFile(deweyDir); cfg != nil && cfg.Synthesis.APIKey != "" {
		return true
	}
	if cfg := readConfigFile(globalConfigDir()); cfg != nil && cfg.Synthesis.APIKey != "" {
		return true
	}
	return false
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
		return resolveGatewaySynthesisConfig(ProviderConfig{
			Provider: cfg.Synthesis.Provider,
			Model:    cfg.Synthesis.Model,
			Endpoint: cfg.Synthesis.Endpoint,
			Project:  cfg.Synthesis.Project,
			Region:   cfg.Synthesis.Region,
		})
	}

	// Check per-vault legacy compile_model.
	if cfg != nil && cfg.CompileModel != "" {
		return ProviderConfig{
			Provider: "ollama",
			Model:    cfg.CompileModel,
			Endpoint: ResolveSynthesisEndpoint(),
		}
	}

	// Check global config.
	globalCfg := readConfigFile(globalConfigDir())
	if globalCfg != nil && (globalCfg.Synthesis.Provider != "" || globalCfg.Synthesis.Model != "") {
		return resolveGatewaySynthesisConfig(ProviderConfig{
			Provider: globalCfg.Synthesis.Provider,
			Model:    globalCfg.Synthesis.Model,
			Endpoint: globalCfg.Synthesis.Endpoint,
			Project:  globalCfg.Synthesis.Project,
			Region:   globalCfg.Synthesis.Region,
		})
	}

	return synthConfigFromEnv()
}

// resolveGatewaySynthesisConfig applies the openai-compatible endpoint
// fallback: when config.yaml leaves synthesis.endpoint empty, the
// DEWEY_SYNTHESIS_ENDPOINT environment variable is consulted. For a
// gateway, OLLAMA_HOST and the localhost default never apply (FR-022).
func resolveGatewaySynthesisConfig(cfg ProviderConfig) ProviderConfig {
	if cfg.Provider == "openai-compatible" && strings.TrimSpace(cfg.Endpoint) == "" {
		cfg.Endpoint = os.Getenv("DEWEY_SYNTHESIS_ENDPOINT")
	}
	return cfg
}

// synthConfigFromEnv builds a synthesis config from environment variables.
func synthConfigFromEnv() ProviderConfig {
	model := os.Getenv("DEWEY_GENERATION_MODEL")
	if model == "" {
		return ProviderConfig{}
	}
	return ProviderConfig{
		Provider: "ollama",
		Model:    model,
		Endpoint: ResolveSynthesisEndpoint(),
	}
}
