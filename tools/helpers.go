package tools

import (
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/unbound-force/dewey/v3/embed"
	"github.com/unbound-force/dewey/v3/llm"
)

// textResult creates a successful text CallToolResult.
func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}
}

// errorResult creates an error CallToolResult (visible to the LLM for self-correction).
func errorResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: msg}},
		IsError: true,
	}
}

// jsonTextResult marshals any value to indented JSON and wraps it as text content.
func jsonTextResult(v any) (*mcp.CallToolResult, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal result: %w", err)
	}
	return textResult(string(data)), nil
}

// jsonRawTextResult pretty-prints raw JSON bytes.
func jsonRawTextResult(raw json.RawMessage) (*mcp.CallToolResult, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return textResult(string(raw)), nil
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return textResult(string(raw)), nil
	}
	return textResult(string(data)), nil
}

// embeddingUnavailableMessage builds the actionable error text for the
// semantic tools. It includes the provider's own unavailable reason when
// the embedder implements embed.UnavailableReasoner, and never assumes
// Ollama (FR-015, SC-004).
func embeddingUnavailableMessage(e embed.Embedder) string {
	if e != nil {
		if reasoner, ok := e.(embed.UnavailableReasoner); ok {
			if reason := reasoner.UnavailableReason(); reason != "" {
				return "Semantic search unavailable: " + reason + "."
			}
		}
	}
	return "Semantic search unavailable: embedding model not loaded. Ensure the embedding provider is running with the configured model."
}

// synthesisUnavailableReason returns the provider's unavailable reason
// when known, otherwise a generic actionable fallback. Prompt mode still
// returns every prompt so callers can synthesize manually (FR-015).
func synthesisUnavailableReason(s llm.Synthesizer) string {
	if s == nil {
		return "synthesis provider is not configured"
	}
	if reasoner, ok := s.(llm.UnavailableReasoner); ok {
		if reason := reasoner.UnavailableReason(); reason != "" {
			return reason
		}
	}
	return "synthesis model is not available"
}
