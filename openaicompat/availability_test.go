package openaicompat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAvailable_2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("probe path: got %q, want /v1/models", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	client := newTestClient(t, CapabilityEmbedding, server.URL+"/v1")
	if !client.Available() {
		t.Fatal("expected available")
	}
	if reason := client.UnavailableReason(); reason != "" {
		t.Fatalf("expected empty reason, got %q", reason)
	}
}

func TestAvailable_ProbeSendsAuthorization(t *testing.T) {
	var mu sync.Mutex
	var auth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		auth = r.Header.Get("Authorization")
		mu.Unlock()
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	t.Setenv("DEWEY_EMBEDDING_API_KEY", "sk-probe")
	client, err := NewClient(Config{Capability: CapabilityEmbedding, Endpoint: server.URL + "/v1"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client.Available()
	mu.Lock()
	defer mu.Unlock()
	if auth != "Bearer sk-probe" {
		t.Fatalf("probe authorization: got %q", auth)
	}
}

func TestAvailable_ConnectionError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	endpoint := server.URL
	server.Close()

	client := newTestClient(t, CapabilityEmbedding, endpoint+"/v1")
	if client.Available() {
		t.Fatal("expected unavailable")
	}
	reason := client.UnavailableReason()
	if !strings.Contains(reason, "unreachable") || !strings.Contains(reason, "embedding") {
		t.Fatalf("unexpected reason: %q", reason)
	}
}

func TestAvailable_401WithoutKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := newTestClient(t, CapabilityEmbedding, server.URL+"/v1")
	if client.Available() {
		t.Fatal("expected unavailable")
	}
	reason := client.UnavailableReason()
	if !strings.Contains(reason, "requires authentication") {
		t.Fatalf("unexpected reason: %q", reason)
	}
	if !strings.Contains(reason, "DEWEY_EMBEDDING_API_KEY") || !strings.Contains(reason, "DEWEY_API_KEY") {
		t.Fatalf("reason must name the expected variables: %q", reason)
	}
}

func TestAvailable_403WithKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	t.Setenv("DEWEY_EMBEDDING_API_KEY", "sk-bad")
	client, err := NewClient(Config{Capability: CapabilityEmbedding, Endpoint: server.URL + "/v1"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if client.Available() {
		t.Fatal("expected unavailable")
	}
	reason := client.UnavailableReason()
	if !strings.Contains(reason, "verify the key") || !strings.Contains(reason, "403") {
		t.Fatalf("unexpected reason: %q", reason)
	}
	if strings.Contains(reason, "sk-bad") {
		t.Fatalf("key leaked: %q", reason)
	}
}

func TestAvailable_OtherStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	client := newTestClient(t, CapabilityEmbedding, server.URL+"/v1")
	if client.Available() {
		t.Fatal("expected unavailable")
	}
	if reason := client.UnavailableReason(); !strings.Contains(reason, "HTTP 502") {
		t.Fatalf("unexpected reason: %q", reason)
	}
}

func TestAvailable_Cache(t *testing.T) {
	var mu sync.Mutex
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := newTestClient(t, CapabilityEmbedding, server.URL+"/v1")
	client.Available()
	client.Available()

	mu.Lock()
	first := requests
	mu.Unlock()
	if first != 1 {
		t.Fatalf("cached call made another network request: %d", first)
	}

	// Force cache expiry, then confirm a fresh probe.
	client.mu.Lock()
	client.lastCheck = time.Now().Add(-availabilityCacheTTL - time.Second)
	client.mu.Unlock()
	client.Available()
	mu.Lock()
	defer mu.Unlock()
	if requests != 2 {
		t.Fatalf("expected a second probe after expiry, got %d", requests)
	}
}

func TestAvailable_ConcurrentWithRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/models") {
			_, _ = w.Write([]byte(`{}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client := newTestClient(t, CapabilityEmbedding, server.URL+"/v1")
	client.waitFn = func(context.Context, time.Duration) error { return nil }

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			client.Available()
		}()
		go func() {
			defer wg.Done()
			if _, err := client.PostJSON(context.Background(), PathEmbeddings, map[string]any{}); err != nil {
				t.Errorf("request leg failed: %v", err)
			}
		}()
	}
	wg.Wait()
}
