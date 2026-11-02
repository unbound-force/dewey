package embed

import "fmt"

// ProviderConfig holds the configuration for an embedding provider.
// Each provider type uses a subset of these fields.
type ProviderConfig struct {
	// Provider selects the embedding backend: "ollama", "vertex", or
	// "openai-compatible". Defaults to "ollama" if empty.
	Provider string

	// Model is the model identifier (e.g., "granite-embedding:30m",
	// "text-embedding-005", "openrouter/amazon/titan-embed-text-v2").
	Model string

	// Endpoint is the base URL for the provider API.
	// Required for ollama and openai-compatible. Ignored for vertex.
	Endpoint string

	// Project is the GCP project ID. Required for vertex only.
	Project string

	// Region is the GCP region (e.g., "us-central1"). Required for vertex only.
	Region string

	// MaxChunkChars is the maximum number of characters per embedding chunk.
	// Defaults to DefaultMaxChunkChars (12288) if zero.
	MaxChunkChars int
}

// NewEmbedderFromConfig creates an Embedder from the given provider configuration.
// Returns an error if the provider is unknown or required fields are missing.
func NewEmbedderFromConfig(cfg ProviderConfig) (Embedder, error) {
	switch cfg.Provider {
	case "ollama", "":
		endpoint := cfg.Endpoint
		if endpoint == "" {
			endpoint = DefaultOllamaEndpoint
		}
		return NewOllamaEmbedder(endpoint, cfg.Model), nil
	case "vertex":
		if cfg.Project == "" {
			return nil, fmt.Errorf("vertex embedding provider requires project")
		}
		if cfg.Region == "" {
			return nil, fmt.Errorf("vertex embedding provider requires region")
		}
		return NewVertexEmbedder(cfg.Project, cfg.Region, cfg.Model)
	case "openai-compatible":
		return NewOpenAICompatibleEmbedder(cfg.Endpoint, cfg.Model)
	default:
		return nil, fmt.Errorf("unknown embedding provider: %q (supported: ollama, vertex, openai-compatible)", cfg.Provider)
	}
}
