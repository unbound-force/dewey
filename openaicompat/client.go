package openaicompat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"
)

const (
	// embeddingTimeout is the FR-025 HTTP timeout for embedding requests.
	embeddingTimeout = 30 * time.Second
	// synthesisTimeout is the FR-025 HTTP timeout for synthesis requests.
	synthesisTimeout = 300 * time.Second
	// probeTimeout is the FR-014/SC-004 availability probe timeout.
	probeTimeout = 1 * time.Second
	// availabilityCacheTTL is the FR-014 cache window.
	availabilityCacheTTL = 30 * time.Second
	// maxResponseBytes mirrors the Vertex providers' 50 MiB read limit.
	maxResponseBytes = 50 * 1024 * 1024
	// maxRedirects preserves the net/http default redirect bound that a
	// custom CheckRedirect replaces. A longer redirect chain is an error,
	// not an infinite loop.
	maxRedirects = 10
)

var clientLogger = log.NewWithOptions(os.Stderr, log.Options{
	Prefix:          "dewey/openaicompat",
	ReportTimestamp: true,
	TimeFormat:      "2006-01-02T15:04:05.000Z07:00",
})

// Config describes one capability-scoped gateway client.
type Config struct {
	// Capability selects the key variable, endpoint field name, and
	// default timeout.
	Capability Capability
	// Endpoint is the configured API base URL (FR-010).
	Endpoint string
}

// Client is the shared OpenAI-compatible transport for one provider
// instance. It is safe for concurrent use.
type Client struct {
	capability Capability
	baseURL    string
	apiKey     string
	httpClient *http.Client

	mu          sync.RWMutex
	available   bool
	lastCheck   time.Time
	checkExpiry time.Duration
	lastReason  string

	// waitFn is the context-aware sleep used between retries. Tests
	// replace it to avoid real delays.
	waitFn func(context.Context, time.Duration) error
}

// NewClient validates the configuration, resolves the environment-only
// API key, and returns a client with a capability-scoped HTTP timeout.
// The endpoint must be present and valid; a missing key is allowed so
// unauthenticated self-hosted gateways work (FR-008).
func NewClient(cfg Config) (*Client, error) {
	if !cfg.Capability.valid() {
		return nil, fmt.Errorf("openaicompat: unknown capability %q", cfg.Capability)
	}
	baseURL, err := NormalizeBaseURL(cfg.Endpoint, cfg.Capability.endpointField())
	if err != nil {
		return nil, err
	}

	timeout := cfg.Capability.defaultTimeout()

	return &Client{
		capability: cfg.Capability,
		baseURL:    baseURL,
		apiKey:     ResolveAPIKey(cfg.Capability, baseURL),
		httpClient: &http.Client{
			Timeout:       timeout,
			CheckRedirect: redirectPolicy,
		},
		checkExpiry: availabilityCacheTTL,
		waitFn:      Wait,
	}, nil
}

// redirectPolicy strips the Authorization header whenever a redirect
// leaves the initial request's origin: a different host (including a
// subdomain of the initial host) or a different scheme (an
// https-to-http downgrade on the same host). The net/http default
// only strips the header across hosts, which still forwards
// credentials to subdomains and over same-host cleartext downgrades
// (spec 017: the Authorization header MUST NOT be forwarded across
// hosts; FR-024: credentials must not transit http outside loopback).
// The chain is also bounded at maxRedirects hops because installing a
// custom CheckRedirect replaces the stdlib's default 10-redirect
// limit, which would otherwise allow an unbounded redirect loop.
func redirectPolicy(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("stopped after %d redirects", maxRedirects)
	}
	if len(via) == 0 {
		return nil
	}
	if sameOrigin(via[0].URL, req.URL) {
		return nil
	}
	req.Header.Del("Authorization")
	return nil
}

// sameOrigin reports whether two URLs share the same scheme and host.
// Hosts compare case-insensitively with any port stripped, the FR-007
// hostname convention; subdomains and scheme downgrades are different
// origins.
func sameOrigin(a, b *url.URL) bool {
	return strings.EqualFold(hostOnly(a.Host), hostOnly(b.Host)) && a.Scheme == b.Scheme
}

// hostOnly strips the port from a URL host, returning the host
// unchanged when no port is present.
func hostOnly(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

// PostJSON marshals payload, POSTs it to one operation path under the
// base URL, and returns the raw 2xx response body. Non-2xx responses
// become bounded, redacted errors; HTTP 429 responses are retried up to
// MaxRetries with Retry-After or exponential backoff.
func (c *Client) PostJSON(ctx context.Context, operation string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("openai-compatible %s marshal request: %w", c.capability, err)
	}
	return c.postWithRetry(ctx, OperationURL(c.baseURL, operation), body)
}

// postWithRetry runs the request/retry loop.
func (c *Client) postWithRetry(ctx context.Context, target string, body []byte) ([]byte, error) {
	for attempt := 0; attempt <= MaxRetries; attempt++ {
		status, respBody, retryAfter, err := c.doRequest(ctx, target, body)
		if err != nil {
			return nil, err
		}
		if status >= http.StatusOK && status < http.StatusMultipleChoices {
			return respBody, nil
		}
		if status != http.StatusTooManyRequests {
			return nil, c.statusError(status, respBody)
		}
		if attempt == MaxRetries {
			return nil, RateLimitError(c.capability, respBody, c.apiKey)
		}

		delay := RetryDelay(attempt, retryAfter)
		clientLogger.Warn("openai-compatible gateway rate limited, retrying",
			"capability", c.capability, "attempt", attempt+1, "delay", delay)
		if err := c.waitFn(ctx, delay); err != nil {
			return nil, err
		}
	}
	return nil, fmt.Errorf("openai-compatible %s retry loop exited unexpectedly", c.capability)
}

// authReason returns the shared 401/403 reason for this client's
// capability: when no key resolved it names the expected environment
// variables (SC-007); when a key resolved it asks the user to verify the
// key. The probe path and the request path share it so both stay
// actionable (FR-008).
func (c *Client) authReason(status int) string {
	if c.apiKey == "" {
		return AuthRequiredReason(c.capability)
	}
	return VerifyKeyReason(c.capability, status)
}

// statusError builds the error for a non-429, non-2xx response. A 401 or
// 403 uses authReason, which already names the environment variables or
// embeds the status, so the status is not appended twice.
func (c *Client) statusError(status int, body []byte) error {
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return fmt.Errorf("%s: %s", c.authReason(status), Excerpt(body, c.apiKey))
	}
	return FormatError(c.capability, status, body, c.apiKey)
}

// doRequest performs one POST attempt and returns the status, bounded
// body, and Retry-After header. The Authorization header is set only
// when a key resolved.
func (c *Client) doRequest(ctx context.Context, target string, body []byte) (int, []byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return 0, nil, "", fmt.Errorf("openai-compatible %s create request: %w", c.capability, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, "", fmt.Errorf("openai-compatible %s request failed: %w", c.capability, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return 0, nil, "", fmt.Errorf("openai-compatible %s read response: %w", c.capability, err)
	}
	return resp.StatusCode, respBody, resp.Header.Get("Retry-After"), nil
}
