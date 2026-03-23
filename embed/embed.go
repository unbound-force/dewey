// Package embed provides interfaces and implementations for generating
// vector embeddings from text content. The primary implementation uses
// Ollama's HTTP API for local model inference.
package embed

import "context"

// Embedder generates vector embeddings from text. Implementations must be
// safe for concurrent use. The interface abstracts the embedding provider,
// enabling testing with mock implementations and future provider swaps.
type Embedder interface {
	// Embed generates a vector embedding for the given text.
	// Returns a float32 slice representing the embedding vector.
	Embed(ctx context.Context, text string) ([]float32, error)

	// Available reports whether the embedding model is loaded and ready.
	// Returns false if the provider is unreachable or the model is not pulled.
	Available() bool
}
