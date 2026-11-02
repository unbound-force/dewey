package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/unbound-force/dewey/v3/openaicompat"
)

// OpenAICompatibleSynthesizer implements Synthesizer against an
// OpenAI-compatible gateway (OpenRouter, ENMaas, LiteLLM, vLLM, or a
// self-hosted server).
//
// It sends one non-streaming POST {endpoint}/chat/completions request
// with a single user message, passes the configured model verbatim, and
// reads choices[0].message.content. Authentication is environment-only
// and resolved by openaicompat (FR-006, FR-007).
type OpenAICompatibleSynthesizer struct {
	client *openaicompat.Client
	model  string
}

// chatMessage is one entry in the OpenAI chat messages array.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatRequest is the JSON body sent to POST /chat/completions. No tools,
// functions, response formats, or max_tokens are sent (FR-021).
type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

// chatResponse is the subset of the completion response Dewey reads.
type chatResponse struct {
	Choices []chatChoice `json:"choices"`
}

// chatChoice is one completion choice.
type chatChoice struct {
	Message chatMessage `json:"message"`
}

// NewOpenAICompatibleSynthesizer validates the required configuration
// and returns a synthesis provider for the gateway. A missing endpoint
// names synthesis.endpoint and a missing model names synthesis.model
// (FR-009, US1-7). A missing API key is allowed so unauthenticated
// self-hosted gateways work (FR-008).
func NewOpenAICompatibleSynthesizer(endpoint, model string) (*OpenAICompatibleSynthesizer, error) {
	if strings.TrimSpace(endpoint) == "" {
		return nil, fmt.Errorf("openai-compatible synthesis provider requires endpoint (synthesis.endpoint)")
	}
	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("openai-compatible synthesis provider requires model (synthesis.model)")
	}

	client, err := openaicompat.NewClient(openaicompat.Config{
		Capability: openaicompat.CapabilitySynthesis,
		Endpoint:   endpoint,
	})
	if err != nil {
		return nil, err
	}
	return &OpenAICompatibleSynthesizer{client: client, model: model}, nil
}

// Synthesize sends the prompt as a single user message and returns the
// first choice's assistant content. Empty or missing content is an error
// so an empty article is never persisted (US1-6).
func (s *OpenAICompatibleSynthesizer) Synthesize(ctx context.Context, prompt string) (string, error) {
	payload := chatRequest{
		Model:    s.model,
		Messages: []chatMessage{{Role: "user", Content: prompt}},
		Stream:   false,
	}

	body, err := s.client.PostJSON(ctx, openaicompat.PathChatCompletions, payload)
	if err != nil {
		return "", err
	}

	var response chatResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", openaicompat.ParseError(openaicompat.CapabilitySynthesis, err)
	}
	if len(response.Choices) == 0 {
		return "", fmt.Errorf("openai-compatible synthesis provider returned no choices")
	}

	content := response.Choices[0].Message.Content
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("openai-compatible synthesis provider returned empty content")
	}
	return content, nil
}

// Available reports whether the gateway answered the cached /models
// probe.
func (s *OpenAICompatibleSynthesizer) Available() bool { return s.client.Available() }

// ModelID returns the configured gateway model identifier.
func (s *OpenAICompatibleSynthesizer) ModelID() string { return s.model }

// UnavailableReason reports why the gateway is unavailable, or "" when it
// is available.
func (s *OpenAICompatibleSynthesizer) UnavailableReason() string {
	return s.client.UnavailableReason()
}
