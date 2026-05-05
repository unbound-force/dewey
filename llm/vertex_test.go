package llm

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

func TestVertexSynthesizer_Synthesize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}

		var req vertexSynthRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.AnthropicVersion != "vertex-2023-10-16" {
			t.Errorf("anthropic_version = %q, want vertex-2023-10-16", req.AnthropicVersion)
		}
		if len(req.Messages) != 1 || req.Messages[0].Content != "test prompt" {
			t.Errorf("unexpected messages: %+v", req.Messages)
		}

		resp := vertexSynthResponse{
			Content: []vertexContentBlock{
				{Type: "text", Text: "synthesized output"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	v := newTestVertexSynth(srv)

	text, err := v.Synthesize(context.Background(), "test prompt")
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if text != "synthesized output" {
		t.Errorf("text = %q, want 'synthesized output'", text)
	}
}

func TestVertexSynthesizer_AuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid credentials"}}`))
	}))
	defer srv.Close()

	v := newTestVertexSynth(srv)

	_, err := v.Synthesize(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error = %q, want to contain '401'", err.Error())
	}
}

func TestVertexSynthesizer_ModelNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"message":"Publisher Model was not found"}}`))
	}))
	defer srv.Close()

	v := newTestVertexSynth(srv)

	_, err := v.Synthesize(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error = %q, want to contain '404'", err.Error())
	}
}

func TestVertexSynthesizer_NoTextContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := vertexSynthResponse{
			Content: []vertexContentBlock{
				{Type: "tool_use", Text: ""},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	v := newTestVertexSynth(srv)
	_, err := v.Synthesize(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for no text content")
	}
	if !strings.Contains(err.Error(), "no text content") {
		t.Errorf("error = %q, want 'no text content'", err.Error())
	}
}

func TestVertexSynthesizer_Available_True(t *testing.T) {
	v := &VertexSynthesizer{
		model: "claude-sonnet-4-6",
		tokenFn: func(_ context.Context) (string, error) {
			return "valid-token", nil
		},
		checkExpiry: 30 * time.Second,
	}
	if !v.Available() {
		t.Error("Available() = false, want true")
	}
}

func TestVertexSynthesizer_Available_False(t *testing.T) {
	v := &VertexSynthesizer{
		model: "claude-sonnet-4-6",
		tokenFn: func(_ context.Context) (string, error) {
			return "", fmt.Errorf("no credentials")
		},
		checkExpiry: 30 * time.Second,
	}
	if v.Available() {
		t.Error("Available() = true, want false")
	}
}

func TestVertexSynthesizer_ModelID(t *testing.T) {
	v := &VertexSynthesizer{model: "claude-sonnet-4-6"}
	if v.ModelID() != "claude-sonnet-4-6" {
		t.Errorf("ModelID = %q, want claude-sonnet-4-6", v.ModelID())
	}
}

func TestNewVertexSynthesizer_MissingModel(t *testing.T) {
	_, err := NewVertexSynthesizer("project", "region", "")
	if err == nil {
		t.Fatal("expected error for missing model")
	}
}

// newTestVertexSynth creates a VertexSynthesizer that routes requests to
// the given test server with a mock token function.
func newTestVertexSynth(srv *httptest.Server) *VertexSynthesizer {
	return &VertexSynthesizer{
		project: "test-project",
		region:  "us-east5",
		model:   "claude-sonnet-4-6",
		client: &http.Client{
			Transport: &vertexURLRewrite{target: srv.URL, transport: srv.Client().Transport},
		},
		tokenFn: func(_ context.Context) (string, error) {
			return "test-token", nil
		},
	}
}

// vertexURLRewrite redirects requests to the test server.
type vertexURLRewrite struct {
	target    string
	transport http.RoundTripper
}

func (t *vertexURLRewrite) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme = "http"
	req.URL.Host = strings.TrimPrefix(t.target, "http://")
	if t.transport != nil {
		return t.transport.RoundTrip(req)
	}
	return http.DefaultTransport.RoundTrip(req)
}
