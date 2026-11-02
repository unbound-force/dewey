package llm

import (
	"fmt"
	"os"
	"time"

	"github.com/unbound-force/dewey/v3/embed"
)

// ProviderConfig holds the configuration for a synthesis provider.
// Each provider type uses a subset of these fields.
type ProviderConfig struct {
	// Provider selects the synthesis backend: "ollama", "vertex", or
	// "openai-compatible". Defaults to "ollama" if empty.
	Provider string

	// Model is the model identifier (e.g., "llama3.2:3b",
	// "claude-sonnet-4-6", "rits/zai-org/glm-5-3").
	Model string

	// Endpoint is the base URL for the provider API.
	// Required for ollama (defaults to the local endpoint) and for
	// openai-compatible. Ignored for vertex.
	Endpoint string

	// Project is the GCP project ID. Required for vertex only.
	Project string

	// Region is the GCP region (e.g., "us-east5"). Required for vertex only.
	Region string

	// APIKey is the credential for openai-compatible providers. The
	// DEWEY_SYNTHESIS_API_KEY environment variable takes precedence over
	// this value. The key is never logged.
	APIKey string

	// Timeout bounds a single synthesis request for openai-compatible
	// providers. Zero means the provider default (300s).
	Timeout time.Duration

	// MaxTokens is the output token budget for openai-compatible
	// providers. Zero means the provider default (16000).
	MaxTokens int
}

// ResolveSynthesisAPIKey returns the API key for an OpenAI-compatible
// provider. DEWEY_SYNTHESIS_API_KEY wins over the configured value so
// operators can inject a secret without editing config.yaml.
func ResolveSynthesisAPIKey(configured string) string {
	if key := os.Getenv("DEWEY_SYNTHESIS_API_KEY"); key != "" {
		return key
	}
	return configured
}

// NewSynthesizerFromConfig creates a Synthesizer from the given provider configuration.
// Returns an error if the provider is unknown or required fields are missing.
func NewSynthesizerFromConfig(cfg ProviderConfig) (Synthesizer, error) {
	switch cfg.Provider {
	case "ollama", "":
		endpoint := cfg.Endpoint
		if endpoint == "" {
			endpoint = embed.DefaultOllamaEndpoint
		}
		return NewOllamaSynthesizer(endpoint, cfg.Model), nil
	case "vertex":
		if cfg.Project == "" {
			return nil, fmt.Errorf("vertex synthesis provider requires project")
		}
		if cfg.Region == "" {
			return nil, fmt.Errorf("vertex synthesis provider requires region")
		}
		return NewVertexSynthesizer(cfg.Project, cfg.Region, cfg.Model)
	case "openai-compatible":
		return NewOpenAICompatibleSynthesizer(
			cfg.Endpoint,
			cfg.Model,
			ResolveSynthesisAPIKey(cfg.APIKey),
			cfg.Timeout,
			cfg.MaxTokens,
		)
	default:
		return nil, fmt.Errorf("unknown synthesis provider: %q (supported: ollama, vertex, openai-compatible)", cfg.Provider)
	}
}
