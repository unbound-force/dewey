package source

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// GitHubSource implements the Source interface for GitHub repositories.
// It fetches issues, pull requests, and READMEs via the GitHub REST API.
//
// Token precedence (FR-015a):
//  1. GITHUB_TOKEN or GH_TOKEN environment variable
//  2. `gh auth token` subprocess if gh CLI is available
//  3. Unauthenticated access (60 req/hr rate limit)
//
// Tokens are NEVER logged or persisted (FR-015b).
type GitHubSource struct {
	id          string
	name        string
	org         string
	repos       []string
	contentType []string // "issues", "pulls", "readme"
	token       string   // runtime-only, never persisted
	baseURL     string   // for testing; defaults to "https://api.github.com"
	client      *http.Client
	lastFetched time.Time
	lastError   string
	status      string

	// rateLimited tracks whether we hit a rate limit during the last fetch.
	rateLimited bool
}

// NewGitHubSource creates a GitHubSource for the given organization and repositories.
func NewGitHubSource(id, name, org string, repos, contentTypes []string) *GitHubSource {
	if len(contentTypes) == 0 {
		contentTypes = []string{"issues", "pulls", "readme"}
	}

	gs := &GitHubSource{
		id:          id,
		name:        name,
		org:         org,
		repos:       repos,
		contentType: contentTypes,
		baseURL:     "https://api.github.com",
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		status: "active",
	}

	// Resolve token using precedence chain (FR-015a).
	// Token is obtained at runtime only and never written to disk or logs.
	gs.token = resolveGitHubToken()

	return gs
}

// resolveGitHubToken obtains a GitHub token using the precedence chain:
// 1. GITHUB_TOKEN env var
// 2. GH_TOKEN env var
// 3. `gh auth token` subprocess
// 4. Empty string (unauthenticated)
//
// Tokens are NEVER logged (FR-015b).
func resolveGitHubToken() string {
	// 1. GITHUB_TOKEN env var.
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		return token
	}

	// 2. GH_TOKEN env var.
	if token := os.Getenv("GH_TOKEN"); token != "" {
		return token
	}

	// 3. `gh auth token` subprocess.
	cmd := exec.Command("gh", "auth", "token")
	out, err := cmd.Output()
	if err == nil {
		token := strings.TrimSpace(string(out))
		if token != "" {
			return token
		}
	}

	// 4. Unauthenticated — log warning per FR-015b.
	logger.Warn("no GitHub token found, using unauthenticated access (60 req/hr limit)",
		"hint", "set GITHUB_TOKEN or install gh CLI for higher rate limits")

	return ""
}

// List returns all documents from configured GitHub repositories.
func (gs *GitHubSource) List() ([]Document, error) {
	var docs []Document
	gs.rateLimited = false

	for _, repo := range gs.repos {
		for _, ct := range gs.contentType {
			var fetched []Document
			var err error

			switch ct {
			case "issues":
				fetched, err = gs.fetchIssues(repo)
			case "pulls":
				fetched, err = gs.fetchPulls(repo)
			case "readme":
				fetched, err = gs.fetchReadme(repo)
			}

			if err != nil {
				if gs.rateLimited {
					logger.Warn("GitHub rate limit reached, stopping fetch",
						"repo", repo, "content", ct)
					gs.lastError = "rate limit exceeded"
					gs.status = "error"
					// Return what we have so far — partial fetch is better than nothing.
					gs.lastFetched = time.Now()
					return docs, nil
				}
				logger.Warn("failed to fetch GitHub content",
					"repo", repo, "content", ct, "err", err)
				continue
			}
			docs = append(docs, fetched...)
		}
	}

	gs.lastFetched = time.Now()
	gs.status = "active"
	gs.lastError = ""
	return docs, nil
}

// Fetch retrieves a single document by its source-specific ID.
func (gs *GitHubSource) Fetch(id string) (*Document, error) {
	// ID format: "repo/type/number" (e.g., "gaze/issues/42")
	parts := strings.SplitN(id, "/", 3)
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid GitHub document ID: %s", id)
	}

	repo, contentType, number := parts[0], parts[1], parts[2]
	url := fmt.Sprintf("%s/repos/%s/%s/%s/%s", gs.baseURL, gs.org, repo, contentType, number)

	body, err := gs.doRequest(url)
	if err != nil {
		return nil, err
	}

	var item githubItem
	if err := json.Unmarshal(body, &item); err != nil {
		return nil, fmt.Errorf("parse GitHub response: %w", err)
	}

	doc := gs.itemToDocument(repo, contentType, &item)
	return &doc, nil
}

// Diff returns changes since the last fetch. GitHub source uses timestamps
// for incremental updates — only items updated after lastFetched are returned.
func (gs *GitHubSource) Diff() ([]Change, error) {
	// For GitHub, Diff is equivalent to List with a since filter.
	// The manager handles the refresh interval; Diff just returns all current items.
	docs, err := gs.List()
	if err != nil {
		return nil, err
	}

	changes := make([]Change, len(docs))
	for i, doc := range docs {
		d := doc // capture for pointer
		changes[i] = Change{
			Type:     ChangeModified,
			Document: &d,
			ID:       doc.ID,
		}
	}
	return changes, nil
}

// Meta returns metadata about this GitHub source.
func (gs *GitHubSource) Meta() SourceMetadata {
	return SourceMetadata{
		ID:            gs.id,
		Type:          "github",
		Name:          gs.name,
		Status:        gs.status,
		ErrorMessage:  gs.lastError,
		LastFetchedAt: gs.lastFetched,
	}
}

// --- GitHub API types ---

type githubItem struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	HTMLURL   string    `json:"html_url"`
	State     string    `json:"state"`
	UpdatedAt time.Time `json:"updated_at"`
	Labels    []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

type githubReadme struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
	HTMLURL  string `json:"html_url"`
}

// --- Fetch helpers ---

func (gs *GitHubSource) fetchIssues(repo string) ([]Document, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/issues?state=all&per_page=100&sort=updated&direction=desc",
		gs.baseURL, gs.org, repo)

	body, err := gs.doRequest(url)
	if err != nil {
		return nil, err
	}

	var items []githubItem
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, fmt.Errorf("parse issues: %w", err)
	}

	docs := make([]Document, 0, len(items))
	for _, item := range items {
		// Skip pull requests that appear in the issues endpoint.
		// The issues API includes PRs; we filter them out here.
		if item.HTMLURL != "" && strings.Contains(item.HTMLURL, "/pull/") {
			continue
		}
		doc := gs.itemToDocument(repo, "issues", &item)
		docs = append(docs, doc)
	}
	return docs, nil
}

func (gs *GitHubSource) fetchPulls(repo string) ([]Document, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls?state=all&per_page=100&sort=updated&direction=desc",
		gs.baseURL, gs.org, repo)

	body, err := gs.doRequest(url)
	if err != nil {
		return nil, err
	}

	var items []githubItem
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, fmt.Errorf("parse pulls: %w", err)
	}

	docs := make([]Document, 0, len(items))
	for _, item := range items {
		doc := gs.itemToDocument(repo, "pulls", &item)
		docs = append(docs, doc)
	}
	return docs, nil
}

func (gs *GitHubSource) fetchReadme(repo string) ([]Document, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/readme", gs.baseURL, gs.org, repo)

	body, err := gs.doRequest(url)
	if err != nil {
		return nil, err
	}

	var readme githubReadme
	if err := json.Unmarshal(body, &readme); err != nil {
		return nil, fmt.Errorf("parse readme: %w", err)
	}

	// GitHub returns base64-encoded content.
	content := readme.Content
	if readme.Encoding == "base64" {
		// Remove newlines from base64 content (GitHub adds line breaks).
		content = strings.ReplaceAll(content, "\n", "")
		decoded, err := decodeBase64(content)
		if err != nil {
			return nil, fmt.Errorf("decode readme: %w", err)
		}
		content = decoded
	}

	doc := Document{
		ID:          fmt.Sprintf("%s/readme", repo),
		Title:       fmt.Sprintf("%s/%s README", gs.org, repo),
		Content:     content,
		ContentHash: computeHash(content),
		SourceID:    gs.id,
		OriginURL:   readme.HTMLURL,
		FetchedAt:   time.Now(),
		Properties: map[string]any{
			"repo": repo,
			"type": "readme",
		},
	}
	return []Document{doc}, nil
}

func (gs *GitHubSource) itemToDocument(repo, contentType string, item *githubItem) Document {
	id := fmt.Sprintf("%s/%s/%d", repo, contentType, item.Number)
	title := fmt.Sprintf("[%s/%s#%d] %s", gs.org, repo, item.Number, item.Title)

	// Build content with metadata header.
	var sb strings.Builder
	fmt.Fprintf(&sb, "# %s\n\n", item.Title)
	if item.State != "" {
		fmt.Fprintf(&sb, "**State**: %s\n", item.State)
	}
	if len(item.Labels) > 0 {
		labels := make([]string, len(item.Labels))
		for i, l := range item.Labels {
			labels[i] = l.Name
		}
		fmt.Fprintf(&sb, "**Labels**: %s\n", strings.Join(labels, ", "))
	}
	sb.WriteString("\n")
	sb.WriteString(item.Body)

	content := sb.String()

	props := map[string]any{
		"repo":   repo,
		"type":   contentType,
		"number": item.Number,
		"state":  item.State,
	}

	return Document{
		ID:          id,
		Title:       title,
		Content:     content,
		ContentHash: computeHash(content),
		SourceID:    gs.id,
		OriginURL:   item.HTMLURL,
		FetchedAt:   time.Now(),
		Properties:  props,
	}
}

// doRequest performs an authenticated HTTP GET request to the GitHub API.
// Handles rate limiting by detecting 403 responses with rate limit headers.
func (gs *GitHubSource) doRequest(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "dewey/1.0")

	// Add auth token if available. Token is NEVER logged (FR-015b).
	if gs.token != "" {
		req.Header.Set("Authorization", "Bearer "+gs.token)
	}

	resp, err := gs.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GitHub API request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Cap response body to 10MB to prevent unbounded memory allocation
	// from malicious or misconfigured API responses.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	// Check for rate limiting.
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		remaining := resp.Header.Get("X-RateLimit-Remaining")
		if remaining == "0" || resp.StatusCode == http.StatusTooManyRequests {
			gs.rateLimited = true
			resetStr := resp.Header.Get("X-RateLimit-Reset")
			resetTime := "unknown"
			if resetStr != "" {
				if ts, err := strconv.ParseInt(resetStr, 10, 64); err == nil {
					resetTime = time.Unix(ts, 0).Format(time.RFC3339)
				}
			}
			return nil, fmt.Errorf("GitHub API rate limit exceeded (resets at %s)", resetTime)
		}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// decodeBase64 decodes a base64-encoded string.
func decodeBase64(s string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}
