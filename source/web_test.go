package source

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWebSource_FetchHTML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><head><title>Test Page</title></head><body><h1>Hello</h1><p>World</p></body></html>`)
	}))
	defer server.Close()

	ws := NewWebSource("web-test", "test", []string{server.URL}, 0, 10*time.Millisecond, "")

	// Verify NewWebSource returns correctly configured source.
	if ws.id != "web-test" {
		t.Errorf("ws.id = %q, want %q", ws.id, "web-test")
	}
	if ws.name != "test" {
		t.Errorf("ws.name = %q, want %q", ws.name, "test")
	}
	if ws.depth != 0 {
		t.Errorf("ws.depth = %d, want 0", ws.depth)
	}
	if ws.rateLimit != 10*time.Millisecond {
		t.Errorf("ws.rateLimit = %v, want 10ms", ws.rateLimit)
	}
	if ws.client == nil {
		t.Fatal("ws.client should not be nil")
	}
	if ws.robotsCache == nil {
		t.Fatal("ws.robotsCache should not be nil")
	}
	if ws.status != "active" {
		t.Errorf("ws.status = %q, want %q", ws.status, "active")
	}

	docs, err := ws.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("expected 1 document, got %d", len(docs))
	}

	doc := docs[0]
	if doc.Title != "Test Page" {
		t.Errorf("title = %q, want %q", doc.Title, "Test Page")
	}
	if !strings.Contains(doc.Content, "Hello") {
		t.Error("content should contain 'Hello'")
	}
	if !strings.Contains(doc.Content, "World") {
		t.Error("content should contain 'World'")
	}
	if doc.SourceID != "web-test" {
		t.Errorf("source_id = %q, want %q", doc.SourceID, "web-test")
	}
	if doc.OriginURL == "" {
		t.Error("origin_url should not be empty")
	}
	if doc.ContentHash == "" {
		t.Error("content hash should not be empty")
	}
	if doc.ID == "" {
		t.Error("document ID should not be empty")
	}
}

func TestWebSource_RobotsCompliance(t *testing.T) {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "User-agent: *\nDisallow: /private/\n")
	})
	mux.HandleFunc("/private/secret", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><body>Secret</body></html>`)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprintf(w, `<html><body><a href="%s/private/secret">Secret</a><a href="%s/public">Public</a></body></html>`, server.URL, server.URL)
	})
	mux.HandleFunc("/public", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><body>Public content</body></html>`)
	})

	ws := NewWebSource("web-test", "test", []string{server.URL + "/"}, 1, 10*time.Millisecond, "")

	docs, err := ws.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	// Should NOT include /private/secret (blocked by robots.txt).
	for _, doc := range docs {
		if strings.Contains(doc.ID, "/private/") {
			t.Errorf("document %q should be blocked by robots.txt", doc.ID)
		}
	}
}

func TestWebSource_RateLimit(t *testing.T) {
	requestTimes := []time.Time{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestTimes = append(requestTimes, time.Now())
		if r.URL.Path == "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		if r.URL.Path == "/" {
			_, _ = fmt.Fprintf(w, `<html><body><a href="/page2">Link</a></body></html>`)
			return
		}
		_, _ = fmt.Fprint(w, `<html><body>Page</body></html>`)
	}))
	defer server.Close()

	rateLimit := 50 * time.Millisecond
	ws := NewWebSource("web-test", "test", []string{server.URL + "/"}, 1, rateLimit, "")

	_, _ = ws.List()

	// Verify rate limiting was applied between requests.
	if len(requestTimes) >= 3 {
		// Skip robots.txt request (first one).
		for i := 2; i < len(requestTimes); i++ {
			gap := requestTimes[i].Sub(requestTimes[i-1])
			if gap < rateLimit/2 { // Allow some tolerance.
				t.Errorf("request gap %d = %v, expected at least %v", i, gap, rateLimit)
			}
		}
	}
}

func TestWebSource_DepthLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		// Each page links to the next.
		depth := strings.Count(r.URL.Path, "/level")
		nextPath := fmt.Sprintf("/level%d", depth+1)
		_, _ = fmt.Fprintf(w, `<html><body>Depth %d <a href="%s">Next</a></body></html>`, depth, nextPath)
	}))
	defer server.Close()

	// Depth 0 = only seed URL.
	ws := NewWebSource("web-test", "test", []string{server.URL + "/"}, 0, 10*time.Millisecond, "")
	docs, _ := ws.List()

	if len(docs) != 1 {
		t.Errorf("depth 0: expected 1 document, got %d", len(docs))
	}
}

func TestWebSource_URLSchemeRestriction(t *testing.T) {
	// file:// and ftp:// schemes should be rejected (FR-017a).
	ws := NewWebSource("web-test", "test", []string{
		"file:///etc/passwd",
		"ftp://example.com/data",
	}, 0, 10*time.Millisecond, "")

	docs, err := ws.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(docs) != 0 {
		t.Errorf("expected 0 documents for non-http schemes, got %d", len(docs))
	}
}

func TestWebSource_SameDomainRedirect(t *testing.T) {
	// Test that same-domain redirects work but cross-domain links are not followed.
	// Note: httptest servers share 127.0.0.1 hostname, so we test the redirect
	// policy by verifying the CheckRedirect function directly.
	ws := NewWebSource("web-test", "test", []string{"https://example.com"}, 0, 10*time.Millisecond, "")

	// The client's CheckRedirect should block cross-domain redirects.
	// We test this by creating requests that simulate cross-domain redirects.
	origReq, _ := http.NewRequest("GET", "https://example.com/page", nil)
	crossReq, _ := http.NewRequest("GET", "https://evil.com/page", nil)

	err := ws.client.CheckRedirect(crossReq, []*http.Request{origReq})
	if err == nil {
		t.Error("cross-domain redirect should be blocked")
	}
	if !strings.Contains(err.Error(), "cross-domain redirect blocked") {
		t.Errorf("error = %q, want cross-domain redirect blocked message", err.Error())
	}

	// Same-domain redirect should be allowed.
	sameReq, _ := http.NewRequest("GET", "https://example.com/other", nil)
	err = ws.client.CheckRedirect(sameReq, []*http.Request{origReq})
	if err != nil {
		t.Errorf("same-domain redirect should be allowed: %v", err)
	}
}

func TestWebSource_MaxResponseBody(t *testing.T) {
	// Generate content larger than 1MB.
	largeContent := strings.Repeat("x", maxResponseBody+100)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprintf(w, `<html><body>%s</body></html>`, largeContent)
	}))
	defer server.Close()

	ws := NewWebSource("web-test", "test", []string{server.URL + "/"}, 0, 10*time.Millisecond, "")

	docs, err := ws.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	// Should still return a document, but truncated.
	if len(docs) != 1 {
		t.Fatalf("expected 1 document, got %d", len(docs))
	}
}

func TestWebSource_NonHTMLContentSkip(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		// Return PDF content type.
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-1.4 fake pdf content"))
	}))
	defer server.Close()

	ws := NewWebSource("web-test", "test", []string{server.URL + "/"}, 0, 10*time.Millisecond, "")

	docs, _ := ws.List()
	if len(docs) != 0 {
		t.Errorf("expected 0 documents for PDF content, got %d", len(docs))
	}
}

func TestNewWebSource_NegativeDepthClamped(t *testing.T) {
	ws := NewWebSource("web-neg", "neg", []string{"https://example.com"}, -5, 0, "")
	if ws.depth != 0 {
		t.Errorf("depth = %d, want 0 (negative depth should be clamped)", ws.depth)
	}
	// Non-positive rateLimit should default to defaultRateLimit.
	if ws.rateLimit != defaultRateLimit {
		t.Errorf("rateLimit = %v, want %v (default)", ws.rateLimit, defaultRateLimit)
	}
}

func TestWebSource_Meta(t *testing.T) {
	ws := NewWebSource("web-go-stdlib", "go-stdlib", []string{"https://pkg.go.dev/std"}, 1, time.Second, "")
	meta := ws.Meta()

	if meta.ID != "web-go-stdlib" {
		t.Errorf("id = %q, want %q", meta.ID, "web-go-stdlib")
	}
	if meta.Type != "web" {
		t.Errorf("type = %q, want %q", meta.Type, "web")
	}
	if meta.Status != "active" {
		t.Errorf("status = %q, want %q", meta.Status, "active")
	}
}

func TestExtractHTMLTitle(t *testing.T) {
	tests := []struct {
		html string
		want string
	}{
		{`<html><head><title>My Page</title></head></html>`, "My Page"},
		{`<html><head><TITLE>Upper Case</TITLE></head></html>`, "Upper Case"},
		{`<html><body>No title</body></html>`, ""},
		{`<title>  Spaces  </title>`, "Spaces"},
	}

	for _, tt := range tests {
		got := extractHTMLTitle(tt.html)
		if got != tt.want {
			t.Errorf("extractHTMLTitle(%q) = %q, want %q", tt.html[:min(len(tt.html), 40)], got, tt.want)
		}
	}
}

func TestWebSource_CacheDocuments_CreatesFiles(t *testing.T) {
	cacheDir := t.TempDir()
	ws := NewWebSource("web-cache-test", "cache-test", nil, 0, 10*time.Millisecond, cacheDir)

	docs := []Document{
		{
			ID:          "https://example.com/page1",
			Title:       "Page 1",
			Content:     "Content of page one",
			ContentHash: computeHash("Content of page one"),
			SourceID:    "web-cache-test",
		},
		{
			ID:          "https://example.com/page2",
			Title:       "Page 2",
			Content:     "Content of page two",
			ContentHash: computeHash("Content of page two"),
			SourceID:    "web-cache-test",
		},
	}

	ws.cacheDocuments(docs)

	// Verify the source-specific subdirectory was created.
	sourceDir := filepath.Join(cacheDir, "web-cache-test")
	info, err := os.Stat(sourceDir)
	if err != nil {
		t.Fatalf("cache source directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("cache source path is not a directory")
	}

	// Verify each document was cached as <hash>.txt.
	for _, doc := range docs {
		filename := computeHash(doc.ID) + ".txt"
		cachePath := filepath.Join(sourceDir, filename)

		content, err := os.ReadFile(cachePath)
		if err != nil {
			t.Fatalf("failed to read cached file for %q: %v", doc.ID, err)
		}
		if string(content) != doc.Content {
			t.Errorf("cached content for %q = %q, want %q", doc.ID, string(content), doc.Content)
		}
	}

	// Verify exactly 2 files were created (no extras).
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		t.Fatalf("failed to read cache directory: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 cached files, got %d", len(entries))
	}
}

func TestWebSource_CacheDocuments_EmptySlice(t *testing.T) {
	cacheDir := t.TempDir()
	ws := NewWebSource("web-cache-empty", "empty", nil, 0, 10*time.Millisecond, cacheDir)

	// Caching an empty slice should still create the directory but no files.
	ws.cacheDocuments([]Document{})

	sourceDir := filepath.Join(cacheDir, "web-cache-empty")
	info, err := os.Stat(sourceDir)
	if err != nil {
		t.Fatalf("cache directory not created for empty docs: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("cache path is not a directory")
	}

	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		t.Fatalf("failed to read cache directory: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 cached files for empty slice, got %d", len(entries))
	}
}

func TestWebSource_CacheDocuments_NoCacheDir(t *testing.T) {
	// When cacheDir is empty, cacheDocuments should be a no-op.
	ws := NewWebSource("web-no-cache", "no-cache", nil, 0, 10*time.Millisecond, "")

	// Should not panic or error.
	ws.cacheDocuments([]Document{
		{ID: "https://example.com", Content: "test"},
	})
}

func TestWebSource_CacheDocuments_NestedDirectory(t *testing.T) {
	// Verify cacheDocuments creates nested directories via MkdirAll.
	cacheDir := filepath.Join(t.TempDir(), "deep", "nested", "path")
	ws := NewWebSource("web-nested", "nested", nil, 0, 10*time.Millisecond, cacheDir)

	docs := []Document{
		{
			ID:      "https://example.com/deep",
			Content: "Deep nested content",
		},
	}

	ws.cacheDocuments(docs)

	// Verify the nested directory was created.
	sourceDir := filepath.Join(cacheDir, "web-nested")
	if _, err := os.Stat(sourceDir); err != nil {
		t.Fatalf("nested cache directory not created: %v", err)
	}

	// Verify file was written.
	filename := computeHash("https://example.com/deep") + ".txt"
	content, err := os.ReadFile(filepath.Join(sourceDir, filename))
	if err != nil {
		t.Fatalf("failed to read cached file: %v", err)
	}
	if string(content) != "Deep nested content" {
		t.Errorf("cached content = %q, want %q", string(content), "Deep nested content")
	}
}

func TestWebSource_Fetch_FromServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><head><title>Fetched Page</title></head><body><p>Fetched content</p></body></html>`)
	}))
	defer server.Close()

	ws := NewWebSource("web-fetch", "fetch", nil, 0, 10*time.Millisecond, "")

	doc, err := ws.Fetch(server.URL + "/page")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if doc.Title != "Fetched Page" {
		t.Errorf("title = %q, want %q", doc.Title, "Fetched Page")
	}
	if !strings.Contains(doc.Content, "Fetched content") {
		t.Errorf("content should contain 'Fetched content', got %q", doc.Content)
	}
	if doc.SourceID != "web-fetch" {
		t.Errorf("source_id = %q, want %q", doc.SourceID, "web-fetch")
	}
	if doc.ContentHash == "" {
		t.Error("content hash should not be empty")
	}
	if doc.OriginURL != server.URL+"/page" {
		t.Errorf("origin_url = %q, want %q", doc.OriginURL, server.URL+"/page")
	}
}

func TestWebSource_Fetch_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	ws := NewWebSource("web-fetch", "fetch", nil, 0, 10*time.Millisecond, "")

	_, err := ws.Fetch(server.URL + "/nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent page")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error = %q, want HTTP 404 message", err.Error())
	}
}

func TestWebSource_Fetch_FromCache(t *testing.T) {
	// Set up a cache with a pre-cached document.
	cacheDir := t.TempDir()
	ws := NewWebSource("web-cached", "cached", nil, 0, 10*time.Millisecond, cacheDir)

	// Pre-populate cache.
	docID := "https://example.com/cached-page"
	sourceDir := filepath.Join(cacheDir, "web-cached")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	filename := computeHash(docID) + ".txt"
	cachedContent := "This is cached content"
	if err := os.WriteFile(filepath.Join(sourceDir, filename), []byte(cachedContent), 0o644); err != nil {
		t.Fatalf("write cache: %v", err)
	}

	// Fetch should return from cache without hitting any server.
	doc, err := ws.Fetch(docID)
	if err != nil {
		t.Fatalf("Fetch from cache: %v", err)
	}

	if doc.Content != cachedContent {
		t.Errorf("content = %q, want %q", doc.Content, cachedContent)
	}
	if doc.ID != docID {
		t.Errorf("id = %q, want %q", doc.ID, docID)
	}
	if doc.SourceID != "web-cached" {
		t.Errorf("source_id = %q, want %q", doc.SourceID, "web-cached")
	}
	if doc.ContentHash == "" {
		t.Error("content hash should not be empty for cached document")
	}
}

func TestWebSource_Fetch_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	ws := NewWebSource("web-err", "err", nil, 0, 10*time.Millisecond, "")

	_, err := ws.Fetch(server.URL + "/error")
	if err == nil {
		t.Fatal("expected error for server error response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error = %q, want HTTP 500 message", err.Error())
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
