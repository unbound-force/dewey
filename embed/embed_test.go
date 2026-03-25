package embed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestOllamaEmbedder_Embed verifies single-text embedding via POST /api/embed.
func TestOllamaEmbedder_Embed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req embedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if req.Model != "granite-embedding:30m" {
			http.Error(w, "model not found", http.StatusNotFound)
			return
		}

		resp := embedResponse{
			Embeddings: [][]float64{{0.1, 0.2, 0.3}},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	e := NewOllamaEmbedder(srv.URL, "granite-embedding:30m")

	vec, err := e.Embed(context.Background(), "test text")
	if err != nil {
		t.Fatalf("Embed() error: %v", err)
	}

	if len(vec) != 3 {
		t.Fatalf("Embed() returned %d dimensions, want 3", len(vec))
	}

	// Verify float64 → float32 conversion.
	if vec[0] != 0.1 || vec[1] != 0.2 || vec[2] != 0.3 {
		t.Errorf("Embed() = %v, want [0.1, 0.2, 0.3]", vec)
	}
}

// TestOllamaEmbedder_EmbedBatch verifies batch embedding with multiple texts.
func TestOllamaEmbedder_EmbedBatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" {
			http.NotFound(w, r)
			return
		}

		var req embedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Input should be an array of strings for batch.
		inputs, ok := req.Input.([]any)
		if !ok {
			http.Error(w, "expected array input", http.StatusBadRequest)
			return
		}

		// Return one embedding per input.
		resp := embedResponse{
			Embeddings: make([][]float64, len(inputs)),
		}
		for i := range inputs {
			resp.Embeddings[i] = []float64{float64(i) * 0.1, float64(i) * 0.2}
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	e := NewOllamaEmbedder(srv.URL, "granite-embedding:30m")

	vecs, err := e.EmbedBatch(context.Background(), []string{"text1", "text2", "text3"})
	if err != nil {
		t.Fatalf("EmbedBatch() error: %v", err)
	}

	if len(vecs) != 3 {
		t.Fatalf("EmbedBatch() returned %d vectors, want 3", len(vecs))
	}

	// Verify each vector has 2 dimensions.
	for i, vec := range vecs {
		if len(vec) != 2 {
			t.Errorf("vector[%d] has %d dimensions, want 2", i, len(vec))
		}
	}
}

// TestOllamaEmbedder_EmbedBatch_Empty verifies empty input returns nil.
func TestOllamaEmbedder_EmbedBatch_Empty(t *testing.T) {
	e := NewOllamaEmbedder("http://localhost:11434", "granite-embedding:30m")

	vecs, err := e.EmbedBatch(context.Background(), []string{})
	if err != nil {
		t.Fatalf("EmbedBatch(empty) error: %v", err)
	}
	if vecs != nil {
		t.Errorf("EmbedBatch(empty) = %v, want nil", vecs)
	}
}

// TestOllamaEmbedder_Available_ModelFound verifies Available() returns true
// when the model is listed in GET /api/tags.
func TestOllamaEmbedder_Available_ModelFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		resp := tagsResponse{
			Models: []tagModel{
				{Name: "llama3:latest"},
				{Name: "granite-embedding:30m"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	e := NewOllamaEmbedder(srv.URL, "granite-embedding:30m")
	e.checkInterval = 0 // Disable caching for test.

	if !e.Available() {
		t.Error("Available() = false, want true")
	}
}

// TestOllamaEmbedder_Available_ModelNotFound verifies Available() returns false
// when the model is not in the tags list.
func TestOllamaEmbedder_Available_ModelNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		resp := tagsResponse{
			Models: []tagModel{
				{Name: "llama3:latest"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	e := NewOllamaEmbedder(srv.URL, "granite-embedding:30m")
	e.checkInterval = 0

	if e.Available() {
		t.Error("Available() = true, want false (model not found)")
	}
}

// TestOllamaEmbedder_Available_ConnectionRefused verifies Available() returns
// false when Ollama is not running.
func TestOllamaEmbedder_Available_ConnectionRefused(t *testing.T) {
	e := NewOllamaEmbedder("http://localhost:1", "granite-embedding:30m")
	e.checkInterval = 0
	e.client.Timeout = 1 * time.Second

	if e.Available() {
		t.Error("Available() = true, want false (connection refused)")
	}
}

// TestOllamaEmbedder_Embed_ConnectionRefused verifies Embed() returns an error
// when Ollama is not running.
func TestOllamaEmbedder_Embed_ConnectionRefused(t *testing.T) {
	e := NewOllamaEmbedder("http://localhost:1", "granite-embedding:30m")
	e.client.Timeout = 1 * time.Second

	_, err := e.Embed(context.Background(), "test")
	if err == nil {
		t.Fatal("Embed() should return error when connection refused")
	}
}

// TestOllamaEmbedder_Embed_ModelNotFound verifies Embed() returns an error
// when the model is not available.
func TestOllamaEmbedder_Embed_ModelNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"model 'nonexistent' not found"}`, http.StatusNotFound)
	}))
	defer srv.Close()

	e := NewOllamaEmbedder(srv.URL, "nonexistent")

	_, err := e.Embed(context.Background(), "test")
	if err == nil {
		t.Fatal("Embed() should return error for unknown model")
	}
}

// TestOllamaEmbedder_ModelID verifies ModelID() returns the configured model.
func TestOllamaEmbedder_ModelID(t *testing.T) {
	e := NewOllamaEmbedder("http://localhost:11434", "granite-embedding:30m")
	if got := e.ModelID(); got != "granite-embedding:30m" {
		t.Errorf("ModelID() = %q, want %q", got, "granite-embedding:30m")
	}
}

// TestNewOllamaEmbedder_ReturnsConfiguredEmbedder verifies the constructor
// returns a fully initialized embedder with correct fields.
func TestNewOllamaEmbedder_ReturnsConfiguredEmbedder(t *testing.T) {
	e := NewOllamaEmbedder("http://custom:9999", "test-model:v1")

	if e == nil {
		t.Fatal("NewOllamaEmbedder returned nil")
	}
	if e.baseURL != "http://custom:9999" {
		t.Errorf("baseURL = %q, want %q", e.baseURL, "http://custom:9999")
	}
	if e.model != "test-model:v1" {
		t.Errorf("model = %q, want %q", e.model, "test-model:v1")
	}
	if e.ModelID() != "test-model:v1" {
		t.Errorf("ModelID() = %q, want %q", e.ModelID(), "test-model:v1")
	}
	if e.client == nil {
		t.Fatal("client should not be nil")
	}
	if e.client.Timeout != 30*time.Second {
		t.Errorf("client.Timeout = %v, want 30s", e.client.Timeout)
	}
	if e.checkInterval != 30*time.Second {
		t.Errorf("checkInterval = %v, want 30s", e.checkInterval)
	}
}

// TestOllamaEmbedder_Available_ConnectionRefused_NoPanic verifies Available()
// returns false without panicking when Ollama is not running.
func TestOllamaEmbedder_Available_ConnectionRefused_NoPanic(t *testing.T) {
	e := NewOllamaEmbedder("http://localhost:1", "granite-embedding:30m")
	e.checkInterval = 0
	e.client.Timeout = 500 * time.Millisecond

	// Should return false, not panic.
	got := e.Available()
	if got {
		t.Error("Available() = true, want false (connection refused)")
	}
}

// TestOllamaEmbedder_Available_Caching verifies that Available() caches results.
func TestOllamaEmbedder_Available_Caching(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		resp := tagsResponse{
			Models: []tagModel{{Name: "granite-embedding:30m"}},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	e := NewOllamaEmbedder(srv.URL, "granite-embedding:30m")
	e.checkInterval = 1 * time.Hour // Long cache interval.

	// First call should hit the server.
	e.Available()
	// Second call should use cache.
	e.Available()

	if callCount != 1 {
		t.Errorf("server called %d times, want 1 (caching should prevent second call)", callCount)
	}
}
