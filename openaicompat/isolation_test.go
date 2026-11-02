package openaicompat_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/unbound-force/dewey/v3/embed"
	"github.com/unbound-force/dewey/v3/llm"
)

// authRecorder records the last Authorization header and path seen by a
// stub gateway.
type authRecorder struct {
	mu   sync.Mutex
	auth string
	path string
}

func (r *authRecorder) record(req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.auth = req.Header.Get("Authorization")
	r.path = req.URL.Path
}

func (r *authRecorder) snapshot() (auth, path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.auth, r.path
}

// TestSeparateGatewaysKeepCredentialsIsolated verifies US4-3: an embedding
// provider and a synthesis provider configured against two different
// gateways each send their own capability-scoped key and never the other's
// (FR-007).
func TestSeparateGatewaysKeepCredentialsIsolated_PerCapability(t *testing.T) {
	var embeddingAuth, synthesisAuth authRecorder

	embeddingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		embeddingAuth.record(r)
		_, _ = w.Write([]byte(`{"data":[{"index":0,"embedding":[0.1,0.2]}]}`))
	}))
	defer embeddingServer.Close()

	synthesisServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		synthesisAuth.record(r)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer synthesisServer.Close()

	t.Setenv("DEWEY_EMBEDDING_API_KEY", "sk-embed-isolated")
	t.Setenv("DEWEY_SYNTHESIS_API_KEY", "sk-synth-isolated")
	t.Setenv("DEWEY_API_KEY", "")
	t.Setenv("OPENROUTER_API_KEY", "")

	embedder, err := embed.NewOpenAICompatibleEmbedder(embeddingServer.URL+"/v1", "embedding-model")
	if err != nil {
		t.Fatalf("NewOpenAICompatibleEmbedder: %v", err)
	}
	synthesizer, err := llm.NewOpenAICompatibleSynthesizer(synthesisServer.URL+"/v1", "synthesis-model")
	if err != nil {
		t.Fatalf("NewOpenAICompatibleSynthesizer: %v", err)
	}

	if _, err := embedder.Embed(context.Background(), "text"); err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if _, err := synthesizer.Synthesize(context.Background(), "prompt"); err != nil {
		t.Fatalf("Synthesize: %v", err)
	}

	gotEmbeddingAuth, gotEmbeddingPath := embeddingAuth.snapshot()
	gotSynthesisAuth, gotSynthesisPath := synthesisAuth.snapshot()

	if gotEmbeddingAuth != "Bearer sk-embed-isolated" {
		t.Errorf("embedding gateway auth = %q, want the embedding-scoped key", gotEmbeddingAuth)
	}
	if gotSynthesisAuth != "Bearer sk-synth-isolated" {
		t.Errorf("synthesis gateway auth = %q, want the synthesis-scoped key", gotSynthesisAuth)
	}
	if gotEmbeddingAuth == "Bearer sk-synth-isolated" {
		t.Error("embedding gateway received the synthesis key")
	}
	if gotSynthesisAuth == "Bearer sk-embed-isolated" {
		t.Error("synthesis gateway received the embedding key")
	}
	if gotEmbeddingPath != "/v1/embeddings" {
		t.Errorf("embedding path = %q", gotEmbeddingPath)
	}
	if gotSynthesisPath != "/v1/chat/completions" {
		t.Errorf("synthesis path = %q", gotSynthesisPath)
	}
}
