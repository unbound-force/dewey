package embed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// embeddingStub is a stub gateway that records the last request.
type embeddingStub struct {
	server    *httptest.Server
	status    int
	body      string
	lastBody  []byte
	lastPath  string
	lastAuth  string
	callCount int
}

func newEmbeddingStub(t *testing.T, status int, body string) *embeddingStub {
	t.Helper()
	stub := &embeddingStub{status: status, body: body}
	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stub.callCount++
		stub.lastPath = r.URL.Path
		stub.lastAuth = r.Header.Get("Authorization")
		if r.Method == http.MethodPost {
			var decoded map[string]any
			_ = json.NewDecoder(r.Body).Decode(&decoded)
			stub.lastBody, _ = json.Marshal(decoded)
		}
		w.WriteHeader(stub.status)
		_, _ = w.Write([]byte(stub.body))
	}))
	t.Cleanup(stub.server.Close)
	return stub
}

const twoVectorBody = `{"data":[
  {"index":1,"embedding":[3.0,4.0]},
  {"index":0,"embedding":[1.0,2.0]}
]}`

func TestOpenAICompatibleEmbedder_RequestShape(t *testing.T) {
	stub := newEmbeddingStub(t, http.StatusOK, twoVectorBody)
	clearEmbeddingKeyEnv(t)

	embedder, err := NewOpenAICompatibleEmbedder(stub.server.URL+"/v1", "openrouter/amazon/titan-embed-text-v2")
	if err != nil {
		t.Fatalf("NewOpenAICompatibleEmbedder: %v", err)
	}

	vectors, err := embedder.EmbedBatch(context.Background(), []string{"first", "second"})
	if err != nil {
		t.Fatalf("EmbedBatch: %v", err)
	}
	if stub.callCount != 1 {
		t.Fatalf("expected one request per batch, got %d", stub.callCount)
	}
	if stub.lastPath != "/v1/embeddings" {
		t.Fatalf("path: got %q, want /v1/embeddings", stub.lastPath)
	}

	if len(vectors) != 2 {
		t.Fatalf("expected 2 vectors, got %d", len(vectors))
	}
	assertVector(t, vectors[0], []float32{1, 2})
	assertVector(t, vectors[1], []float32{3, 4})

	var payload struct {
		Model string   `json:"model"`
		Input []string `json:"input"`
	}
	if err := json.Unmarshal(stub.lastBody, &payload); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	if payload.Model != "openrouter/amazon/titan-embed-text-v2" {
		t.Fatalf("model must be verbatim: %q", payload.Model)
	}
	if len(payload.Input) != 2 || payload.Input[0] != "first" || payload.Input[1] != "second" {
		t.Fatalf("input order not preserved: %v", payload.Input)
	}
}

func TestOpenAICompatibleEmbedder_EmbedSingle(t *testing.T) {
	stub := newEmbeddingStub(t, http.StatusOK, `{"data":[{"index":0,"embedding":[0.5,0.25]}]}`)
	clearEmbeddingKeyEnv(t)

	embedder, err := NewOpenAICompatibleEmbedder(stub.server.URL+"/v1", "model")
	if err != nil {
		t.Fatalf("NewOpenAICompatibleEmbedder: %v", err)
	}
	vector, err := embedder.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	assertVector(t, vector, []float32{0.5, 0.25})
}

func TestOpenAICompatibleEmbedder_EmptyBatchNoNetworkCall(t *testing.T) {
	stub := newEmbeddingStub(t, http.StatusOK, `{"data":[]}`)
	clearEmbeddingKeyEnv(t)

	embedder, err := NewOpenAICompatibleEmbedder(stub.server.URL+"/v1", "model")
	if err != nil {
		t.Fatalf("NewOpenAICompatibleEmbedder: %v", err)
	}
	vectors, err := embedder.EmbedBatch(context.Background(), nil)
	if err != nil {
		t.Fatalf("EmbedBatch: %v", err)
	}
	if vectors != nil {
		t.Fatalf("expected nil vectors, got %v", vectors)
	}
	if stub.callCount != 0 {
		t.Fatalf("empty input must not make a network call, got %d", stub.callCount)
	}
}

func TestOpenAICompatibleEmbedder_OrderingErrors(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		inputs   []string
		wantWord string
	}{
		{
			name:     "count mismatch",
			body:     `{"data":[{"index":0,"embedding":[1]}]}`,
			inputs:   []string{"a", "b"},
			wantWord: "1 vectors for 2 inputs",
		},
		{
			name:     "duplicate index",
			body:     `{"data":[{"index":0,"embedding":[1]},{"index":0,"embedding":[2]}]}`,
			inputs:   []string{"a", "b"},
			wantWord: "duplicate index 0",
		},
		{
			name:     "out of range index",
			body:     `{"data":[{"index":0,"embedding":[1]},{"index":5,"embedding":[2]}]}`,
			inputs:   []string{"a", "b"},
			wantWord: "out of range",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := newEmbeddingStub(t, http.StatusOK, tt.body)
			clearEmbeddingKeyEnv(t)
			embedder, err := NewOpenAICompatibleEmbedder(stub.server.URL+"/v1", "model")
			if err != nil {
				t.Fatalf("NewOpenAICompatibleEmbedder: %v", err)
			}
			_, err = embedder.EmbedBatch(context.Background(), tt.inputs)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantWord) {
				t.Fatalf("expected %q in error, got %v", tt.wantWord, err)
			}
		})
	}
}

func TestOpenAICompatibleEmbedder_NonASCIIRoundTrip(t *testing.T) {
	stub := newEmbeddingStub(t, http.StatusOK, `{"data":[{"index":0,"embedding":[1]}]}`)
	clearEmbeddingKeyEnv(t)

	embedder, err := NewOpenAICompatibleEmbedder(stub.server.URL+"/v1", "model")
	if err != nil {
		t.Fatalf("NewOpenAICompatibleEmbedder: %v", err)
	}
	if _, err := embedder.Embed(context.Background(), "héllo — 世界"); err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if !strings.Contains(string(stub.lastBody), "héllo — 世界") {
		t.Fatalf("non-ASCII text did not round-trip: %s", stub.lastBody)
	}
}

func TestOpenAICompatibleEmbedder_401WithoutKeyNamesVariables(t *testing.T) {
	stub := newEmbeddingStub(t, http.StatusUnauthorized, "unauthorized")
	clearEmbeddingKeyEnv(t)

	embedder, err := NewOpenAICompatibleEmbedder(stub.server.URL+"/v1", "model")
	if err != nil {
		t.Fatalf("NewOpenAICompatibleEmbedder: %v", err)
	}
	_, err = embedder.Embed(context.Background(), "text")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "DEWEY_EMBEDDING_API_KEY") {
		t.Fatalf("error must name the scoped variable: %v", err)
	}
	if !strings.Contains(err.Error(), "DEWEY_API_KEY") {
		t.Fatalf("error must name the shared variable: %v", err)
	}
}

func TestOpenAICompatibleEmbedder_401WithKeyRedacts(t *testing.T) {
	stub := newEmbeddingStub(t, http.StatusUnauthorized, "bad key sk-embed-secret")
	t.Setenv("DEWEY_EMBEDDING_API_KEY", "sk-embed-secret")
	t.Setenv("DEWEY_API_KEY", "")

	embedder, err := NewOpenAICompatibleEmbedder(stub.server.URL+"/v1", "model")
	if err != nil {
		t.Fatalf("NewOpenAICompatibleEmbedder: %v", err)
	}
	_, err = embedder.Embed(context.Background(), "text")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "sk-embed-secret") {
		t.Fatalf("key leaked: %v", err)
	}
	if !strings.Contains(err.Error(), "verify the key") {
		t.Fatalf("expected verify-key guidance: %v", err)
	}
	if stub.lastAuth != "Bearer sk-embed-secret" {
		t.Fatalf("authorization: got %q", stub.lastAuth)
	}
}

func TestNewOpenAICompatibleEmbedder_Validation(t *testing.T) {
	if _, err := NewOpenAICompatibleEmbedder("", "model"); err == nil || !strings.Contains(err.Error(), "embedding.endpoint") {
		t.Fatalf("missing endpoint error: %v", err)
	}
	if _, err := NewOpenAICompatibleEmbedder("https://host/v1", ""); err == nil || !strings.Contains(err.Error(), "embedding.model") {
		t.Fatalf("missing model error: %v", err)
	}
}

func TestOpenAICompatibleEmbedder_AvailabilityAndModelID(t *testing.T) {
	stub := newEmbeddingStub(t, http.StatusOK, `{"data":[]}`)
	clearEmbeddingKeyEnv(t)

	embedder, err := NewOpenAICompatibleEmbedder(stub.server.URL+"/v1", "gateway-embedding-model")
	if err != nil {
		t.Fatalf("NewOpenAICompatibleEmbedder: %v", err)
	}
	if embedder.ModelID() != "gateway-embedding-model" {
		t.Fatalf("ModelID: got %q", embedder.ModelID())
	}
	if !embedder.Available() {
		t.Fatal("expected available")
	}
	if reason := embedder.UnavailableReason(); reason != "" {
		t.Fatalf("expected empty reason, got %q", reason)
	}
}

func assertVector(t *testing.T, got, want []float32) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("vector length: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("vector[%d]: got %v, want %v", i, got[i], want[i])
		}
	}
}

// clearEmbeddingKeyEnv blanks the embedding credential variables.
func clearEmbeddingKeyEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DEWEY_EMBEDDING_API_KEY", "")
	t.Setenv("DEWEY_API_KEY", "")
	t.Setenv("OPENROUTER_API_KEY", "")
}
