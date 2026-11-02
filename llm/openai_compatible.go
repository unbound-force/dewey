package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Defaults for the OpenAI-compatible synthesis provider. These mirror the
// Vertex provider limits so callers get the same retry and output
// guarantees regardless of backend.
const (
	// openAISynthTimeout bounds a single chat-completions request.
	openAISynthTimeout = 300 * time.Second

	// openAISynthMaxTokens is the default output token budget. It is
	// deliberately generous: curation and compilation prompts routinely
	// exceed 4096 tokens of output (issue #76).
	openAISynthMaxTokens = 16000

	// openAISynthMaxRetries is the number of retries after the first
	// attempt when the provider returns HTTP 429.
	openAISynthMaxRetries = 5

	// openAISynthBaseDelay is the initial backoff delay for 429 retries.
	openAISynthBaseDelay = time.Second

	// openAISynthMaxDelay caps the backoff delay for 429 retries.
	openAISynthMaxDelay = 60 * time.Second

	// openAISynthMaxResponseBytes bounds a provider response body. Bodies
	// larger than this are rejected rather than silently truncated into a
	// confusing JSON parse error.
	openAISynthMaxResponseBytes = 50 * 1024 * 1024
)

// OpenAICompatibleSynthesizer implements Synthesizer against any provider
// that exposes the OpenAI chat-completions contract (for example enmaas at
// https://api.enmaas.devshift.net/v1).
//
// The API key is only ever placed in the Authorization header. It is never
// logged and is redacted from response-body excerpts included in errors.
type OpenAICompatibleSynthesizer struct {
	endpoint  string
	model     string
	apiKey    string
	maxTokens int
	client    *http.Client

	// Retry tuning. The constructor sets production defaults; tests may
	// shorten them to exercise retry behavior without real waits.
	baseDelay time.Duration
	maxDelay  time.Duration
}

// openAIChatRequest is the OpenAI chat-completions request body.
type openAIChatRequest struct {
	Model     string              `json:"model"`
	Messages  []openAIChatMessage `json:"messages"`
	MaxTokens int                 `json:"max_tokens"`
}

// openAIChatMessage is one message in a chat-completions conversation.
type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openAIChatResponse is the subset of the chat-completions response that
// Dewey needs. Choice is a named type so tests and callers can reason about
// it without anonymous structs.
type openAIChatResponse struct {
	Choices []openAIChatChoice `json:"choices"`
	Error   *openAIChatError   `json:"error,omitempty"`
}

// openAIChatChoice is one completion choice.
type openAIChatChoice struct {
	Message      openAIChatMessage `json:"message"`
	FinishReason string            `json:"finish_reason"`
}

// openAIChatError is the error envelope some compatible providers return
// with a non-2xx status or alongside a 200 response with no choices.
type openAIChatError struct {
	Message string `json:"message"`
}

// NewOpenAICompatibleSynthesizer creates a synthesizer for an
// OpenAI-compatible chat-completions endpoint.
//
// endpoint must be an absolute http or https URL, either a base URL such as
// "https://host/v1" or a full ".../chat/completions" URL. model and apiKey
// are required. A non-positive timeout falls back to 300 seconds and a
// non-positive maxTokens falls back to 16000.
func NewOpenAICompatibleSynthesizer(endpoint, model, apiKey string, timeout time.Duration, maxTokens int) (*OpenAICompatibleSynthesizer, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("openai-compatible synthesizer requires endpoint")
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("openai-compatible synthesizer requires an absolute http(s) endpoint without credentials, query, or fragment")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("openai-compatible synthesizer endpoint must not contain credentials, a query string, or a fragment")
	}
	if model == "" {
		return nil, fmt.Errorf("openai-compatible synthesizer requires model")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("openai-compatible synthesizer requires an API key (set DEWEY_SYNTHESIS_API_KEY or synthesis.api_key)")
	}
	if timeout <= 0 {
		timeout = openAISynthTimeout
	}
	if maxTokens <= 0 {
		maxTokens = openAISynthMaxTokens
	}
	if u.Scheme == "http" && !isLoopbackHost(u.Hostname()) {
		logger.Warn("openai-compatible synthesis endpoint is not HTTPS; the API key and prompt will be sent in cleartext",
			"host", u.Hostname())
	}

	return &OpenAICompatibleSynthesizer{
		endpoint:  strings.TrimRight(endpoint, "/"),
		model:     model,
		apiKey:    apiKey,
		maxTokens: maxTokens,
		client: &http.Client{
			Timeout: timeout,
			// Never follow redirects: a compromised or open-redirecting
			// gateway must not receive the bearer token or prompt elsewhere.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		baseDelay: openAISynthBaseDelay,
		maxDelay:  openAISynthMaxDelay,
	}, nil
}

// isLoopbackHost reports whether host names the local machine.
func isLoopbackHost(host string) bool {
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return strings.HasPrefix(host, "127.")
}

// chatURL returns the full chat-completions URL, tolerating endpoints that
// already include the path.
func (o *OpenAICompatibleSynthesizer) chatURL() string {
	if strings.HasSuffix(o.endpoint, "/chat/completions") {
		return o.endpoint
	}
	return o.endpoint + "/chat/completions"
}

// Synthesize sends prompt to the configured provider and returns the first
// assistant message. It retries HTTP 429 responses with bounded exponential
// backoff (honoring Retry-After) and fails loudly when the provider reports
// the response was truncated for lack of output tokens.
func (o *OpenAICompatibleSynthesizer) Synthesize(ctx context.Context, prompt string) (string, error) {
	reqBody := openAIChatRequest{
		Model: o.model,
		Messages: []openAIChatMessage{
			{Role: "user", Content: prompt},
		},
		MaxTokens: o.maxTokens,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal openai-compatible request: %w", err)
	}

	statusCode, respBody, err := o.doRequestWithRetry(ctx, bodyBytes)
	if err != nil {
		return "", err
	}

	if statusCode != http.StatusOK {
		return "", fmt.Errorf("openai-compatible synthesis error (HTTP %d): %s", statusCode, o.truncateBody(respBody))
	}

	var result openAIChatResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse openai-compatible response: %s", o.redact(err.Error()))
	}
	if result.Error != nil && result.Error.Message != "" {
		return "", fmt.Errorf("openai-compatible synthesis error: %s", o.redact(result.Error.Message))
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("openai-compatible response contained no choices")
	}

	choice := result.Choices[0]
	if choice.FinishReason == "length" {
		return "", fmt.Errorf("openai-compatible response was truncated (finish_reason=length); increase synthesis.max_tokens (currently %d) or reduce the prompt size", o.maxTokens)
	}
	if strings.TrimSpace(choice.Message.Content) == "" {
		return "", fmt.Errorf("openai-compatible response contained no text content")
	}

	return choice.Message.Content, nil
}

// doRequestWithRetry sends the chat request, retrying only HTTP 429 with
// exponential backoff. The returned body is not redacted; callers must use
// truncateBody or redact before surfacing it.
func (o *OpenAICompatibleSynthesizer) doRequestWithRetry(ctx context.Context, bodyBytes []byte) (int, []byte, error) {
	for attempt := range openAISynthMaxRetries + 1 {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.chatURL(), bytes.NewReader(bodyBytes))
		if err != nil {
			return 0, nil, fmt.Errorf("create openai-compatible request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+o.apiKey)

		resp, err := o.client.Do(req)
		if err != nil {
			// Never wrap a transport error with request headers; the error
			// itself cannot contain the key because http.Client redacts it.
			return 0, nil, fmt.Errorf("openai-compatible request: %w", err)
		}

		respBody, err := readLimitedBody(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return 0, nil, fmt.Errorf("read openai-compatible response: %w", err)
		}

		if resp.StatusCode != http.StatusTooManyRequests {
			return resp.StatusCode, respBody, nil
		}

		if attempt == openAISynthMaxRetries {
			return 0, nil, fmt.Errorf("openai-compatible provider rate limited (HTTP 429) after %d retries: %s",
				openAISynthMaxRetries, o.truncateBody(respBody))
		}

		delay := synthRetryDelay(attempt, resp.Header.Get("Retry-After"), o.baseDelay, o.maxDelay)
		logger.Warn("openai-compatible provider rate limited, retrying", "attempt", attempt+1, "delay", delay)

		select {
		case <-ctx.Done():
			return 0, nil, ctx.Err()
		case <-time.After(delay):
		}
	}
	// Unreachable — the loop always returns.
	return 0, nil, fmt.Errorf("openai-compatible retry loop exited unexpectedly")
}

// readLimitedBody reads a response body up to openAISynthMaxResponseBytes and
// returns an explicit error when the body exceeds the limit.
func readLimitedBody(r io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, openAISynthMaxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > openAISynthMaxResponseBytes {
		return nil, fmt.Errorf("response exceeded %d byte limit", openAISynthMaxResponseBytes)
	}
	return body, nil
}

// truncateBody redacts the API key and bounds a response body excerpt so it
// is safe to include in an error.
func (o *OpenAICompatibleSynthesizer) truncateBody(body []byte) string {
	text := o.redact(string(body))
	if runes := []rune(text); len(runes) > 512 {
		text = string(runes[:512]) + "... (truncated)"
	}
	return text
}

// redact removes the configured API key from s.
func (o *OpenAICompatibleSynthesizer) redact(s string) string {
	if o.apiKey == "" {
		return s
	}
	s = strings.ReplaceAll(s, o.apiKey, "[REDACTED]")
	// Providers may echo credentials in URL-encoded form.
	return strings.ReplaceAll(s, url.QueryEscape(o.apiKey), "[REDACTED]")
}

// Available reports whether the provider is fully configured. It performs no
// network call: endpoint, model, and API key presence are validated at
// construction. This mirrors the Vertex provider's credential-presence check
// and keeps server startup free of provider latency.
func (o *OpenAICompatibleSynthesizer) Available() bool {
	return o.endpoint != "" && o.model != "" && o.apiKey != ""
}

// ModelID returns the configured model identifier.
func (o *OpenAICompatibleSynthesizer) ModelID() string {
	return o.model
}

// UnavailableReason reports why the synthesizer is unavailable, or "" when
// it is available. Construction validates endpoint, model, and API key, so
// every synthesizer built by NewOpenAICompatibleSynthesizer is configured;
// only a zero-value synthesizer reports a reason.
func (o *OpenAICompatibleSynthesizer) UnavailableReason() string {
	if o.endpoint == "" || o.model == "" || o.apiKey == "" {
		return "synthesizer is not fully configured"
	}
	return ""
}
