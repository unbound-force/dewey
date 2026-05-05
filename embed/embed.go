// Package embed provides interfaces and implementations for generating
// vector embeddings from text content. Implementations include OllamaEmbedder
// (local inference via Ollama HTTP API) and VertexEmbedder (Google Vertex AI
// prediction API with application-default credentials).
package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Embedder generates vector embeddings from text. Implementations must be
// safe for concurrent use. The interface abstracts the embedding provider,
// enabling testing with mock implementations and future provider swaps.
type Embedder interface {
	// Embed generates a vector embedding for the given text.
	// Returns a float32 slice representing the embedding vector.
	Embed(ctx context.Context, text string) ([]float32, error)

	// EmbedBatch generates vector embeddings for multiple texts in a single request.
	// Returns one float32 slice per input text, in the same order.
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)

	// Available reports whether the embedding model is loaded and ready.
	// Returns false if the provider is unreachable or the model is not pulled.
	Available() bool

	// ModelID returns the model identifier used for embeddings.
	ModelID() string
}

// OllamaEmbedder implements Embedder using Ollama's HTTP API.
// It calls POST /api/embed for embedding generation and GET /api/tags
// for model availability checks.
//
// Design decision: Uses standard net/http instead of an SDK dependency
// because the Ollama API is simple (2 endpoints) and the Embedder interface
// allows swapping implementations without changing callers.
type OllamaEmbedder struct {
	baseURL string
	model   string
	client  *http.Client

	// Cache model availability to avoid repeated HTTP calls.
	// Invalidated periodically (every 30 seconds).
	mu            sync.RWMutex
	available     bool
	lastCheck     time.Time
	checkInterval time.Duration
}

// NewOllamaEmbedder creates an OllamaEmbedder that connects to the Ollama
// API at baseURL (e.g., "http://localhost:11434") using the specified model
// (e.g., "granite-embedding:30m"). Returns a ready-to-use embedder with
// a 30-second HTTP timeout and 30-second availability cache interval.
//
// The returned embedder is safe for concurrent use. Call [OllamaEmbedder.Available]
// to check if the model is loaded before calling [OllamaEmbedder.Embed].
func NewOllamaEmbedder(baseURL, model string) *OllamaEmbedder {
	return &OllamaEmbedder{
		baseURL: baseURL,
		model:   model,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		checkInterval: 30 * time.Second,
	}
}

// embedRequest is the JSON body sent to POST /api/embed.
type embedRequest struct {
	Model string `json:"model"`
	Input any    `json:"input"` // string or []string
}

// embedResponse is the JSON response from POST /api/embed.
type embedResponse struct {
	Embeddings [][]float64 `json:"embeddings"`
}

// tagsResponse is the JSON response from GET /api/tags.
type tagsResponse struct {
	Models []tagModel `json:"models"`
}

// tagModel represents a single model entry in the tags response.
type tagModel struct {
	Name string `json:"name"`
}

// Embed generates a vector embedding for the given text by calling
// Ollama's POST /api/embed endpoint. Returns the float32 embedding
// vector for the input text. Returns an error if the HTTP request fails,
// Ollama returns a non-200 status, the response cannot be parsed, or
// the response contains no embeddings.
func (o *OllamaEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	vectors, err := o.doEmbed(ctx, text)
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, fmt.Errorf("ollama returned empty embeddings")
	}
	return vectors[0], nil
}

// EmbedBatch generates vector embeddings for multiple texts in a single
// request by passing an array of strings to the Ollama API. Returns one
// float32 vector per input text in the same order. Returns (nil, nil) if
// texts is empty. Returns an error if the HTTP request fails or the
// response cannot be parsed.
func (o *OllamaEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	return o.doEmbed(ctx, texts)
}

// Available reports whether the configured embedding model is available
// in the Ollama instance by querying GET /api/tags. Returns false if
// Ollama is unreachable, returns a non-200 status, or the model is not
// in the list. Caches the result for 30 seconds to avoid excessive HTTP
// calls. Safe for concurrent use.
func (o *OllamaEmbedder) Available() bool {
	o.mu.RLock()
	if time.Since(o.lastCheck) < o.checkInterval {
		avail := o.available
		o.mu.RUnlock()
		return avail
	}
	o.mu.RUnlock()

	// Cache expired — re-check.
	o.mu.Lock()
	defer o.mu.Unlock()

	// Double-check after acquiring write lock (another goroutine may have updated).
	if time.Since(o.lastCheck) < o.checkInterval {
		return o.available
	}

	o.available = o.checkModelAvailable()
	o.lastCheck = time.Now()
	return o.available
}

// ModelID returns the model identifier string (e.g., "granite-embedding:30m")
// used for embedding generation and storage lookups.
func (o *OllamaEmbedder) ModelID() string {
	return o.model
}

// doEmbed performs the actual HTTP call to POST /api/embed.
// The input parameter can be a string (single) or []string (batch).
func (o *OllamaEmbedder) doEmbed(ctx context.Context, input any) ([][]float32, error) {
	reqBody := embedRequest{
		Model: o.model,
		Input: input,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal embed request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create embed request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama embed request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Cap response body to 50MB to prevent unbounded memory allocation
	// from unexpectedly large embedding responses.
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 50*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read embed response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama embed returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var embedResp embedResponse
	if err := json.Unmarshal(respBody, &embedResp); err != nil {
		return nil, fmt.Errorf("parse embed response: %w", err)
	}

	// Convert float64 → float32 for storage efficiency.
	// Ollama returns float64 but embedding precision doesn't require it.
	result := make([][]float32, len(embedResp.Embeddings))
	for i, vec := range embedResp.Embeddings {
		f32 := make([]float32, len(vec))
		for j, v := range vec {
			f32[j] = float32(v)
		}
		result[i] = f32
	}

	return result, nil
}

// checkModelAvailable queries GET /api/tags to see if the configured
// model is available in the Ollama instance.
func (o *OllamaEmbedder) checkModelAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.baseURL+"/api/tags", nil)
	if err != nil {
		return false
	}

	resp, err := o.client.Do(req)
	if err != nil {
		return false // Ollama not running or unreachable.
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	// Cap response body to 50MB to prevent unbounded memory allocation.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 50*1024*1024))
	if err != nil {
		return false
	}

	var tags tagsResponse
	if err := json.Unmarshal(body, &tags); err != nil {
		return false
	}

	for _, m := range tags.Models {
		if m.Name == o.model {
			return true
		}
	}
	return false
}
