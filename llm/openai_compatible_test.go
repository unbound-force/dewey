package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// synthesizeStub starts a stub gateway and returns the recorded request
// body (raw JSON) after the synthesizer call.
type synthesizeStub struct {
	server    *httptest.Server
	status    int
	body      string
	lastBody  []byte
	lastPath  string
	lastAuth  string
	callCount int
}

func newSynthesizeStub(t *testing.T, status int, body string) *synthesizeStub {
	t.Helper()
	stub := &synthesizeStub{status: status, body: body}
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

const completionBody = `{"choices":[{"message":{"role":"assistant","content":"compiled article"}}]}`

func TestOpenAICompatibleSynthesizer_RequestShape(t *testing.T) {
	stub := newSynthesizeStub(t, http.StatusOK, completionBody)
	clearSynthesisKeyEnv(t)

	synth, err := NewOpenAICompatibleSynthesizer(stub.server.URL+"/v1", "enmaas/rits/zai-org/glm-5-3")
	if err != nil {
		t.Fatalf("NewOpenAICompatibleSynthesizer: %v", err)
	}

	got, err := synth.Synthesize(context.Background(), "Synthesize the learnings")
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if got != "compiled article" {
		t.Fatalf("got %q, want compiled article", got)
	}
	if stub.lastPath != "/v1/chat/completions" {
		t.Fatalf("path: got %q, want /v1/chat/completions", stub.lastPath)
	}

	raw := string(stub.lastBody)
	for _, forbidden := range []string{"max_tokens", "tools", "functions", "response_format"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("request must not contain %q: %s", forbidden, raw)
		}
	}

	var payload struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		Stream bool `json:"stream"`
	}
	if err := json.Unmarshal(stub.lastBody, &payload); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	if payload.Model != "enmaas/rits/zai-org/glm-5-3" {
		t.Fatalf("model must be verbatim: %q", payload.Model)
	}
	if payload.Stream {
		t.Fatal("stream must be false")
	}
	if len(payload.Messages) != 1 {
		t.Fatalf("expected one message, got %d", len(payload.Messages))
	}
	if payload.Messages[0].Role != "user" || payload.Messages[0].Content != "Synthesize the learnings" {
		t.Fatalf("unexpected message: %+v", payload.Messages[0])
	}
}

func TestOpenAICompatibleSynthesizer_NonASCIIRoundTrip(t *testing.T) {
	stub := newSynthesizeStub(t, http.StatusOK, `{"choices":[{"message":{"role":"assistant","content":"résumé — 世界"}}]}`)
	clearSynthesisKeyEnv(t)

	synth, err := NewOpenAICompatibleSynthesizer(stub.server.URL+"/v1", "model")
	if err != nil {
		t.Fatalf("NewOpenAICompatibleSynthesizer: %v", err)
	}
	got, err := synth.Synthesize(context.Background(), "héllo — 世界")
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if got != "résumé — 世界" {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(string(stub.lastBody), "héllo — 世界") {
		t.Fatalf("prompt did not round-trip as UTF-8: %s", stub.lastBody)
	}
}

func TestOpenAICompatibleSynthesizer_EmptyContent(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{name: "empty content", body: `{"choices":[{"message":{"role":"assistant","content":""}}]}`, wantErr: "returned empty content"},
		{name: "no choices", body: `{"choices":[]}`, wantErr: "returned no choices"},
		{name: "missing content", body: `{"choices":[{"message":{"role":"assistant"}}]}`, wantErr: "returned empty content"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := newSynthesizeStub(t, http.StatusOK, tt.body)
			clearSynthesisKeyEnv(t)
			synth, err := NewOpenAICompatibleSynthesizer(stub.server.URL+"/v1", "model")
			if err != nil {
				t.Fatalf("NewOpenAICompatibleSynthesizer: %v", err)
			}
			_, err = synth.Synthesize(context.Background(), "prompt")
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected %q in error, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestOpenAICompatibleSynthesizer_401WithoutKeyNamesVariables(t *testing.T) {
	stub := newSynthesizeStub(t, http.StatusUnauthorized, "unauthorized")
	clearSynthesisKeyEnv(t)

	synth, err := NewOpenAICompatibleSynthesizer(stub.server.URL+"/v1", "model")
	if err != nil {
		t.Fatalf("NewOpenAICompatibleSynthesizer: %v", err)
	}
	_, err = synth.Synthesize(context.Background(), "prompt")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "DEWEY_SYNTHESIS_API_KEY") {
		t.Fatalf("error must name the scoped variable: %v", err)
	}
	if !strings.Contains(err.Error(), "DEWEY_API_KEY") {
		t.Fatalf("error must name the shared variable: %v", err)
	}
	if strings.Contains(err.Error(), "config.yaml") {
		t.Fatalf("error must not suggest storing the key: %v", err)
	}
}

func TestOpenAICompatibleSynthesizer_401WithKeyRedacts(t *testing.T) {
	stub := newSynthesizeStub(t, http.StatusUnauthorized, "bad key sk-synth-secret")
	t.Setenv("DEWEY_SYNTHESIS_API_KEY", "sk-synth-secret")
	t.Setenv("DEWEY_API_KEY", "")

	synth, err := NewOpenAICompatibleSynthesizer(stub.server.URL+"/v1", "model")
	if err != nil {
		t.Fatalf("NewOpenAICompatibleSynthesizer: %v", err)
	}
	_, err = synth.Synthesize(context.Background(), "prompt")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "sk-synth-secret") {
		t.Fatalf("key leaked: %v", err)
	}
	if !strings.Contains(err.Error(), "verify the key") {
		t.Fatalf("expected verify-key guidance: %v", err)
	}
	if stub.lastAuth != "Bearer sk-synth-secret" {
		t.Fatalf("authorization: got %q", stub.lastAuth)
	}
}

func TestOpenAICompatibleSynthesizer_NonSuccessError(t *testing.T) {
	stub := newSynthesizeStub(t, http.StatusInternalServerError, "gateway exploded")
	clearSynthesisKeyEnv(t)

	synth, err := NewOpenAICompatibleSynthesizer(stub.server.URL+"/v1", "model")
	if err != nil {
		t.Fatalf("NewOpenAICompatibleSynthesizer: %v", err)
	}
	_, err = synth.Synthesize(context.Background(), "prompt")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "openai-compatible synthesis error (HTTP 500): gateway exploded") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOpenAICompatibleSynthesizer_AvailabilityAndModelID(t *testing.T) {
	stub := newSynthesizeStub(t, http.StatusOK, `{"data":[]}`)
	clearSynthesisKeyEnv(t)

	synth, err := NewOpenAICompatibleSynthesizer(stub.server.URL+"/v1", "gateway-model")
	if err != nil {
		t.Fatalf("NewOpenAICompatibleSynthesizer: %v", err)
	}
	if synth.ModelID() != "gateway-model" {
		t.Fatalf("ModelID: got %q", synth.ModelID())
	}
	if !synth.Available() {
		t.Fatal("expected available")
	}
	if reason := synth.UnavailableReason(); reason != "" {
		t.Fatalf("expected empty reason, got %q", reason)
	}
}

func TestNewOpenAICompatibleSynthesizer_Validation(t *testing.T) {
	if _, err := NewOpenAICompatibleSynthesizer("", "model"); err == nil || !strings.Contains(err.Error(), "synthesis.endpoint") {
		t.Fatalf("missing endpoint error: %v", err)
	}
	if _, err := NewOpenAICompatibleSynthesizer("https://host/v1", ""); err == nil || !strings.Contains(err.Error(), "synthesis.model") {
		t.Fatalf("missing model error: %v", err)
	}
}

// clearSynthesisKeyEnv blanks the synthesis credential variables.
func clearSynthesisKeyEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DEWEY_SYNTHESIS_API_KEY", "")
	t.Setenv("DEWEY_API_KEY", "")
	t.Setenv("OPENROUTER_API_KEY", "")
}
