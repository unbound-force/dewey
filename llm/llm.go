// Package llm provides interfaces and implementations for generating
// natural language text from structured prompts. The primary implementation
// uses Ollama's HTTP API for local model inference via POST /api/generate.
//
// Design decision: Separate from embed.Embedder because embedding and
// synthesis use different Ollama API endpoints (/api/embed vs /api/generate),
// different models (embedding model vs generation model), and have different
// error modes. Single Responsibility Principle.
//
// This package is a leaf dependency — it MUST NOT import any other Dewey
// packages. Only stdlib + charmbracelet/log are allowed.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/charmbracelet/log"
)

// logger is the package-level structured logger for llm operations.
var logger = log.NewWithOptions(os.Stderr, log.Options{
	Prefix:          "dewey/llm",
	ReportTimestamp: true,
	TimeFormat:      "2006-01-02T15:04:05.000Z07:00",
})

// SetLogLevel sets the logging level for the llm package.
// Use log.DebugLevel for verbose output during diagnostics.
func SetLogLevel(level log.Level) {
	logger.SetLevel(level)
}

// SetLogOutput replaces the llm package logger with one that writes to
// the given writer at the given level. Used to enable file logging.
func SetLogOutput(w io.Writer, level log.Level) {
	newLogger := log.NewWithOptions(w, log.Options{
		Prefix:          "dewey/llm",
		Level:           level,
		ReportTimestamp: true,
		TimeFormat:      "2006-01-02T15:04:05.000Z07:00",
	})
	*logger = *newLogger
}

// Synthesizer generates natural language text from prompts. Used by the
// compile tool to synthesize compiled articles from clustered learnings.
// Implementations must be safe for concurrent use.
type Synthesizer interface {
	// Synthesize generates text from a prompt. Returns the generated text
	// or an error if generation fails (model unavailable, timeout, etc.).
	Synthesize(ctx context.Context, prompt string) (string, error)

	// Available reports whether the generation model is loaded and ready.
	// Returns false if the provider is unreachable or the model is not pulled.
	Available() bool

	// ModelID returns the model identifier used for text generation.
	ModelID() string
}

// OllamaSynthesizer implements Synthesizer using Ollama's HTTP API.
// Calls POST /api/generate for text generation.
//
// Design decision: Uses the same HTTP client pattern as OllamaEmbedder
// but targets a different endpoint and model. The generation model
// (e.g., "llama3.2:3b") is typically larger than the embedding model
// (e.g., "granite-embedding:30m").
type OllamaSynthesizer struct {
	baseURL string
	model   string
	client  *http.Client

	// Cache model availability to avoid repeated HTTP calls.
	// Invalidated periodically (every 30 seconds).
	mu            sync.RWMutex
	available     bool
	lastCheck     time.Time
	checkInterval time.Duration
}

// NewOllamaSynthesizer creates an OllamaSynthesizer that connects to
// the Ollama API at baseURL (e.g., "http://localhost:11434") using the
// specified generation model (e.g., "llama3.2:3b"). Returns a ready-to-use
// synthesizer with a 120-second HTTP timeout (generation is slower than
// embedding) and 30-second availability cache interval.
//
// The returned synthesizer is safe for concurrent use. Call
// [OllamaSynthesizer.Available] to check if the model is loaded before
// calling [OllamaSynthesizer.Synthesize].
func NewOllamaSynthesizer(baseURL, model string) *OllamaSynthesizer {
	return &OllamaSynthesizer{
		baseURL: baseURL,
		model:   model,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
		checkInterval: 30 * time.Second,
	}
}

// generateRequest is the JSON body sent to POST /api/generate.
type generateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

// generateResponse is the JSON response from POST /api/generate
// when stream=false.
type generateResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

// tagsResponse is the JSON response from GET /api/tags.
type tagsResponse struct {
	Models []tagModel `json:"models"`
}

// tagModel represents a single model entry in the tags response.
type tagModel struct {
	Name string `json:"name"`
}

// Synthesize generates text by calling Ollama's POST /api/generate
// endpoint with stream=false. Returns the complete generated text.
// Returns an error if the HTTP request fails, Ollama returns a non-200
// status, or the response cannot be parsed.
//
// The request respects context cancellation via http.NewRequestWithContext,
// allowing callers to set deadlines or cancel long-running generations.
func (o *OllamaSynthesizer) Synthesize(ctx context.Context, prompt string) (string, error) {
	reqBody := generateRequest{
		Model:  o.model,
		Prompt: prompt,
		Stream: false,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal generate request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create generate request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	logger.Debug("sending generate request", "model", o.model, "prompt_len", len(prompt))

	resp, err := o.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama generate request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Cap response body to 50MB to prevent unbounded memory allocation
	// from unexpectedly large generation responses.
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 50*1024*1024))
	if err != nil {
		return "", fmt.Errorf("read generate response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama generate returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var genResp generateResponse
	if err := json.Unmarshal(respBody, &genResp); err != nil {
		return "", fmt.Errorf("parse generate response: %w", err)
	}

	logger.Debug("generate complete", "model", o.model, "response_len", len(genResp.Response))

	return genResp.Response, nil
}

// Available reports whether the configured generation model is available
// in the Ollama instance by querying GET /api/tags. Returns false if
// Ollama is unreachable, returns a non-200 status, or the model is not
// in the list. Caches the result for 30 seconds to avoid excessive HTTP
// calls. Safe for concurrent use.
func (o *OllamaSynthesizer) Available() bool {
	o.mu.RLock()
	if time.Since(o.lastCheck) < o.checkInterval {
		avail := o.available
		o.mu.RUnlock()
		return avail
	}
	o.mu.RUnlock()

	// Cache expired — re-check.
	o.mu.Lock()
	defer o.mu.Unlock()

	// Double-check after acquiring write lock (another goroutine may have updated).
	if time.Since(o.lastCheck) < o.checkInterval {
		return o.available
	}

	o.available = o.checkModelAvailable()
	o.lastCheck = time.Now()
	return o.available
}

// ModelID returns the model identifier string (e.g., "llama3.2:3b")
// used for text generation.
func (o *OllamaSynthesizer) ModelID() string {
	return o.model
}

// checkModelAvailable queries GET /api/tags to see if the configured
// model is available in the Ollama instance.
func (o *OllamaSynthesizer) checkModelAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.baseURL+"/api/tags", nil)
	if err != nil {
		return false
	}

	resp, err := o.client.Do(req)
	if err != nil {
		return false // Ollama not running or unreachable.
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	// Cap response body to 50MB to prevent unbounded memory allocation.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 50*1024*1024))
	if err != nil {
		return false
	}

	var tags tagsResponse
	if err := json.Unmarshal(body, &tags); err != nil {
		return false
	}

	for _, m := range tags.Models {
		if m.Name == o.model {
			return true
		}
	}
	return false
}

// NoopSynthesizer is a test double that returns a fixed response.
// Used in tests to avoid requiring a running Ollama instance.
// Exported for use in tool tests across packages.
type NoopSynthesizer struct {
	// Response is the text returned by Synthesize.
	Response string
	// Err is the error returned by Synthesize.
	Err error
	// Model is the value returned by ModelID.
	Model string
	// Avail is the value returned by Available.
	Avail bool
}

// Synthesize returns the configured Response and Err.
func (n *NoopSynthesizer) Synthesize(_ context.Context, _ string) (string, error) {
	return n.Response, n.Err
}

// Available returns the configured Avail value.
func (n *NoopSynthesizer) Available() bool { return n.Avail }

// ModelID returns the configured Model string.
func (n *NoopSynthesizer) ModelID() string { return n.Model }
