package openaicompat

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// newTestClient builds a client for the given server endpoint with the
// given capability, blanking credential variables first.
func newTestClient(t *testing.T, capability Capability, endpoint string) *Client {
	t.Helper()
	clearKeyEnv(t)
	client, err := NewClient(Config{Capability: capability, Endpoint: endpoint})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client.waitFn = func(context.Context, time.Duration) error { return nil }
	return client
}

func TestClient_RequestShape(t *testing.T) {
	var mu sync.Mutex
	var gotMethod, gotPath, gotContentType, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		gotAuth = r.Header.Get("Authorization")
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	readRequest := func() (method, path, contentType, auth string) {
		mu.Lock()
		defer mu.Unlock()
		return gotMethod, gotPath, gotContentType, gotAuth
	}

	t.Run("with key", func(t *testing.T) {
		t.Setenv("DEWEY_EMBEDDING_API_KEY", "sk-embedding")
		client, err := NewClient(Config{Capability: CapabilityEmbedding, Endpoint: server.URL + "/v1"})
		if err != nil {
			t.Fatalf("NewClient: %v", err)
		}
		client.waitFn = func(context.Context, time.Duration) error { return nil }

		body, err := client.PostJSON(context.Background(), PathEmbeddings, map[string]any{"model": "m"})
		if err != nil {
			t.Fatalf("PostJSON: %v", err)
		}
		if string(body) != `{"ok":true}` {
			t.Fatalf("unexpected body: %q", body)
		}
		method, path, contentType, auth := readRequest()
		if method != http.MethodPost {
			t.Fatalf("method: got %q", method)
		}
		if path != "/v1/embeddings" {
			t.Fatalf("path: got %q", path)
		}
		if contentType != "application/json" {
			t.Fatalf("content type: got %q", contentType)
		}
		if auth != "Bearer sk-embedding" {
			t.Fatalf("authorization: got %q", auth)
		}
	})

	t.Run("without key omits authorization", func(t *testing.T) {
		clearKeyEnv(t)
		client, err := NewClient(Config{Capability: CapabilityEmbedding, Endpoint: server.URL + "/v1"})
		if err != nil {
			t.Fatalf("NewClient: %v", err)
		}
		client.waitFn = func(context.Context, time.Duration) error { return nil }

		if client.apiKey != "" {
			t.Fatal("expected no key")
		}
		if _, err := client.PostJSON(context.Background(), PathEmbeddings, map[string]any{}); err != nil {
			t.Fatalf("PostJSON: %v", err)
		}
		if _, _, _, auth := readRequest(); auth != "" {
			t.Fatalf("authorization must be omitted, got %q", auth)
		}
	})
}

func TestClient_NonSuccessError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal boom"))
	}))
	defer server.Close()

	client := newTestClient(t, CapabilityEmbedding, server.URL+"/v1")
	_, err := client.PostJSON(context.Background(), PathEmbeddings, map[string]any{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "openai-compatible embedding error (HTTP 500): internal boom") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_MalformedJSONIsProviderError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("{not json"))
	}))
	defer server.Close()

	client := newTestClient(t, CapabilitySynthesis, server.URL+"/v1")
	body, err := client.PostJSON(context.Background(), PathChatCompletions, map[string]any{})
	if err != nil {
		t.Fatalf("PostJSON should return raw bytes: %v", err)
	}
	var target map[string]any
	parseErr := json.Unmarshal(body, &target)
	if parseErr == nil {
		t.Fatal("expected parse failure")
	}
	if wrapped := ParseError(CapabilitySynthesis, parseErr); !strings.Contains(wrapped.Error(), "openai-compatible synthesis malformed response") {
		t.Fatalf("unexpected parse error: %v", wrapped)
	}
}

func TestClient_RetryThenSuccess(t *testing.T) {
	var mu sync.Mutex
	attempts := 0
	var delays []time.Duration

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		attempts++
		if attempts < 3 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte("slow down"))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client := newTestClient(t, CapabilityEmbedding, server.URL+"/v1")
	client.waitFn = func(_ context.Context, d time.Duration) error {
		delays = append(delays, d)
		return nil
	}

	body, err := client.PostJSON(context.Background(), PathEmbeddings, map[string]any{})
	if err != nil {
		t.Fatalf("PostJSON: %v", err)
	}
	if string(body) != `{"ok":true}` {
		t.Fatalf("unexpected body: %q", body)
	}
	if attempts != 3 {
		t.Fatalf("attempts: got %d, want 3", attempts)
	}
	if len(delays) != 2 || delays[0] != 2*time.Second || delays[1] != 2*time.Second {
		t.Fatalf("Retry-After not honored: %v", delays)
	}
}

func TestClient_RetryExhaustion(t *testing.T) {
	var mu sync.Mutex
	attempts := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		attempts++
		mu.Unlock()
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("rate limited"))
	}))
	defer server.Close()

	client := newTestClient(t, CapabilityEmbedding, server.URL+"/v1")
	_, err := client.PostJSON(context.Background(), PathEmbeddings, map[string]any{})
	if err == nil {
		t.Fatal("expected rate-limit error")
	}
	if !strings.Contains(err.Error(), "after 5 retries") {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts != MaxRetries+1 {
		t.Fatalf("attempts: got %d, want %d", attempts, MaxRetries+1)
	}
}

// TestClient_ContextCancellationDuringRetry pins FR-013: cancellation
// arriving between a 429 and the next attempt propagates through the
// retry wait, so the loop stops and the server sees exactly one
// request. The client uses the real Wait, not a test stub.
func TestClient_ContextCancellationDuringRetry(t *testing.T) {
	var mu sync.Mutex
	attempts := 0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		attempts++
		n := attempts
		mu.Unlock()
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("rate limited"))
		if n == 1 {
			// Cancel after the first 429 is delivered so the
			// backoff wait — not the transport — must observe it.
			cancel()
		}
	}))
	defer server.Close()

	client := newTestClient(t, CapabilityEmbedding, server.URL+"/v1")
	client.waitFn = Wait // real context-aware backoff

	_, err := client.PostJSON(ctx, PathEmbeddings, map[string]any{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected error: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if attempts != 1 {
		t.Fatalf("attempts: got %d, want exactly 1 (cancellation must stop the retry loop)", attempts)
	}
}

func TestClient_CapabilityTimeouts(t *testing.T) {
	clearKeyEnv(t)
	embedding, err := NewClient(Config{Capability: CapabilityEmbedding, Endpoint: "https://gateway.example/v1"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if embedding.httpClient.Timeout != 30*time.Second {
		t.Fatalf("embedding timeout: got %v, want 30s", embedding.httpClient.Timeout)
	}

	synthesis, err := NewClient(Config{Capability: CapabilitySynthesis, Endpoint: "https://gateway.example/v1"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if synthesis.httpClient.Timeout != 300*time.Second {
		t.Fatalf("synthesis timeout: got %v, want 300s", synthesis.httpClient.Timeout)
	}

	// Pin the probe timeout (FR-014/SC-004): it bounds how long the
	// availability probe holds the client write mutex against a
	// hanging gateway, so a regression would serialize concurrent
	// semantic-tool calls.
	if probeTimeout != time.Second {
		t.Fatalf("probe timeout: got %v, want 1s", probeTimeout)
	}
	// Pin the availability cache window (FR-014) and its wiring into
	// new clients.
	if availabilityCacheTTL != 30*time.Second {
		t.Fatalf("availability cache TTL: got %v, want 30s", availabilityCacheTTL)
	}
	if embedding.checkExpiry != 30*time.Second {
		t.Fatalf("check expiry: got %v, want 30s", embedding.checkExpiry)
	}
}

// captureTransport serves one redirect from the initial https request to
// first.example and records the Authorization header of the redirected
// request. The 307 is keyed on scheme as well as host so the redirect
// target — including a same-host scheme downgrade like
// http://first.example — falls through to the 200 branch and the chain
// terminates. A stub RoundTripper is required because a loopback
// httptest server cannot synthesize subdomain or
// same-host-scheme-downgrade redirect shapes.
type captureTransport struct {
	mu          sync.Mutex
	secondAuth  string
	secondCalls int

	// redirectTo is the Location served for the first request; it is
	// set before the client runs and never mutated afterwards.
	redirectTo string
}

func (c *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme == "https" && req.URL.Host == "first.example" {
		return &http.Response{
			StatusCode: http.StatusTemporaryRedirect,
			Header:     http.Header{"Location": []string{c.redirectTo}},
			Body:       http.NoBody,
			Request:    req,
		}, nil
	}
	c.mu.Lock()
	c.secondCalls++
	c.secondAuth = req.Header.Get("Authorization")
	c.mu.Unlock()
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
		Request:    req,
	}, nil
}

// TestClient_RedirectAuthorizationPolicy pins the explicit redirect
// policy: the Authorization header survives a redirect only when the
// destination scheme and host both match the initial request (hosts
// compared port-stripped and case-insensitively, the FR-007
// convention). Cross-host redirects, subdomains, and same-host
// https-to-http downgrades all drop it. The net/http default would
// keep the header for same-host and subdomain destinations regardless
// of scheme, so this policy is what enforces the spec's
// "Authorization header MUST NOT be forwarded across hosts" rule.
// A redirect chain longer than maxRedirects terminates with an error,
// preserving the stdlib default bound that a custom CheckRedirect
// replaces.
func TestClient_RedirectAuthorizationPolicy(t *testing.T) {
	cases := []struct {
		name       string
		redirectTo string
		wantAuth   string
		wantErr    string
	}{
		{"cross host strips", "https://second.example/v1/embeddings", "", ""},
		{"subdomain strips", "https://api.first.example/v1/embeddings", "", ""},
		{"same host scheme downgrade strips", "http://first.example/v1/embeddings", "", ""},
		{"same origin different port keeps", "https://first.example:8443/v1/embeddings", "Bearer sk-embedding", ""},
		{"redirect loop errors after bound", "https://first.example/v1/embeddings", "", "stopped after 10 redirects"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			transport := &captureTransport{redirectTo: tc.redirectTo}
			t.Setenv("DEWEY_EMBEDDING_API_KEY", "sk-embedding")

			client, err := NewClient(Config{Capability: CapabilityEmbedding, Endpoint: "https://first.example/v1"})
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			client.httpClient.Transport = transport
			client.waitFn = func(context.Context, time.Duration) error { return nil }

			_, err = client.PostJSON(context.Background(), PathEmbeddings, map[string]any{})
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got success", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
				}
			} else if err != nil {
				t.Fatalf("PostJSON: %v", err)
			}

			wantSecond := 1
			if tc.wantErr != "" {
				wantSecond = 0
			}
			transport.mu.Lock()
			defer transport.mu.Unlock()
			if transport.secondCalls != wantSecond {
				t.Fatalf("expected %d redirected request(s), got %d", wantSecond, transport.secondCalls)
			}
			if tc.wantErr == "" && transport.secondAuth != tc.wantAuth {
				t.Fatalf("redirected authorization: got %q, want %q", transport.secondAuth, tc.wantAuth)
			}
		})
	}
}

func TestNewClient_InvalidConfig(t *testing.T) {
	if _, err := NewClient(Config{Capability: CapabilityEmbedding, Endpoint: ""}); err == nil {
		t.Fatal("expected endpoint error")
	}
	if _, err := NewClient(Config{Capability: CapabilityEmbedding, Endpoint: "ftp://host/v1"}); err == nil {
		t.Fatal("expected scheme error")
	}
	if _, err := NewClient(Config{Capability: "bogus", Endpoint: "https://host/v1"}); err == nil {
		t.Fatal("expected capability error")
	}
}
