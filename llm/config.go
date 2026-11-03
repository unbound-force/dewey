package llm

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

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

// synthesisTimeout decodes synthesis.timeout from either a duration string
// (for example "300s") or an integer number of seconds. Parse failures are
// captured on the value instead of failing the whole file: one malformed
// optional field must not discard every other synthesis setting. Callers
// report the captured error via Err and fall back to the provider default.
type synthesisTimeout struct {
	duration time.Duration
	err      error
}

// UnmarshalYAML implements yaml.Unmarshaler. It never returns an error for a
// malformed value; the parse error is recorded on the value for use-site
// reporting so sibling config fields still load.
func (t *synthesisTimeout) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		if value.Tag == "!!int" {
			secs, err := strconv.Atoi(value.Value)
			if err != nil {
				t.err = fmt.Errorf("invalid synthesis.timeout %q: %w", value.Value, err)
				return nil
			}
			if secs < 0 {
				t.err = fmt.Errorf("synthesis.timeout must not be negative")
				return nil
			}
			t.duration = time.Duration(secs) * time.Second
			return nil
		}
		d, err := time.ParseDuration(value.Value)
		if err != nil {
			t.err = fmt.Errorf("invalid synthesis.timeout %q: %w", value.Value, err)
			return nil
		}
		t.duration = d
		return nil
	default:
		t.err = fmt.Errorf("synthesis.timeout must be a duration string or integer seconds")
		return nil
	}
}

// Duration returns the decoded duration. It is zero when the value was
// malformed or absent; providers apply their own default for zero timeouts.
func (t synthesisTimeout) Duration() time.Duration { return t.duration }

// Err returns the parse error captured during unmarshalling, if any.
func (t synthesisTimeout) Err() error { return t.err }

// synthesisTimeoutValue converts the tolerant timeout field into a
// ProviderConfig value, warning once when the configured value was malformed
// so only the timeout falls back to the provider default.
func synthesisTimeoutValue(t synthesisTimeout) time.Duration {
	if err := t.Err(); err != nil {
		logger.Warn("ignoring invalid synthesis.timeout, using provider default", "err", err)
	}
	return t.Duration()
}

// configFile represents the structure of config.yaml
// relevant to synthesis configuration.
type configFile struct {
	Synthesis struct {
		Provider  string           `yaml:"provider"`
		Model     string           `yaml:"model"`
		Endpoint  string           `yaml:"endpoint"`
		Project   string           `yaml:"project"`
		Region    string           `yaml:"region"`
		APIKey    string           `yaml:"api_key"`
		Timeout   synthesisTimeout `yaml:"timeout"`
		MaxTokens int              `yaml:"max_tokens"`
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
			Provider:  cfg.Synthesis.Provider,
			Model:     cfg.Synthesis.Model,
			Endpoint:  cfg.Synthesis.Endpoint,
			Project:   cfg.Synthesis.Project,
			Region:    cfg.Synthesis.Region,
			APIKey:    cfg.Synthesis.APIKey,
			Timeout:   synthesisTimeoutValue(cfg.Synthesis.Timeout),
			MaxTokens: cfg.Synthesis.MaxTokens,
		}
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
		return ProviderConfig{
			Provider:  globalCfg.Synthesis.Provider,
			Model:     globalCfg.Synthesis.Model,
			Endpoint:  globalCfg.Synthesis.Endpoint,
			Project:   globalCfg.Synthesis.Project,
			Region:    globalCfg.Synthesis.Region,
			APIKey:    globalCfg.Synthesis.APIKey,
			Timeout:   synthesisTimeoutValue(globalCfg.Synthesis.Timeout),
			MaxTokens: globalCfg.Synthesis.MaxTokens,
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
	return ProviderConfig{
		Provider: "ollama",
		Model:    model,
		Endpoint: ResolveSynthesisEndpoint(),
	}
}
