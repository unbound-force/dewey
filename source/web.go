package source

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/k3a/html2text"
)

const (
	// maxResponseBody is the maximum response body size per page (FR-017b).
	maxResponseBody = 1 * 1024 * 1024 // 1MB

	// maxPagesPerSource is the maximum number of pages per source (FR-017b).
	maxPagesPerSource = 100

	// defaultRateLimit is the default delay between requests.
	defaultRateLimit = 1 * time.Second
)

// WebSource implements the Source interface for web crawl content.
// It fetches HTML pages, converts them to plain text via k3a/html2text,
// and respects robots.txt directives and rate limits.
//
// Safety constraints (FR-017a/b/c):
//   - Only http:// and https:// schemes allowed
//   - Max 1MB response body per page
//   - Max 100 pages per source
//   - Follow redirects within same domain only
//   - Respect robots.txt
//   - Configurable rate limiting (default: 1s between requests)
type WebSource struct {
	id          string
	name        string
	urls        []string
	depth       int
	rateLimit   time.Duration
	cacheDir    string
	client      *http.Client
	lastFetched time.Time
	lastError   string
	status      string

	// robotsCache stores parsed robots.txt rules per domain.
	robotsCache map[string]*robotsRules
}

// NewWebSource creates a WebSource for the given URLs.
func NewWebSource(id, name string, urls []string, depth int, rateLimit time.Duration, cacheDir string) *WebSource {
	if depth < 0 {
		depth = 0
	}
	if rateLimit <= 0 {
		rateLimit = defaultRateLimit
	}

	ws := &WebSource{
		id:        id,
		name:      name,
		urls:      urls,
		depth:     depth,
		rateLimit: rateLimit,
		cacheDir:  cacheDir,
		client: &http.Client{
			Timeout: 30 * time.Second,
			// Custom redirect policy: same-domain only (FR-017c).
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return fmt.Errorf("too many redirects")
				}
				if len(via) > 0 {
					origHost := via[0].URL.Hostname()
					newHost := req.URL.Hostname()
					if origHost != newHost {
						return fmt.Errorf("cross-domain redirect blocked: %s → %s (FR-017c)", origHost, newHost)
					}
				}
				return nil
			},
		},
		status:      "active",
		robotsCache: make(map[string]*robotsRules),
	}

	return ws
}

// List returns all documents from configured web URLs.
func (ws *WebSource) List() ([]Document, error) {
	var docs []Document
	visited := make(map[string]bool)

	for _, seedURL := range ws.urls {
		// Validate URL scheme (FR-017a).
		parsed, err := url.Parse(seedURL)
		if err != nil {
			logger.Warn("invalid URL, skipping", "url", seedURL, "err", err)
			continue
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			logger.Warn("unsupported URL scheme, skipping (only http/https allowed)",
				"url", seedURL, "scheme", parsed.Scheme)
			continue
		}

		// Crawl from seed URL.
		crawled := ws.crawl(seedURL, ws.depth, visited)
		docs = append(docs, crawled...)

		// Enforce max pages per source (FR-017b).
		if len(docs) >= maxPagesPerSource {
			logger.Warn("max pages per source reached, stopping crawl",
				"limit", maxPagesPerSource)
			docs = docs[:maxPagesPerSource]
			break
		}
	}

	ws.lastFetched = time.Now()
	if len(docs) == 0 && ws.lastError != "" {
		ws.status = "error"
	} else {
		ws.status = "active"
		ws.lastError = ""
	}

	// Cache documents to disk.
	if ws.cacheDir != "" {
		ws.cacheDocuments(docs)
	}

	return docs, nil
}

// Fetch retrieves a single document by URL.
func (ws *WebSource) Fetch(id string) (*Document, error) {
	// Check cache first.
	if ws.cacheDir != "" {
		if doc := ws.loadFromCache(id); doc != nil {
			return doc, nil
		}
	}

	doc, _, err := ws.fetchPage(id)
	if err != nil {
		return nil, err
	}
	return doc, nil
}

// Diff returns changes since the last fetch. Web sources don't support
// incremental updates — every fetch is a full crawl.
func (ws *WebSource) Diff() ([]Change, error) {
	docs, err := ws.List()
	if err != nil {
		return nil, err
	}

	changes := make([]Change, len(docs))
	for i, doc := range docs {
		d := doc
		changes[i] = Change{
			Type:     ChangeModified,
			Document: &d,
			ID:       doc.ID,
		}
	}
	return changes, nil
}

// Meta returns metadata about this web source.
func (ws *WebSource) Meta() SourceMetadata {
	return SourceMetadata{
		ID:            ws.id,
		Type:          "web",
		Name:          ws.name,
		Status:        ws.status,
		ErrorMessage:  ws.lastError,
		LastFetchedAt: ws.lastFetched,
	}
}

// --- Crawl implementation ---

func (ws *WebSource) crawl(seedURL string, maxDepth int, visited map[string]bool) []Document {
	var docs []Document
	type crawlItem struct {
		url   string
		depth int
	}

	queue := []crawlItem{{url: seedURL, depth: 0}}

	for len(queue) > 0 && len(docs) < maxPagesPerSource {
		item := queue[0]
		queue = queue[1:]

		if visited[item.url] || item.depth > maxDepth {
			continue
		}
		visited[item.url] = true

		// Validate URL scheme (FR-017a).
		parsed, err := url.Parse(item.url)
		if err != nil {
			continue
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			continue
		}

		// Check robots.txt compliance (FR-017).
		if !ws.isAllowedByRobots(parsed) {
			logger.Warn("blocked by robots.txt, skipping", "url", item.url)
			continue
		}

		// Rate limiting.
		time.Sleep(ws.rateLimit)

		doc, rawHTML, err := ws.fetchPage(item.url)
		if err != nil {
			logger.Warn("failed to fetch page", "url", item.url, "err", err)
			ws.lastError = err.Error()
			continue
		}

		docs = append(docs, *doc)

		// Extract links from raw HTML for deeper crawling (same domain only, FR-017c).
		// Using raw HTML instead of plain text ensures href attributes are preserved.
		if item.depth < maxDepth {
			seedHost := parsed.Hostname()
			links := extractLinks(rawHTML, item.url)
			for _, link := range links {
				linkParsed, err := url.Parse(link)
				if err != nil {
					continue
				}
				if linkParsed.Hostname() == seedHost && !visited[link] {
					queue = append(queue, crawlItem{url: link, depth: item.depth + 1})
				}
			}
		}
	}

	return docs
}

// fetchPage fetches a URL and returns both the Document (with plain text content
// for indexing) and the raw HTML (for link extraction). Returning raw HTML
// separately ensures extractLinks operates on actual anchor tags, not on
// plain text that has had HTML stripped.
func (ws *WebSource) fetchPage(pageURL string) (doc *Document, rawHTML string, err error) {
	req, err := http.NewRequest(http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "dewey/1.0 (knowledge-graph-indexer)")
	req.Header.Set("Accept", "text/html")

	resp, err := ws.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("fetch %q: %w", pageURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("HTTP %d for %q", resp.StatusCode, pageURL)
	}

	// Check content type — skip non-HTML content (T058D).
	contentType := resp.Header.Get("Content-Type")
	if contentType != "" && !strings.Contains(contentType, "text/html") && !strings.Contains(contentType, "text/plain") {
		return nil, "", fmt.Errorf("non-HTML content type %q for %q, skipping", contentType, pageURL)
	}

	// Read body with size limit (FR-017b).
	limitedReader := io.LimitReader(resp.Body, maxResponseBody+1)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, "", fmt.Errorf("read response: %w", err)
	}
	if len(body) > maxResponseBody {
		logger.Warn("response body exceeds 1MB limit, truncating", "url", pageURL)
		body = body[:maxResponseBody]
	}

	rawHTML = string(body)

	// Convert HTML to plain text using k3a/html2text (Decision 5).
	plainText := html2text.HTML2Text(rawHTML)

	// Extract title from HTML.
	title := extractHTMLTitle(rawHTML)
	if title == "" {
		title = pageURL
	}

	d := &Document{
		ID:          pageURL,
		Title:       title,
		Content:     plainText,
		ContentHash: computeHash(plainText),
		SourceID:    ws.id,
		OriginURL:   pageURL,
		FetchedAt:   time.Now(),
		Properties: map[string]any{
			"url":   pageURL,
			"depth": 0, // Will be set by crawler
		},
	}

	return d, rawHTML, nil
}

// --- robots.txt support ---

type robotsRules struct {
	disallowed []string
}

func (ws *WebSource) isAllowedByRobots(u *url.URL) bool {
	domain := u.Scheme + "://" + u.Host

	rules, ok := ws.robotsCache[domain]
	if !ok {
		rules = ws.fetchRobotsTxt(domain)
		ws.robotsCache[domain] = rules
	}

	if rules == nil {
		return true // No robots.txt — allow all.
	}

	path := u.Path
	for _, disallowed := range rules.disallowed {
		if strings.HasPrefix(path, disallowed) {
			return false
		}
	}
	return true
}

func (ws *WebSource) fetchRobotsTxt(domain string) *robotsRules {
	resp, err := ws.client.Get(domain + "/robots.txt")
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil // No robots.txt or error — allow all.
	}
	defer func() { _ = resp.Body.Close() }()

	rules := &robotsRules{}
	scanner := bufio.NewScanner(resp.Body)
	inUserAgent := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(strings.ToLower(line), "user-agent:") {
			agent := strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
			inUserAgent = agent == "*" || strings.Contains(strings.ToLower(agent), "dewey")
		} else if inUserAgent && strings.HasPrefix(strings.ToLower(line), "disallow:") {
			path := strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
			if path != "" {
				rules.disallowed = append(rules.disallowed, path)
			}
		}
	}

	return rules
}

// --- Cache support ---

func (ws *WebSource) cacheDocuments(docs []Document) {
	if ws.cacheDir == "" {
		return
	}

	cacheDir := filepath.Join(ws.cacheDir, ws.id)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		logger.Warn("failed to create cache directory", "dir", cacheDir, "err", err)
		return
	}

	for _, doc := range docs {
		// Use URL hash as filename to avoid path issues.
		filename := computeHash(doc.ID) + ".txt"
		cachePath := filepath.Join(cacheDir, filename)
		if err := os.WriteFile(cachePath, []byte(doc.Content), 0o644); err != nil {
			logger.Warn("failed to cache document", "url", doc.ID, "err", err)
		}
	}
}

func (ws *WebSource) loadFromCache(id string) *Document {
	if ws.cacheDir == "" {
		return nil
	}

	filename := computeHash(id) + ".txt"
	cachePath := filepath.Join(ws.cacheDir, ws.id, filename)

	content, err := os.ReadFile(cachePath)
	if err != nil {
		return nil
	}

	return &Document{
		ID:          id,
		Title:       id,
		Content:     string(content),
		ContentHash: computeHash(string(content)),
		SourceID:    ws.id,
		OriginURL:   id,
		FetchedAt:   time.Now(),
	}
}

// --- HTML helpers ---

// extractHTMLTitle extracts the <title> content from HTML.
func extractHTMLTitle(html string) string {
	lower := strings.ToLower(html)
	start := strings.Index(lower, "<title>")
	if start == -1 {
		return ""
	}
	start += len("<title>")
	end := strings.Index(lower[start:], "</title>")
	if end == -1 {
		return ""
	}
	return strings.TrimSpace(html[start : start+end])
}

// extractLinks extracts href values from anchor tags in HTML content.
// Only returns absolute URLs on the same domain.
func extractLinks(htmlContent, baseURL string) []string {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}

	var links []string
	lower := strings.ToLower(htmlContent)
	searchFrom := 0

	for {
		idx := strings.Index(lower[searchFrom:], "href=\"")
		if idx == -1 {
			break
		}
		start := searchFrom + idx + len("href=\"")
		end := strings.Index(htmlContent[start:], "\"")
		if end == -1 {
			break
		}

		href := htmlContent[start : start+end]
		searchFrom = start + end

		// Resolve relative URLs.
		parsed, err := url.Parse(href)
		if err != nil {
			continue
		}
		resolved := base.ResolveReference(parsed)

		// Only include same-domain http/https links.
		if (resolved.Scheme == "http" || resolved.Scheme == "https") &&
			resolved.Hostname() == base.Hostname() {
			// Strip fragment.
			resolved.Fragment = ""
			links = append(links, resolved.String())
		}
	}

	return links
}
