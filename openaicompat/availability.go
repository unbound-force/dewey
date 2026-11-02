package openaicompat

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Available reports whether the gateway responded to the cached
// authenticated GET /models probe with any 2xx status (FR-014). The
// result is cached for 30 seconds and the call is safe for concurrent
// use; double-checked locking prevents duplicate probes when several
// callers miss the cache at once.
func (c *Client) Available() bool {
	c.mu.RLock()
	if time.Since(c.lastCheck) < c.checkExpiry {
		available := c.available
		c.mu.RUnlock()
		return available
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.lastCheck) < c.checkExpiry {
		return c.available
	}

	available, reason := c.probe()
	c.available = available
	c.lastReason = reason
	c.lastCheck = time.Now()
	return available
}

// UnavailableReason returns the credential-free reason recorded by the
// most recent probe, or "" when the gateway is available. It triggers a
// probe through Available so callers always see the current cache state.
func (c *Client) UnavailableReason() string {
	c.Available()
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.available {
		return ""
	}
	return c.lastReason
}

// probe performs one authenticated GET /models request with the
// one-second probe timeout. The response body is drained (bounded) and
// closed even though only the status is used.
func (c *Client) probe() (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, OperationURL(c.baseURL, PathModels), nil)
	if err != nil {
		return false, "openai-compatible " + c.capability.String() + " endpoint is invalid"
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, "openai-compatible " + c.capability.String() + " endpoint is unreachable; check the endpoint and that the gateway is running"
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes))

	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return true, ""
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		if c.apiKey == "" {
			return false, AuthRequiredReason(c.capability)
		}
		return false, VerifyKeyReason(c.capability, resp.StatusCode)
	}
	return false, fmt.Sprintf("openai-compatible %s endpoint returned HTTP %d", c.capability, resp.StatusCode)
}
