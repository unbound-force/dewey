package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCall_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request structure.
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if r.URL.Path != "/api" {
			t.Errorf("path = %q, want /api", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("content-type = %q, want application/json", ct)
		}

		// Decode the request body to verify structure.
		var req struct {
			Method string `json:"method"`
			Args   []any  `json:"args"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if req.Method != "logseq.Editor.getPage" {
			t.Errorf("req.Method = %q, want %q", req.Method, "logseq.Editor.getPage")
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "test-page",
			"uuid": "page-uuid-1",
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	raw, err := c.call(context.Background(), "logseq.Editor.getPage", "test-page")
	if err != nil {
		t.Fatalf("call() error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if result["name"] != "test-page" {
		t.Errorf("name = %v, want %q", result["name"], "test-page")
	}
}

func TestCall_AuthHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`"ok"`))
	}))
	defer srv.Close()

	c := New(srv.URL, "my-secret-token")
	_, err := c.call(context.Background(), "logseq.Editor.getCurrentPage")
	if err != nil {
		t.Fatalf("call() error: %v", err)
	}
	if gotAuth != "Bearer my-secret-token" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer my-secret-token")
	}
}

func TestCall_NoAuthWhenTokenEmpty(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`"ok"`))
	}))
	defer srv.Close()

	c := New(srv.URL, "")
	_, err := c.call(context.Background(), "logseq.Editor.getCurrentPage")
	if err != nil {
		t.Fatalf("call() error: %v", err)
	}
	if gotAuth != "" {
		t.Errorf("Authorization = %q, want empty (no token)", gotAuth)
	}
}

func TestCall_ClientError_NoRetry(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "")
	_, err := c.call(context.Background(), "logseq.Editor.getPage", "bad")
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1 (no retry on 4xx)", attempts)
	}
}

func TestCall_ServerError_Retries(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"internal"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`"recovered"`))
	}))
	defer srv.Close()

	c := New(srv.URL, "")
	raw, err := c.call(context.Background(), "logseq.Editor.getPage", "test")
	if err != nil {
		t.Fatalf("call() error: %v (should have recovered after retries)", err)
	}
	if attempts < 3 {
		t.Errorf("attempts = %d, want >= 3 (retried on 5xx)", attempts)
	}
	if string(raw) != `"recovered"` {
		t.Errorf("raw = %q, want %q", string(raw), `"recovered"`)
	}
}

func TestCall_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Slow response — should be cancelled.
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL, "")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := c.call(ctx, "logseq.Editor.getPage", "slow")
	if err == nil {
		t.Fatal("expected error when context is cancelled")
	}
}

func TestNew_DefaultURL(t *testing.T) {
	// Clear env vars to test defaults.
	t.Setenv("LOGSEQ_API_URL", "")
	t.Setenv("LOGSEQ_API_TOKEN", "")

	c := New("", "")
	if c == nil {
		t.Fatal("New() returned nil")
	}
	if c.apiURL != defaultAPIURL {
		t.Errorf("apiURL = %q, want %q", c.apiURL, defaultAPIURL)
	}
	// Verify httpClient is initialized with a timeout.
	if c.httpClient == nil {
		t.Fatal("httpClient should not be nil")
	}
	if c.httpClient.Timeout != defaultTimeout {
		t.Errorf("httpClient.Timeout = %v, want %v", c.httpClient.Timeout, defaultTimeout)
	}
}

func TestNew_CustomURL(t *testing.T) {
	c := New("http://custom:1234", "tok")
	if c == nil {
		t.Fatal("New() returned nil")
	}
	if c.apiURL != "http://custom:1234" {
		t.Errorf("apiURL = %q, want %q", c.apiURL, "http://custom:1234")
	}
	if c.token != "tok" {
		t.Errorf("token = %q, want %q", c.token, "tok")
	}
	if c.httpClient == nil {
		t.Fatal("httpClient should not be nil")
	}
}

func TestNew_EnvVars(t *testing.T) {
	t.Setenv("LOGSEQ_API_URL", "http://env-url:9999")
	t.Setenv("LOGSEQ_API_TOKEN", "env-token")

	c := New("", "")
	if c.apiURL != "http://env-url:9999" {
		t.Errorf("apiURL = %q, want %q", c.apiURL, "http://env-url:9999")
	}
	if c.token != "env-token" {
		t.Errorf("token = %q, want %q", c.token, "env-token")
	}
}

func TestPing_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`null`))
	}))
	defer srv.Close()

	c := New(srv.URL, "")
	err := c.Ping(context.Background())
	if err != nil {
		t.Fatalf("Ping() error: %v", err)
	}
}

func TestPing_Error(t *testing.T) {
	// Use a server that always returns 500.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`"error"`))
	}))
	defer srv.Close()

	c := New(srv.URL, "")
	err := c.Ping(context.Background())
	if err == nil {
		t.Fatal("Ping() should return error for 500 responses")
	}
}

func TestGetAllPages_Integration(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"name": "page1", "uuid": "uuid1"},
			{"name": "page2", "uuid": "uuid2"},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "")
	pages, err := c.GetAllPages(context.Background())
	if err != nil {
		t.Fatalf("GetAllPages() error: %v", err)
	}
	if len(pages) != 2 {
		t.Errorf("got %d pages, want 2", len(pages))
	}
	if pages[0].Name != "page1" {
		t.Errorf("first page name = %q, want %q", pages[0].Name, "page1")
	}
}

func TestGetPage_Integration(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "test-page",
			"uuid": "test-uuid",
			"id":   42,
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "")
	page, err := c.GetPage(context.Background(), "test-page")
	if err != nil {
		t.Fatalf("GetPage() error: %v", err)
	}
	if page == nil {
		t.Fatal("GetPage() returned nil")
	}
	if page.Name != "test-page" {
		t.Errorf("page.Name = %q, want %q", page.Name, "test-page")
	}
}

func TestDatascriptQuery_Integration(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[[{"uuid":"b1","content":"test"}]]`))
	}))
	defer srv.Close()

	c := New(srv.URL, "")
	raw, err := c.DatascriptQuery(context.Background(), "[:find ...]")
	if err != nil {
		t.Fatalf("DatascriptQuery() error: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("DatascriptQuery() returned empty response")
	}
}
