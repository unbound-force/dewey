package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"golang.org/x/oauth2/google"
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

	token, err := v.tokenFn(ctx)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.rawPredictURL(), bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("create vertex request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := v.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("vertex AI request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 50*1024*1024))
	if err != nil {
		return "", fmt.Errorf("read vertex response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("vertex AI error (HTTP %d): %s", resp.StatusCode, string(respBody))
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
