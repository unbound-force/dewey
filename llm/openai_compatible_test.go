package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

const testAPIKey = "unit-test-placeholder-not-a-secret"

func newTestOpenAISynth(t *testing.T, endpoint string) *OpenAICompatibleSynthesizer {
	t.Helper()
	s, err := NewOpenAICompatibleSynthesizer(endpoint, "rits/zai-org/glm-5-3", testAPIKey, 5*time.Second, 1234)
	if err != nil {
		t.Fatalf("NewOpenAICompatibleSynthesizer: %v", err)
	}
	return s
}

func TestOpenAICompatibleSynthesizer_Synthesize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %s, want /v1/chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+testAPIKey {
			t.Errorf("Authorization = %q, want bearer token", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}

		var req openAIChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Model != "rits/zai-org/glm-5-3" {
			t.Errorf("model = %q", req.Model)
		}
		if req.MaxTokens != 1234 {
			t.Errorf("max_tokens = %d, want 1234", req.MaxTokens)
		}
		if len(req.Messages) != 1 || req.Messages[0].Role != "user" || req.Messages[0].Content != "test prompt" {
			t.Errorf("messages = %+v", req.Messages)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"synthesized output"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	s := newTestOpenAISynth(t, srv.URL+"/v1")

	text, err := s.Synthesize(context.Background(), "test prompt")
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if text != "synthesized output" {
		t.Errorf("text = %q, want 'synthesized output'", text)
	}
}

func TestOpenAICompatibleSynthesizer_EndpointIncludesChatPath(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	s := newTestOpenAISynth(t, srv.URL+"/v1/chat/completions")
	if _, err := s.Synthesize(context.Background(), "p"); err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if gotPath != "/v1/chat/completions" {
		t.Errorf("path = %q, want /v1/chat/completions", gotPath)
	}
}

func TestOpenAICompatibleSynthesizer_HTTPErrorRedactsKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid key ` + testAPIKey + `"}}`))
	}))
	defer srv.Close()

	s := newTestOpenAISynth(t, srv.URL)
	_, err := s.Synthesize(context.Background(), "p")
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error = %q, want HTTP status", err.Error())
	}
	if strings.Contains(err.Error(), testAPIKey) {
		t.Errorf("error leaked the API key: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "[REDACTED]") {
		t.Errorf("error = %q, want redaction marker", err.Error())
	}
}

func TestOpenAICompatibleSynthesizer_TruncatedOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"partial"},"finish_reason":"length"}]}`))
	}))
	defer srv.Close()

	_, err := newTestOpenAISynth(t, srv.URL).Synthesize(context.Background(), "p")
	if err == nil {
		t.Fatal("expected truncation error")
	}
	if !strings.Contains(err.Error(), "truncated") {
		t.Errorf("error = %q, want truncation", err.Error())
	}
	if !strings.Contains(err.Error(), "max_tokens") {
		t.Errorf("error = %q, want max_tokens guidance", err.Error())
	}
}

func TestOpenAICompatibleSynthesizer_EmptyContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"   "},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	_, err := newTestOpenAISynth(t, srv.URL).Synthesize(context.Background(), "p")
	if err == nil || !strings.Contains(err.Error(), "no text content") {
		t.Fatalf("error = %v, want 'no text content'", err)
	}
}

func TestOpenAICompatibleSynthesizer_NoChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv.Close()

	_, err := newTestOpenAISynth(t, srv.URL).Synthesize(context.Background(), "p")
	if err == nil || !strings.Contains(err.Error(), "no choices") {
		t.Fatalf("error = %v, want 'no choices'", err)
	}
}

func TestOpenAICompatibleSynthesizer_MalformedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	_, err := newTestOpenAISynth(t, srv.URL).Synthesize(context.Background(), "p")
	if err == nil || !strings.Contains(err.Error(), "parse") {
		t.Fatalf("error = %v, want parse error", err)
	}
}

func TestOpenAICompatibleSynthesizer_RetryOn429ThenSuccess(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"slow down"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"after retry"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	s := newTestOpenAISynth(t, srv.URL)
	s.baseDelay = time.Millisecond
	s.maxDelay = 2 * time.Millisecond
	text, err := s.Synthesize(context.Background(), "p")
	if err != nil {
		t.Fatalf("Synthesize after retry: %v", err)
	}
	if text != "after retry" {
		t.Errorf("text = %q", text)
	}
	if attempts != 2 {
		t.Errorf("attempts = %d, want 2", attempts)
	}
}

func TestOpenAICompatibleSynthesizer_ContextCancelledDuringRetry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"slow down"}}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := newTestOpenAISynth(t, srv.URL).Synthesize(ctx, "p")
	if err == nil {
		t.Fatal("expected context error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want context.DeadlineExceeded", err)
	}
}

func TestOpenAICompatibleSynthesizer_Defaults(t *testing.T) {
	s, err := NewOpenAICompatibleSynthesizer("https://example.com/v1", "model", "key", 0, 0)
	if err != nil {
		t.Fatalf("NewOpenAICompatibleSynthesizer: %v", err)
	}
	if s.client.Timeout != openAISynthTimeout {
		t.Errorf("timeout = %v, want %v", s.client.Timeout, openAISynthTimeout)
	}
	if s.maxTokens != openAISynthMaxTokens {
		t.Errorf("maxTokens = %d, want %d", s.maxTokens, openAISynthMaxTokens)
	}
	if s.ModelID() != "model" {
		t.Errorf("ModelID = %q", s.ModelID())
	}
	if !s.Available() {
		t.Error("Available() = false, want true")
	}
}

func TestOpenAICompatibleSynthesizer_RequiresFields(t *testing.T) {
	cases := []struct {
		name     string
		endpoint string
		model    string
		key      string
		want     string
	}{
		{"missing endpoint", "", "model", "key", "endpoint"},
		{"missing model", "https://example.com", "", "key", "model"},
		{"missing key", "https://example.com", "model", "", "API key"},
		{"invalid endpoint", "api.example.com/v1", "model", "key", "absolute http(s)"},
		{"unsupported scheme", "ftp://example.com/v1", "model", "key", "absolute http(s)"},
		{"userinfo", "https://user:pass@example.com/v1", "model", "key", "must not contain credentials"},
		{"query", "https://example.com/v1?token=x", "model", "key", "must not contain credentials"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewOpenAICompatibleSynthesizer(tc.endpoint, tc.model, tc.key, 0, 0)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want %q", err.Error(), tc.want)
			}
		})
	}
}

func TestOpenAICompatibleSynthesizer_RetryExhausted(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"slow down ` + testAPIKey + `"}}`))
	}))
	defer srv.Close()

	s := newTestOpenAISynth(t, srv.URL)
	s.baseDelay = time.Millisecond
	s.maxDelay = 2 * time.Millisecond

	_, err := s.Synthesize(context.Background(), "p")
	if err == nil {
		t.Fatal("expected error after retries are exhausted")
	}
	if attempts != openAISynthMaxRetries+1 {
		t.Errorf("attempts = %d, want %d", attempts, openAISynthMaxRetries+1)
	}
	if !strings.Contains(err.Error(), "after 5 retries") {
		t.Errorf("error = %q, want retry count", err.Error())
	}
	if strings.Contains(err.Error(), testAPIKey) {
		t.Errorf("error leaked the API key: %q", err.Error())
	}
}

func TestOpenAICompatibleSynthesizer_ErrorEnvelopeWith200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"error":{"message":"model overloaded"}}`))
	}))
	defer srv.Close()

	_, err := newTestOpenAISynth(t, srv.URL).Synthesize(context.Background(), "p")
	if err == nil || !strings.Contains(err.Error(), "model overloaded") {
		t.Fatalf("error = %v, want provider error envelope", err)
	}
}

func TestOpenAICompatibleSynthesizer_DoesNotFollowRedirects(t *testing.T) {
	var redirected bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirected = true
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"leaked"},"finish_reason":"stop"}]}`))
	}))
	defer target.Close()

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer source.Close()

	_, err := newTestOpenAISynth(t, source.URL).Synthesize(context.Background(), "p")
	if err == nil {
		t.Fatal("expected error for redirect response")
	}
	if redirected {
		t.Error("client followed the redirect and sent the request to another host")
	}
	if !strings.Contains(err.Error(), "302") {
		t.Errorf("error = %q, want HTTP 302", err.Error())
	}
}

func TestOpenAICompatibleSynthesizer_ParseErrorRedactsKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not json ` + testAPIKey))
	}))
	defer srv.Close()

	_, err := newTestOpenAISynth(t, srv.URL).Synthesize(context.Background(), "p")
	if err == nil {
		t.Fatal("expected parse error")
	}
	if strings.Contains(err.Error(), testAPIKey) {
		t.Errorf("parse error leaked the API key: %q", err.Error())
	}
}

func TestOpenAICompatibleSynthesizer_TransportError(t *testing.T) {
	s := newTestOpenAISynth(t, "http://127.0.0.1:1")
	_, err := s.Synthesize(context.Background(), "p")
	if err == nil {
		t.Fatal("expected transport error")
	}
	if strings.Contains(err.Error(), testAPIKey) {
		t.Errorf("transport error leaked the API key: %q", err.Error())
	}
}

func TestOpenAICompatibleSynthesizer_AvailableFalse(t *testing.T) {
	s := &OpenAICompatibleSynthesizer{}
	if s.Available() {
		t.Error("Available() = true for zero-value synthesizer, want false")
	}
}

func TestOpenAICompatibleSynthesizer_RetryAfterZeroIsImmediate(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"slow down"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	start := time.Now()
	if _, err := newTestOpenAISynth(t, srv.URL).Synthesize(context.Background(), "p"); err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("Retry-After: 0 took %v, want an immediate retry", elapsed)
	}
}

func TestTruncateBody_BoundsWithoutRuneSplit(t *testing.T) {
	s := newTestOpenAISynth(t, "http://127.0.0.1:0")

	// More than 512 multibyte runes: the excerpt must end at a rune
	// boundary with the truncation marker, never mid-rune.
	got := s.truncateBody([]byte(strings.Repeat("é", 600)))
	if !strings.HasSuffix(got, "... (truncated)") {
		t.Errorf("excerpt = %q, want truncation marker", got)
	}
	if strings.Contains(got, "\uFFFD") {
		t.Errorf("excerpt split a rune: %q", got)
	}
	head := strings.TrimSuffix(got, "... (truncated)")
	if runes := []rune(head); len(runes) > 512 {
		t.Errorf("excerpt rune count = %d, want <= 512", len(runes))
	}
}

func TestTruncateBody_LargeBodyRedactsBeforeBounding(t *testing.T) {
	s := newTestOpenAISynth(t, "http://127.0.0.1:0")

	// A body well past the 2048-byte bound with the key straddling the
	// cut: redaction runs on the full body first, so no fragment of the
	// key can survive the byte-bound slice.
	body := []byte(strings.Repeat("a", 2030) + testAPIKey + strings.Repeat("b", 2030))
	got := s.truncateBody(body)
	if strings.Contains(got, testAPIKey) {
		t.Errorf("excerpt leaked the API key: %q", got)
	}
	if fragment := testAPIKey[:12]; strings.Contains(got, fragment) {
		t.Errorf("excerpt leaked a key fragment: %q", got)
	}
	if runes := []rune(strings.TrimSuffix(got, "... (truncated)")); len(runes) > 512 {
		t.Errorf("excerpt rune count = %d, want <= 512", len(runes))
	}
}

func TestTruncateBody_ByteCapBacksOffToRuneBoundary(t *testing.T) {
	s := newTestOpenAISynth(t, "http://127.0.0.1:0")

	// 2046 bytes of two-byte runes followed by four-byte runes: the
	// 2048-byte cap slices inside the first emoji. The excerpt must back
	// off to the last complete rune boundary instead of relying on the
	// rune-count cap to re-encode a partial rune away.
	body := []byte(strings.Repeat("é", 1023) + strings.Repeat("😀", 10))
	got := s.truncateBody(body)
	if !strings.HasSuffix(got, "... (truncated)") {
		t.Errorf("excerpt = %q, want truncation marker", got)
	}
	if head := strings.TrimSuffix(got, "... (truncated)"); !utf8.ValidString(head) {
		t.Errorf("excerpt ends mid-rune: %q", head)
	}
	if strings.Contains(got, "\uFFFD") {
		t.Errorf("excerpt contains a replacement rune: %q", got)
	}
}

// TestIsLoopbackHost_MatchesOpenAicompatSemantics pins parity between
// llm.IsLoopbackHost and openaicompat.IsLoopbackHost: the doctor consults
// both helpers in one run, so a host must classify identically in each
// (PR 145 round-4 review amendment).
func TestIsLoopbackHost_MatchesOpenAicompatSemantics(t *testing.T) {
	for _, host := range []string{
		"localhost",
		"LOCALHOST",
		"  localhost  ",
		"127.0.0.1",
		"127.200.1.5",
		"::1",
		"::ffff:127.0.0.1",
	} {
		if !IsLoopbackHost(host) {
			t.Errorf("IsLoopbackHost(%q) = false, want true", host)
		}
	}
	for _, host := range []string{
		"192.0.2.1",
		"example.com",
		"",
		"127.1",
	} {
		if IsLoopbackHost(host) {
			t.Errorf("IsLoopbackHost(%q) = true, want false", host)
		}
	}
}
