package source

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestGitHubSource creates a GitHubSource pointing at a test server.
func newTestGitHubSource(t *testing.T, server *httptest.Server) *GitHubSource {
	t.Helper()
	gs := NewGitHubSource("github-test", "test", "test-org", []string{"test-repo"}, []string{"issues", "readme"})
	gs.baseURL = server.URL
	gs.token = "test-token" // Use a test token to avoid env var lookup.
	return gs
}

func TestGitHubSource_FetchIssues(t *testing.T) {
	issues := []map[string]any{
		{
			"number":     1,
			"title":      "Bug report",
			"body":       "Something is broken",
			"html_url":   "https://github.com/test-org/test-repo/issues/1",
			"state":      "open",
			"updated_at": "2026-03-22T10:00:00Z",
			"labels":     []map[string]string{{"name": "bug"}},
		},
		{
			"number":     2,
			"title":      "Feature request",
			"body":       "Add new feature",
			"html_url":   "https://github.com/test-org/test-repo/issues/2",
			"state":      "open",
			"updated_at": "2026-03-22T11:00:00Z",
			"labels":     []map[string]string{},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/issues") {
			_ = json.NewEncoder(w).Encode(issues)
			return
		}
		if strings.Contains(r.URL.Path, "/readme") {
			_ = json.NewEncoder(w).Encode(map[string]string{
				"content":  "IyBSRUFETUU=", // base64 "# README"
				"encoding": "base64",
				"html_url": "https://github.com/test-org/test-repo/blob/main/README.md",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	gs := newTestGitHubSource(t, server)
	docs, err := gs.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	// Should have 2 issues + 1 readme = 3 documents.
	if len(docs) != 3 {
		t.Fatalf("expected 3 documents, got %d", len(docs))
	}

	// Verify first issue.
	found := false
	for _, doc := range docs {
		if strings.Contains(doc.Title, "Bug report") {
			found = true
			if doc.OriginURL != "https://github.com/test-org/test-repo/issues/1" {
				t.Errorf("origin_url = %q", doc.OriginURL)
			}
			if doc.SourceID != "github-test" {
				t.Errorf("source_id = %q, want %q", doc.SourceID, "github-test")
			}
		}
	}
	if !found {
		t.Error("bug report issue not found in results")
	}
}

func TestGitHubSource_AuthTokenPrecedence(t *testing.T) {
	// Test that GITHUB_TOKEN env var takes precedence.
	t.Setenv("GITHUB_TOKEN", "env-token")
	t.Setenv("GH_TOKEN", "gh-token")

	token := resolveGitHubToken()
	if token != "env-token" {
		t.Errorf("token = %q, want %q (GITHUB_TOKEN should take precedence)", token, "env-token")
	}
}

func TestGitHubSource_GHTokenFallback(t *testing.T) {
	// Clear GITHUB_TOKEN, set GH_TOKEN.
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "gh-token")

	token := resolveGitHubToken()
	if token != "gh-token" {
		t.Errorf("token = %q, want %q (GH_TOKEN should be fallback)", token, "gh-token")
	}
}

func TestGitHubSource_NoToken(t *testing.T) {
	// Clear both env vars.
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")

	// resolveGitHubToken will try `gh auth token` which may or may not work.
	// We just verify it doesn't panic.
	_ = resolveGitHubToken()
}

func TestGitHubSource_RateLimit(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount > 1 {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", "1711100000")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message": "API rate limit exceeded"}`))
			return
		}
		// First request succeeds with issues.
		if strings.Contains(r.URL.Path, "/issues") {
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"number": 1, "title": "Issue 1", "body": "body", "html_url": "https://example.com/1", "state": "open", "updated_at": "2026-03-22T10:00:00Z", "labels": []any{}},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	gs := newTestGitHubSource(t, server)
	gs.contentType = []string{"issues", "readme"} // readme will hit rate limit

	docs, err := gs.List()
	// Should not return an error — rate limit is handled gracefully (FR-020).
	if err != nil {
		t.Fatalf("List should not error on rate limit: %v", err)
	}

	// Should have partial results (at least the first issue).
	if len(docs) == 0 {
		t.Error("expected at least partial results despite rate limit")
	}

	// Source should report rate limit error.
	meta := gs.Meta()
	if meta.Status != "error" {
		t.Errorf("status = %q, want %q after rate limit", meta.Status, "error")
	}
}

func TestGitHubSource_AuthHeader(t *testing.T) {
	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode([]any{})
	}))
	defer server.Close()

	gs := newTestGitHubSource(t, server)
	gs.token = "secret-token"
	gs.contentType = []string{"issues"}
	_, _ = gs.List()

	if authHeader != "Bearer secret-token" {
		t.Errorf("auth header = %q, want %q", authHeader, "Bearer secret-token")
	}
}

func TestGitHubSource_NoAuthHeader(t *testing.T) {
	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode([]any{})
	}))
	defer server.Close()

	gs := newTestGitHubSource(t, server)
	gs.token = "" // No token — unauthenticated.
	gs.contentType = []string{"issues"}
	_, _ = gs.List()

	if authHeader != "" {
		t.Errorf("auth header should be empty for unauthenticated, got %q", authHeader)
	}
}

func TestGitHubSource_TokenNotLogged(t *testing.T) {
	// Verify the token field is not exported and not in Meta().
	gs := NewGitHubSource("test", "test", "org", []string{"repo"}, nil)
	gs.token = "super-secret"

	meta := gs.Meta()
	metaJSON, _ := json.Marshal(meta)
	if strings.Contains(string(metaJSON), "super-secret") {
		t.Error("token should not appear in Meta() output")
	}
}

func TestGitHubSource_SkipsPullsInIssuesEndpoint(t *testing.T) {
	// GitHub's issues endpoint includes PRs. We should filter them out.
	items := []map[string]any{
		{
			"number":     1,
			"title":      "Real issue",
			"body":       "body",
			"html_url":   "https://github.com/org/repo/issues/1",
			"state":      "open",
			"updated_at": "2026-03-22T10:00:00Z",
			"labels":     []any{},
		},
		{
			"number":     2,
			"title":      "Pull request",
			"body":       "body",
			"html_url":   "https://github.com/org/repo/pull/2",
			"state":      "open",
			"updated_at": "2026-03-22T10:00:00Z",
			"labels":     []any{},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(items)
	}))
	defer server.Close()

	gs := newTestGitHubSource(t, server)
	gs.contentType = []string{"issues"}

	docs, err := gs.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	// Should only have the real issue, not the PR.
	if len(docs) != 1 {
		t.Fatalf("expected 1 document (PR filtered), got %d", len(docs))
	}
}

func TestGitHubSource_Meta(t *testing.T) {
	// Ensure GITHUB_TOKEN is set to avoid side effects from gh CLI lookup.
	t.Setenv("GITHUB_TOKEN", "test")

	gs := NewGitHubSource("github-gaze", "gaze", "unbound-force", []string{"gaze"}, nil)
	meta := gs.Meta()

	if meta.ID != "github-gaze" {
		t.Errorf("id = %q, want %q", meta.ID, "github-gaze")
	}
	if meta.Type != "github" {
		t.Errorf("type = %q, want %q", meta.Type, "github")
	}
}
