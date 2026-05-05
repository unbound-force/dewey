package llm

import "fmt"

// ProviderConfig holds the configuration for a synthesis provider.
// Each provider type uses a subset of these fields.
type ProviderConfig struct {
	// Provider selects the synthesis backend: "ollama" or "vertex".
	// Defaults to "ollama" if empty.
	Provider string

	// Model is the model identifier (e.g., "llama3.2:3b",
	// "claude-sonnet-4-6").
	Model string

	// Endpoint is the base URL for the provider API.
	// Required for ollama. Ignored for vertex.
	Endpoint string

	// Project is the GCP project ID. Required for vertex only.
	Project string

	// Region is the GCP region (e.g., "us-east5"). Required for vertex only.
	Region string
}

// NewSynthesizerFromConfig creates a Synthesizer from the given provider configuration.
// Returns an error if the provider is unknown or required fields are missing.
func NewSynthesizerFromConfig(cfg ProviderConfig) (Synthesizer, error) {
	switch cfg.Provider {
	case "ollama", "":
		endpoint := cfg.Endpoint
		if endpoint == "" {
			endpoint = "http://localhost:11434"
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
	default:
		return nil, fmt.Errorf("unknown synthesis provider: %q (supported: ollama, vertex)", cfg.Provider)
	}
}
