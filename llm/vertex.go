package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"golang.org/x/oauth2/google"
)

const (
	// vertexSynthMaxRetries is the maximum number of retry attempts on 429 responses.
	vertexSynthMaxRetries = 5
	// vertexSynthBaseDelay is the initial backoff delay before the first retry.
	vertexSynthBaseDelay = 1 * time.Second
	// vertexSynthMaxDelay caps the exponential backoff to prevent excessive waits.
	vertexSynthMaxDelay = 60 * time.Second
)

// VertexSynthesizer implements Synthesizer using Google Vertex AI's rawPredict API.
// Sends requests in Anthropic Messages format to Claude models hosted on Vertex AI.
// Uses application-default credentials for authentication (no CGO).
//
// Design decision D4 (pluggable-providers design.md): Direct REST calls to
// the rawPredict endpoint using Anthropic Messages format. Tested successfully
// with claude-opus-4-6 returning results in ~31 seconds.
type VertexSynthesizer struct {
	project string
	region  string
	model   string
	client  *http.Client
	tokenFn func(ctx context.Context) (string, error)

	// Cached availability check.
	mu          sync.RWMutex
	available   bool
	lastCheck   time.Time
	checkExpiry time.Duration
}

// vertexSynthRequest is the Anthropic Messages format wrapped for Vertex rawPredict.
type vertexSynthRequest struct {
	AnthropicVersion string          `json:"anthropic_version"`
	MaxTokens        int             `json:"max_tokens"`
	Messages         []vertexMessage `json:"messages"`
}

// vertexMessage represents a message in the Anthropic Messages format.
type vertexMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// vertexSynthResponse is the Anthropic Messages response from Vertex rawPredict.
type vertexSynthResponse struct {
	Content []vertexContentBlock `json:"content"`
}

// vertexContentBlock contains the text output from synthesis.
type vertexContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// NewVertexSynthesizer creates a VertexSynthesizer for the given GCP project,
// region, and model (e.g., "claude-sonnet-4-6"). Returns an error if required
// fields are missing.
func NewVertexSynthesizer(project, region, model string) (*VertexSynthesizer, error) {
	if model == "" {
		return nil, fmt.Errorf("vertex synthesizer requires model")
	}
	v := &VertexSynthesizer{
		project: project,
		region:  region,
		model:   model,
		client: &http.Client{
			Timeout: 120 * time.Second, // Match OllamaSynthesizer timeout.
		},
		checkExpiry: 30 * time.Second,
	}
	v.tokenFn = v.defaultGetToken
	return v, nil
}

// rawPredictURL builds the Vertex AI rawPredict endpoint URL for Anthropic models.
func (v *VertexSynthesizer) rawPredictURL() string {
	return fmt.Sprintf(
		"https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/anthropic/models/%s:rawPredict",
		v.region, v.project, v.region, v.model,
	)
}

// defaultGetToken retrieves an OAuth2 access token using application-default credentials.
func (v *VertexSynthesizer) defaultGetToken(ctx context.Context) (string, error) {
	creds, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return "", fmt.Errorf("vertex AI credentials: %w (run 'gcloud auth application-default login --scopes=https://www.googleapis.com/auth/cloud-platform')", err)
	}
	tok, err := creds.TokenSource.Token()
	if err != nil {
		return "", fmt.Errorf("vertex AI token: %w", err)
	}
	return tok.AccessToken, nil
}

// Synthesize generates text from a prompt via Vertex AI Claude.
// Retries up to 5 times on HTTP 429 (Too Many Requests) with exponential backoff.
func (v *VertexSynthesizer) Synthesize(ctx context.Context, prompt string) (string, error) {
	reqBody := vertexSynthRequest{
		AnthropicVersion: "vertex-2023-10-16",
		MaxTokens:        4096,
		Messages: []vertexMessage{
			{Role: "user", Content: prompt},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal vertex request: %w", err)
	}

	statusCode, respBody, err := v.doRequestWithRetry(ctx, v.rawPredictURL(), bodyBytes)
	if err != nil {
		return "", err
	}

	if statusCode != http.StatusOK {
		return "", fmt.Errorf("vertex AI error (HTTP %d): %s", statusCode, string(respBody))
	}

	var result vertexSynthResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse vertex response: %w", err)
	}

	// Extract text from the first content block.
	for _, block := range result.Content {
		if block.Type == "text" {
			return block.Text, nil
		}
	}

	return "", fmt.Errorf("vertex AI response contained no text content")
}

// doRequestWithRetry sends an HTTP POST to url with bodyBytes, retrying on
// HTTP 429 with exponential backoff. Returns the final status code, response
// body, and any transport-level error.
func (v *VertexSynthesizer) doRequestWithRetry(ctx context.Context, url string, bodyBytes []byte) (int, []byte, error) {
	for attempt := range vertexSynthMaxRetries + 1 {
		token, err := v.tokenFn(ctx)
		if err != nil {
			return 0, nil, err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
		if err != nil {
			return 0, nil, fmt.Errorf("create vertex request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := v.client.Do(req)
		if err != nil {
			return 0, nil, fmt.Errorf("vertex AI request: %w", err)
		}

		respBody, err := io.ReadAll(io.LimitReader(resp.Body, 50*1024*1024))
		_ = resp.Body.Close()
		if err != nil {
			return 0, nil, fmt.Errorf("read vertex response: %w", err)
		}

		if resp.StatusCode != http.StatusTooManyRequests {
			return resp.StatusCode, respBody, nil
		}

		if attempt == vertexSynthMaxRetries {
			return 0, nil, fmt.Errorf("vertex AI rate limited (HTTP 429) after %d retries: %s", vertexSynthMaxRetries, string(respBody))
		}

		delay := vertexSynthRetryDelay(attempt, resp.Header.Get("Retry-After"))
		log.Warn("vertex AI rate limited, retrying", "attempt", attempt+1, "delay", delay)

		select {
		case <-ctx.Done():
			return 0, nil, ctx.Err()
		case <-time.After(delay):
		}
	}
	// Unreachable — the loop always returns or breaks.
	return 0, nil, fmt.Errorf("vertex AI retry loop exited unexpectedly")
}

// vertexSynthRetryDelay computes the backoff delay for a retry attempt.
// If the Retry-After header contains a valid number of seconds, that
// value is used (capped at vertexSynthMaxDelay). Otherwise, exponential
// backoff with jitter is applied: baseDelay * 2^attempt ± 25%, capped at
// vertexSynthMaxDelay. Jitter prevents thundering herd when multiple
// goroutines retry simultaneously. Only integer-seconds Retry-After format
// is supported; HTTP-date format falls back to exponential backoff.
func vertexSynthRetryDelay(attempt int, retryAfter string) time.Duration {
	if retryAfter != "" {
		if secs, err := strconv.Atoi(retryAfter); err == nil && secs > 0 {
			d := time.Duration(secs) * time.Second
			if d > vertexSynthMaxDelay {
				d = vertexSynthMaxDelay
			}
			return d
		}
	}
	d := time.Duration(float64(vertexSynthBaseDelay) * math.Pow(2, float64(attempt)))
	if d > vertexSynthMaxDelay {
		d = vertexSynthMaxDelay
	}
	// Add ±25% jitter to desynchronize concurrent retries.
	jitter := time.Duration(rand.Int64N(int64(d)/2)) - d/4
	d += jitter
	if d < 0 {
		d = 0
	}
	return d
}

// Available reports whether Vertex AI credentials are configured.
// Caches the result for 30 seconds.
func (v *VertexSynthesizer) Available() bool {
	v.mu.RLock()
	if time.Since(v.lastCheck) < v.checkExpiry {
		avail := v.available
		v.mu.RUnlock()
		return avail
	}
	v.mu.RUnlock()

	v.mu.Lock()
	defer v.mu.Unlock()

	if time.Since(v.lastCheck) < v.checkExpiry {
		return v.available
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := v.tokenFn(ctx)
	v.available = err == nil
	v.lastCheck = time.Now()
	return v.available
}

// ModelID returns the configured Vertex AI model identifier.
func (v *VertexSynthesizer) ModelID() string {
	return v.model
}
