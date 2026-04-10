package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestOllamaSynthesizer_Synthesize verifies text generation via POST /api/generate
// with stream=false. The mock server validates the request format and returns
// a single JSON response.
func TestOllamaSynthesizer_Synthesize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req generateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if req.Model != "llama3.2:3b" {
			http.Error(w, "model not found", http.StatusNotFound)
			return
		}

		if req.Stream {
			t.Error("expected stream=false in request")
		}

		resp := generateResponse{
			Response: "compiled article text",
			Done:     true,
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	s := NewOllamaSynthesizer(srv.URL, "llama3.2:3b")

	result, err := s.Synthesize(context.Background(), "synthesize this content")
	if err != nil {
		t.Fatalf("Synthesize() error: %v", err)
	}

	if result != "compiled article text" {
		t.Errorf("Synthesize() = %q, want %q", result, "compiled article text")
	}
}

// TestOllamaSynthesizer_Synthesize_ValidatesRequest verifies that the correct
// JSON payload is sent to the Ollama API including model, prompt, and stream fields.
func TestOllamaSynthesizer_Synthesize_ValidatesRequest(t *testing.T) {
	var receivedReq generateRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&receivedReq); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp := generateResponse{Response: "ok", Done: true}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	s := NewOllamaSynthesizer(srv.URL, "llama3.2:3b")
	_, err := s.Synthesize(context.Background(), "test prompt")
	if err != nil {
		t.Fatalf("Synthesize() error: %v", err)
	}

	if receivedReq.Model != "llama3.2:3b" {
		t.Errorf("request model = %q, want %q", receivedReq.Model, "llama3.2:3b")
	}
	if receivedReq.Prompt != "test prompt" {
		t.Errorf("request prompt = %q, want %q", receivedReq.Prompt, "test prompt")
	}
	if receivedReq.Stream {
		t.Error("request stream = true, want false")
	}
}

// TestOllamaSynthesizer_Synthesize_Error verifies that a 500 status from
// Ollama is returned as an error.
func TestOllamaSynthesizer_Synthesize_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
	}))
	defer srv.Close()

	s := NewOllamaSynthesizer(srv.URL, "llama3.2:3b")

	_, err := s.Synthesize(context.Background(), "test prompt")
	if err == nil {
		t.Fatal("Synthesize() should return error on 500 status")
	}
}

// TestOllamaSynthesizer_Synthesize_MalformedResponse verifies that a
// malformed JSON response from Ollama is returned as an error.
func TestOllamaSynthesizer_Synthesize_MalformedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{invalid json`))
	}))
	defer srv.Close()

	s := NewOllamaSynthesizer(srv.URL, "llama3.2:3b")

	_, err := s.Synthesize(context.Background(), "test prompt")
	if err == nil {
		t.Fatal("Synthesize() should return error on malformed JSON")
	}
}

// TestOllamaSynthesizer_Synthesize_ContextCancellation verifies that
// Synthesize respects context cancellation.
func TestOllamaSynthesizer_Synthesize_ContextCancellation(t *testing.T) {
	// Use a channel to unblock the handler when the test is done,
	// so srv.Close() doesn't wait for the slow handler to finish.
	done := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block until the test signals completion.
		<-done
	}))
	defer srv.Close()
	defer close(done)

	s := NewOllamaSynthesizer(srv.URL, "llama3.2:3b")

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := s.Synthesize(ctx, "test prompt")
	if err == nil {
		t.Fatal("Synthesize() should return error on context cancellation")
	}
}

// TestOllamaSynthesizer_Available_ModelExists verifies Available() returns
// true when the model is listed in GET /api/tags.
func TestOllamaSynthesizer_Available_ModelExists(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		resp := tagsResponse{
			Models: []tagModel{
				{Name: "llama3:latest"},
				{Name: "llama3.2:3b"},
				{Name: "granite-embedding:30m"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	s := NewOllamaSynthesizer(srv.URL, "llama3.2:3b")
	s.checkInterval = 0 // Disable caching for test.

	if !s.Available() {
		t.Error("Available() = false, want true")
	}
}

// TestOllamaSynthesizer_Available_ModelMissing verifies Available() returns
// false when the model is not in the tags list.
func TestOllamaSynthesizer_Available_ModelMissing(t *testing.T) {
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

	s := NewOllamaSynthesizer(srv.URL, "llama3.2:3b")
	s.checkInterval = 0

	if s.Available() {
		t.Error("Available() = true, want false (model not in list)")
	}
}

// TestOllamaSynthesizer_Unreachable verifies Available() returns false
// when Ollama is not running (connection refused).
func TestOllamaSynthesizer_Unreachable(t *testing.T) {
	// Create a server and immediately close it to get an unreachable URL.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	s := NewOllamaSynthesizer(url, "llama3.2:3b")
	s.checkInterval = 0
	s.client.Timeout = 1 * time.Second

	if s.Available() {
		t.Error("Available() = true, want false (server closed)")
	}
}

// TestOllamaSynthesizer_Synthesize_Unreachable verifies Synthesize() returns
// an error when Ollama is not running.
func TestOllamaSynthesizer_Synthesize_Unreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	s := NewOllamaSynthesizer(url, "llama3.2:3b")
	s.client.Timeout = 1 * time.Second

	_, err := s.Synthesize(context.Background(), "test")
	if err == nil {
		t.Fatal("Synthesize() should return error when server is unreachable")
	}
}

// TestOllamaSynthesizer_Available_Caching verifies that Available() caches
// results and does not make repeated HTTP calls within the cache interval.
func TestOllamaSynthesizer_Available_Caching(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		resp := tagsResponse{
			Models: []tagModel{{Name: "llama3.2:3b"}},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	s := NewOllamaSynthesizer(srv.URL, "llama3.2:3b")
	s.checkInterval = 1 * time.Hour // Long cache interval.

	// First call should hit the server.
	s.Available()
	// Second call should use cache.
	s.Available()

	if callCount != 1 {
		t.Errorf("server called %d times, want 1 (caching should prevent second call)", callCount)
	}
}

// TestOllamaSynthesizer_ModelID verifies ModelID() returns the configured model.
func TestOllamaSynthesizer_ModelID(t *testing.T) {
	s := NewOllamaSynthesizer("http://localhost:11434", "llama3.2:3b")
	if got := s.ModelID(); got != "llama3.2:3b" {
		t.Errorf("ModelID() = %q, want %q", got, "llama3.2:3b")
	}
}

// TestNewOllamaSynthesizer_ReturnsConfiguredSynthesizer verifies the
// constructor returns a fully initialized synthesizer with correct fields.
func TestNewOllamaSynthesizer_ReturnsConfiguredSynthesizer(t *testing.T) {
	s := NewOllamaSynthesizer("http://custom:9999", "test-model:v1")

	if s == nil {
		t.Fatal("NewOllamaSynthesizer returned nil")
	}
	if s.baseURL != "http://custom:9999" {
		t.Errorf("baseURL = %q, want %q", s.baseURL, "http://custom:9999")
	}
	if s.model != "test-model:v1" {
		t.Errorf("model = %q, want %q", s.model, "test-model:v1")
	}
	if s.ModelID() != "test-model:v1" {
		t.Errorf("ModelID() = %q, want %q", s.ModelID(), "test-model:v1")
	}
	if s.client == nil {
		t.Fatal("client should not be nil")
	}
	if s.client.Timeout != 120*time.Second {
		t.Errorf("client.Timeout = %v, want 120s", s.client.Timeout)
	}
	if s.checkInterval != 30*time.Second {
		t.Errorf("checkInterval = %v, want 30s", s.checkInterval)
	}
}

// TestNoopSynthesizer_Synthesize verifies the test double returns the
// configured response and error.
func TestNoopSynthesizer_Synthesize(t *testing.T) {
	n := &NoopSynthesizer{
		Response: "test response",
		Err:      nil,
		Model:    "noop",
		Avail:    true,
	}

	result, err := n.Synthesize(context.Background(), "any prompt")
	if err != nil {
		t.Fatalf("Synthesize() error: %v", err)
	}
	if result != "test response" {
		t.Errorf("Synthesize() = %q, want %q", result, "test response")
	}
}

// TestNoopSynthesizer_Synthesize_Error verifies the test double returns
// the configured error.
func TestNoopSynthesizer_Synthesize_Error(t *testing.T) {
	expectedErr := errors.New("synthesis failed")
	n := &NoopSynthesizer{
		Response: "",
		Err:      expectedErr,
	}

	_, err := n.Synthesize(context.Background(), "any prompt")
	if err == nil {
		t.Fatal("Synthesize() should return configured error")
	}
	if err != expectedErr {
		t.Errorf("Synthesize() error = %v, want %v", err, expectedErr)
	}
}

// TestNoopSynthesizer_Available verifies the test double returns the
// configured availability.
func TestNoopSynthesizer_Available(t *testing.T) {
	n := &NoopSynthesizer{Avail: true}
	if !n.Available() {
		t.Error("Available() = false, want true")
	}

	n.Avail = false
	if n.Available() {
		t.Error("Available() = true, want false")
	}
}

// TestNoopSynthesizer_ModelID verifies the test double returns the
// configured model identifier.
func TestNoopSynthesizer_ModelID(t *testing.T) {
	n := &NoopSynthesizer{Model: "noop-model"}
	if got := n.ModelID(); got != "noop-model" {
		t.Errorf("ModelID() = %q, want %q", got, "noop-model")
	}
}

// TestNoopSynthesizer_ImplementsSynthesizer verifies that NoopSynthesizer
// satisfies the Synthesizer interface at compile time.
func TestNoopSynthesizer_ImplementsSynthesizer(t *testing.T) {
	var _ Synthesizer = (*NoopSynthesizer)(nil)
	var _ Synthesizer = (*OllamaSynthesizer)(nil)
}
