package embed

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/unbound-force/dewey/v3/openaicompat"
)

// OpenAICompatibleEmbedder implements Embedder against an
// OpenAI-compatible gateway (OpenRouter, ENMaas, LiteLLM, vLLM, or a
// self-hosted server).
//
// It sends one POST {endpoint}/embeddings request per batch with the
// configured model passed verbatim, restores input order using each
// response entry's index field, and converts vectors to float32.
// Authentication is environment-only and resolved by openaicompat
// (FR-006, FR-007).
type OpenAICompatibleEmbedder struct {
	client *openaicompat.Client
	model  string
}

// gatewayEmbeddingRequest is the JSON body sent to POST /embeddings.
type gatewayEmbeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

// gatewayEmbeddingResponse is the JSON response from POST /embeddings.
type gatewayEmbeddingResponse struct {
	Data []gatewayEmbedding `json:"data"`
}

// gatewayEmbedding is one vector entry in the response.
type gatewayEmbedding struct {
	Index     int       `json:"index"`
	Embedding []float64 `json:"embedding"`
}

// NewOpenAICompatibleEmbedder validates the required configuration and
// returns an embedding provider for the gateway. A missing endpoint names
// embedding.endpoint and a missing model names embedding.model (FR-009,
// US2-6). A missing API key is allowed so unauthenticated self-hosted
// gateways work (FR-008).
func NewOpenAICompatibleEmbedder(endpoint, model string) (*OpenAICompatibleEmbedder, error) {
	if strings.TrimSpace(endpoint) == "" {
		return nil, fmt.Errorf("openai-compatible embedding provider requires endpoint (embedding.endpoint)")
	}
	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("openai-compatible embedding provider requires model (embedding.model)")
	}

	client, err := openaicompat.NewClient(openaicompat.Config{
		Capability: openaicompat.CapabilityEmbedding,
		Endpoint:   endpoint,
	})
	if err != nil {
		return nil, err
	}
	return &OpenAICompatibleEmbedder{client: client, model: model}, nil
}

// Embed generates a vector for a single text.
func (e *OpenAICompatibleEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	vectors, err := e.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, fmt.Errorf("openai-compatible embedding provider returned no vectors")
	}
	return vectors[0], nil
}

// EmbedBatch generates one vector per input in a single request, in the
// original input order. An empty batch returns (nil, nil) without a
// network call (US2-5).
func (e *OpenAICompatibleEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	payload := gatewayEmbeddingRequest{Model: e.model, Input: texts}
	body, err := e.client.PostJSON(ctx, openaicompat.PathEmbeddings, payload)
	if err != nil {
		return nil, err
	}

	var response gatewayEmbeddingResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, openaicompat.ParseError(openaicompat.CapabilityEmbedding, err)
	}
	return orderVectors(response.Data, len(texts))
}

// orderVectors restores input order by index and converts to float32. A
// count mismatch, duplicate index, or out-of-range index is a descriptive
// error (FR-023, US2-3, US2-4).
func orderVectors(entries []gatewayEmbedding, inputs int) ([][]float32, error) {
	if len(entries) != inputs {
		return nil, fmt.Errorf(
			"openai-compatible embedding provider returned %d vectors for %d inputs",
			len(entries), inputs,
		)
	}

	vectors := make([][]float32, inputs)
	seen := make([]bool, inputs)
	for _, entry := range entries {
		if entry.Index < 0 || entry.Index >= inputs {
			return nil, fmt.Errorf(
				"openai-compatible embedding provider returned index %d out of range for %d inputs",
				entry.Index, inputs,
			)
		}
		if seen[entry.Index] {
			return nil, fmt.Errorf(
				"openai-compatible embedding provider returned duplicate index %d",
				entry.Index,
			)
		}
		seen[entry.Index] = true
		vectors[entry.Index] = toFloat32(entry.Embedding)
	}
	return vectors, nil
}

// toFloat32 converts a float64 vector to float32 for storage efficiency.
func toFloat32(values []float64) []float32 {
	converted := make([]float32, len(values))
	for i, value := range values {
		converted[i] = float32(value)
	}
	return converted
}

// Available reports whether the gateway answered the cached /models
// probe.
func (e *OpenAICompatibleEmbedder) Available() bool { return e.client.Available() }

// ModelID returns the configured gateway model identifier.
func (e *OpenAICompatibleEmbedder) ModelID() string { return e.model }

// UnavailableReason reports why the gateway is unavailable, or "" when it
// is available.
func (e *OpenAICompatibleEmbedder) UnavailableReason() string {
	return e.client.UnavailableReason()
}
