package source

import (
	"fmt"
	"net/http"
	"net/http/httptest"
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
