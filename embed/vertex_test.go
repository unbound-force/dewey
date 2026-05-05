package embed

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestVertexEmbedder_Embed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if !strings.Contains(r.URL.Path, ":predict") {
			t.Errorf("path = %s, want :predict suffix", r.URL.Path)
		}

		var req vertexEmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(req.Instances) != 1 {
			t.Fatalf("instances = %d, want 1", len(req.Instances))
		}
		if req.Instances[0].Content != "hello world" {
			t.Errorf("content = %q, want 'hello world'", req.Instances[0].Content)
		}

		resp := vertexEmbedResponse{
			Predictions: []vertexPrediction{
				{Embeddings: vertexEmbeddingValues{Values: []float64{0.1, 0.2, 0.3}}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	v := newTestVertexEmbedder(srv)

	vec, err := v.Embed(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vec) != 3 {
		t.Fatalf("vector length = %d, want 3", len(vec))
	}
	if vec[0] != 0.1 || vec[1] != 0.2 || vec[2] != 0.3 {
		t.Errorf("vector = %v, want [0.1, 0.2, 0.3]", vec)
	}
}

func TestVertexEmbedder_EmbedBatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req vertexEmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(req.Instances) != 2 {
			t.Fatalf("instances = %d, want 2", len(req.Instances))
		}

		resp := vertexEmbedResponse{
			Predictions: []vertexPrediction{
				{Embeddings: vertexEmbeddingValues{Values: []float64{0.1, 0.2}}},
				{Embeddings: vertexEmbeddingValues{Values: []float64{0.3, 0.4}}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	v := newTestVertexEmbedder(srv)

	vectors, err := v.EmbedBatch(context.Background(), []string{"text1", "text2"})
	if err != nil {
		t.Fatalf("EmbedBatch: %v", err)
	}
	if len(vectors) != 2 {
		t.Fatalf("vectors = %d, want 2", len(vectors))
	}
	if vectors[0][0] != 0.1 {
		t.Errorf("vectors[0][0] = %f, want 0.1", vectors[0][0])
	}
	if vectors[1][0] != 0.3 {
		t.Errorf("vectors[1][0] = %f, want 0.3", vectors[1][0])
	}
}

func TestVertexEmbedder_ErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"Organization Policy constraint violated"}}`))
	}))
	defer srv.Close()

	v := newTestVertexEmbedder(srv)

	_, err := v.Embed(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
	if !strings.Contains(err.Error(), "Organization Policy") {
		t.Errorf("error = %q, want to contain 'Organization Policy'", err.Error())
	}
}

func TestVertexEmbedder_AuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"Request had invalid authentication credentials"}}`))
	}))
	defer srv.Close()

	v := newTestVertexEmbedder(srv)

	_, err := v.Embed(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error = %q, want to contain '401'", err.Error())
	}
}

func TestVertexEmbedder_ModelID(t *testing.T) {
	v := &VertexEmbedder{model: "text-embedding-005"}
	if v.ModelID() != "text-embedding-005" {
		t.Errorf("ModelID = %q, want text-embedding-005", v.ModelID())
	}
}

func TestVertexEmbedder_Available_True(t *testing.T) {
	v := &VertexEmbedder{
		model: "text-embedding-005",
		tokenFn: func(_ context.Context) (string, error) {
			return "valid-token", nil
		},
		checkExpiry: 30 * time.Second,
	}
	if !v.Available() {
		t.Error("Available() = false, want true when tokenFn succeeds")
	}
}

func TestVertexEmbedder_Available_False(t *testing.T) {
	v := &VertexEmbedder{
		model: "text-embedding-005",
		tokenFn: func(_ context.Context) (string, error) {
			return "", fmt.Errorf("no credentials")
		},
		checkExpiry: 30 * time.Second,
	}
	if v.Available() {
		t.Error("Available() = true, want false when tokenFn fails")
	}
}

func TestVertexEmbedder_Available_Cached(t *testing.T) {
	calls := 0
	v := &VertexEmbedder{
		model: "text-embedding-005",
		tokenFn: func(_ context.Context) (string, error) {
			calls++
			return "token", nil
		},
		checkExpiry: 1 * time.Hour,
	}
	v.Available()
	v.Available()
	if calls != 1 {
		t.Errorf("tokenFn called %d times, want 1 (cached)", calls)
	}
}

func TestVertexEmbedder_EmbedBatch_EmptyInput(t *testing.T) {
	v := &VertexEmbedder{model: "test"}
	result, err := v.EmbedBatch(context.Background(), []string{})
	if err != nil {
		t.Fatalf("EmbedBatch empty: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil for empty input, got %v", result)
	}
}

func TestVertexEmbedder_EmbedBatch_PredictionCountMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Return 1 prediction for 2 inputs
		resp := vertexEmbedResponse{
			Predictions: []vertexPrediction{
				{Embeddings: vertexEmbeddingValues{Values: []float64{0.1}}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	v := newTestVertexEmbedder(srv)
	_, err := v.EmbedBatch(context.Background(), []string{"a", "b"})
	if err == nil {
		t.Fatal("expected error for prediction count mismatch")
	}
	if !strings.Contains(err.Error(), "2 inputs") {
		t.Errorf("error = %q, want to mention input count", err.Error())
	}
}

func TestNewVertexEmbedder_MissingModel(t *testing.T) {
	_, err := NewVertexEmbedder("project", "region", "")
	if err == nil {
		t.Fatal("expected error for missing model")
	}
}

// newTestVertexEmbedder creates a VertexEmbedder that routes requests to
// the given test server and uses a mock token function.
func newTestVertexEmbedder(srv *httptest.Server) *VertexEmbedder {
	return &VertexEmbedder{
		project: "test-project",
		region:  "us-central1",
		model:   "text-embedding-005",
		client: &http.Client{
			Transport: &urlRewriteTransport{target: srv.URL, transport: srv.Client().Transport},
		},
		tokenFn: func(_ context.Context) (string, error) {
			return "test-token", nil
		},
	}
}

// urlRewriteTransport redirects all requests to the test server URL
// while preserving the original request path and method.
type urlRewriteTransport struct {
	target    string
	transport http.RoundTripper
}

func (t *urlRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme = "http"
	req.URL.Host = strings.TrimPrefix(t.target, "http://")
	if t.transport != nil {
		return t.transport.RoundTrip(req)
	}
	return http.DefaultTransport.RoundTrip(req)
}
