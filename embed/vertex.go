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

	"golang.org/x/oauth2/google"
)

// VertexEmbedder implements Embedder using Google Vertex AI's prediction API.
// Uses application-default credentials for authentication (no CGO).
//
// Design decision D3 (pluggable-providers design.md): Direct REST calls
// to the Vertex AI prediction endpoint. The google oauth2 library handles
// credential discovery, caching, and automatic token refresh.
type VertexEmbedder struct {
	project string
	region  string
	model   string
	client  *http.Client
	tokenFn func(ctx context.Context) (string, error)

	// Cached availability check.
	mu          sync.RWMutex
	available   bool
	lastCheck   time.Time
	checkExpiry time.Duration
}

// vertexEmbedRequest is the JSON body sent to the Vertex AI predict endpoint.
type vertexEmbedRequest struct {
	Instances []vertexInstance `json:"instances"`
}

// vertexInstance represents a single text instance for embedding.
type vertexInstance struct {
	Content string `json:"content"`
}

// vertexEmbedResponse is the JSON response from the Vertex AI predict endpoint.
type vertexEmbedResponse struct {
	Predictions []vertexPrediction `json:"predictions"`
}

// vertexPrediction contains the embedding values from a single prediction.
type vertexPrediction struct {
	Embeddings vertexEmbeddingValues `json:"embeddings"`
}

// vertexEmbeddingValues holds the vector values.
type vertexEmbeddingValues struct {
	Values []float64 `json:"values"`
}

// NewVertexEmbedder creates a VertexEmbedder for the given GCP project, region,
// and model. Returns an error if required fields are missing.
//
// Authentication uses application-default credentials discovered via
// golang.org/x/oauth2/google.FindDefaultCredentials. Users must run
// `gcloud auth application-default login` before using this embedder.
func NewVertexEmbedder(project, region, model string) (*VertexEmbedder, error) {
	if model == "" {
		return nil, fmt.Errorf("vertex embedder requires model")
	}
	e := &VertexEmbedder{
		project: project,
		region:  region,
		model:   model,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		checkExpiry: 30 * time.Second,
	}
	e.tokenFn = e.defaultGetToken
	return e, nil
}

// predictURL builds the Vertex AI prediction endpoint URL.
func (v *VertexEmbedder) predictURL() string {
	return fmt.Sprintf(
		"https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/google/models/%s:predict",
		v.region, v.project, v.region, v.model,
	)
}

// defaultGetToken retrieves an OAuth2 access token using application-default credentials.
func (v *VertexEmbedder) defaultGetToken(ctx context.Context) (string, error) {
	creds, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return "", fmt.Errorf("vertex AI credentials: %w (run 'gcloud auth application-default login --scopes=https://www.googleapis.com/auth/cloud-platform')", err)
	}
	tok, err := creds.TokenSource.Token()
	if err != nil {
		return "", fmt.Errorf("vertex AI token: %w", err)
	}
	return tok.AccessToken, nil
}

// Embed generates a vector embedding for a single text via Vertex AI.
func (v *VertexEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	vectors, err := v.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, fmt.Errorf("vertex AI returned no embeddings")
	}
	return vectors[0], nil
}

// EmbedBatch generates vector embeddings for multiple texts in a single request.
// Returns one float32 slice per input text, in the same order.
func (v *VertexEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	// Build request with one instance per text.
	instances := make([]vertexInstance, len(texts))
	for i, t := range texts {
		instances[i] = vertexInstance{Content: t}
	}
	reqBody := vertexEmbedRequest{Instances: instances}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal vertex request: %w", err)
	}

	token, err := v.tokenFn(ctx)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.predictURL(), bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create vertex request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := v.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vertex AI request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 50*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read vertex response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vertex AI error (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var result vertexEmbedResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse vertex response: %w", err)
	}

	if len(result.Predictions) != len(texts) {
		return nil, fmt.Errorf("vertex AI returned %d predictions for %d inputs", len(result.Predictions), len(texts))
	}

	// Convert float64 → float32 vectors.
	vectors := make([][]float32, len(result.Predictions))
	for i, pred := range result.Predictions {
		vec := make([]float32, len(pred.Embeddings.Values))
		for j, val := range pred.Embeddings.Values {
			vec[j] = float32(val)
		}
		vectors[i] = vec
	}

	return vectors, nil
}

// Available reports whether Vertex AI credentials are configured and the
// model is accessible. Caches the result for 30 seconds.
func (v *VertexEmbedder) Available() bool {
	v.mu.RLock()
	if time.Since(v.lastCheck) < v.checkExpiry {
		avail := v.available
		v.mu.RUnlock()
		return avail
	}
	v.mu.RUnlock()

	v.mu.Lock()
	defer v.mu.Unlock()

	// Double-check after acquiring write lock.
	if time.Since(v.lastCheck) < v.checkExpiry {
		return v.available
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := v.tokenFn(ctx)
	v.available = err == nil
	v.lastCheck = time.Now()
	return v.available
}

// ModelID returns the configured Vertex AI model identifier.
func (v *VertexEmbedder) ModelID() string {
	return v.model
}
