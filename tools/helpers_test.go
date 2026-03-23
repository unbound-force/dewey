package tools

import (
	"encoding/json"
	"testing"
)

func TestTextResult(t *testing.T) {
	r := textResult("hello world")
	if r == nil {
		t.Fatal("textResult returned nil")
	}
	if r.IsError {
		t.Error("textResult should not be an error")
	}
	if len(r.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(r.Content))
	}
}

func TestErrorResult(t *testing.T) {
	r := errorResult("something failed")
	if r == nil {
		t.Fatal("errorResult returned nil")
	}
	if !r.IsError {
		t.Error("errorResult should be an error")
	}
	if len(r.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(r.Content))
	}
}

func TestJsonTextResult(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		wantErr bool
	}{
		{
			name:  "simple map",
			input: map[string]any{"key": "value", "count": 42},
		},
		{
			name:  "slice",
			input: []string{"a", "b", "c"},
		},
		{
			name:  "nested struct",
			input: struct{ Name string }{"test"},
		},
		{
			name:  "nil",
			input: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := jsonTextResult(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("jsonTextResult() error: %v", err)
			}
			if r == nil {
				t.Fatal("jsonTextResult returned nil")
			}
			if r.IsError {
				t.Error("jsonTextResult should not be an error")
			}
			if len(r.Content) != 1 {
				t.Fatalf("expected 1 content item, got %d", len(r.Content))
			}
		})
	}
}

func TestJsonRawTextResult(t *testing.T) {
	tests := []struct {
		name  string
		input json.RawMessage
	}{
		{
			name:  "valid JSON object",
			input: json.RawMessage(`{"key": "value"}`),
		},
		{
			name:  "valid JSON array",
			input: json.RawMessage(`[1, 2, 3]`),
		},
		{
			name:  "invalid JSON falls back to raw string",
			input: json.RawMessage(`not json at all`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := jsonRawTextResult(tt.input)
			if err != nil {
				t.Fatalf("jsonRawTextResult() error: %v", err)
			}
			if r == nil {
				t.Fatal("jsonRawTextResult returned nil")
			}
			if r.IsError {
				t.Error("jsonRawTextResult should not be an error")
			}
			if len(r.Content) != 1 {
				t.Fatalf("expected 1 content item, got %d", len(r.Content))
			}
		})
	}
}

func TestFormatResults_ViaJsonTextResult(t *testing.T) {
	// jsonTextResult is the primary formatting function used by all tools.
	// Verify it produces valid, re-parseable JSON.
	input := map[string]any{
		"query":   "test search",
		"count":   3,
		"results": []string{"a", "b", "c"},
	}

	r, err := jsonTextResult(input)
	if err != nil {
		t.Fatalf("jsonTextResult() error: %v", err)
	}

	// The content should be valid JSON we can unmarshal.
	tc := r.Content[0]
	// Type assert to get text
	type hasText interface{ GetText() string }
	if textC, ok := tc.(hasText); ok {
		var parsed map[string]any
		if err := json.Unmarshal([]byte(textC.GetText()), &parsed); err != nil {
			t.Fatalf("result is not valid JSON: %v", err)
		}
		if parsed["query"] != "test search" {
			t.Errorf("query = %v, want %q", parsed["query"], "test search")
		}
		if parsed["count"] != float64(3) {
			t.Errorf("count = %v, want 3", parsed["count"])
		}
	}
}
