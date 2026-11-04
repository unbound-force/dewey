package openaicompat

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestExcerpt_CapAndMarker(t *testing.T) {
	body := []byte(strings.Repeat("a", 600))
	got := Excerpt(body, "")
	if !strings.HasSuffix(got, truncationMarker) {
		t.Fatalf("expected truncation marker, got %q", got)
	}
	if !strings.HasPrefix(got, strings.Repeat("a", MaxExcerptBytes)) {
		t.Fatalf("expected %d-byte prefix", MaxExcerptBytes)
	}
}

func TestExcerpt_ShortBodyUnchanged(t *testing.T) {
	if got := Excerpt([]byte("boom"), ""); got != "boom" {
		t.Fatalf("got %q, want boom", got)
	}
}

func TestExcerpt_RedactsRepeatedKey(t *testing.T) {
	key := "sk-test-secret"
	got := Excerpt([]byte("bad "+key+" and "+key), key)
	if strings.Contains(got, key) {
		t.Fatalf("key leaked: %q", got)
	}
	if strings.Count(got, redactionToken) != 2 {
		t.Fatalf("expected two redactions, got %q", got)
	}
}

func TestExcerpt_RedactsBeforeTruncation(t *testing.T) {
	key := "sk-straddling-secret"
	body := []byte(strings.Repeat("a", MaxExcerptBytes-5) + key + strings.Repeat("b", 200))
	got := Excerpt(body, key)
	if strings.Contains(got, key) {
		t.Fatalf("full key leaked: %q", got)
	}
	// No prefix of the key long enough to be meaningful may survive.
	for i := 1; i <= len(key); i++ {
		if strings.Contains(got, key[:i]) && i >= 6 {
			t.Fatalf("key prefix %q leaked across the truncation boundary: %q", key[:i], got)
		}
	}
}

func TestExcerpt_TruncatesAtRuneBoundary(t *testing.T) {
	// A multi-byte rune straddling the cap must be excluded whole, not
	// sliced mid-sequence into invalid UTF-8.
	tests := []struct {
		name      string
		straddle  string
		prefixLen int
	}{
		{name: "two byte rune", straddle: "é", prefixLen: MaxExcerptBytes - 1},
		{name: "three byte rune", straddle: "世", prefixLen: MaxExcerptBytes - 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(strings.Repeat("a", tt.prefixLen) + tt.straddle + strings.Repeat("b", 64))
			got := Excerpt(body, "")
			if !utf8.ValidString(got) {
				t.Fatalf("excerpt is not valid UTF-8: %q", got)
			}
			if !strings.HasSuffix(got, truncationMarker) {
				t.Fatalf("expected truncation marker, got %q", got)
			}
			capped := strings.TrimSuffix(got, truncationMarker)
			if capped != strings.Repeat("a", tt.prefixLen) {
				t.Fatalf("expected %d-byte prefix without partial rune, got %q", tt.prefixLen, capped)
			}
		})
	}
}

func TestSanitize_ControlCharacters(t *testing.T) {
	input := "a\nb\tc\rd\x00e\x7ff\x08g"
	if got := Sanitize(input); got != "abcdefg" {
		t.Fatalf("got %q, want abcdefg", got)
	}
}

func TestSanitize_EscapeSequences(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "csi color", input: "\x1b[31mred\x1b[0m", want: "red"},
		{name: "osc title", input: "\x1b]0;title\x07text", want: "text"},
		{name: "osc string terminator", input: "\x1b]0;title\x1b\\text", want: "text"},
		{name: "two byte escape", input: "\x1bMtext", want: "text"},
		{name: "unterminated csi", input: "pre\x1b[31", want: "pre"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Sanitize(tt.input); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSanitize_PreservesUTF8(t *testing.T) {
	input := "héllo — 世界"
	if got := Sanitize(input); got != input {
		t.Fatalf("got %q, want %q", got, input)
	}
}

func TestFormatError_Shape(t *testing.T) {
	err := FormatError(CapabilityEmbedding, 500, []byte("boom"), "sk-secret")
	msg := err.Error()
	if !strings.HasPrefix(msg, "openai-compatible embedding error (HTTP 500): ") {
		t.Fatalf("unexpected error shape: %q", msg)
	}
	if !strings.Contains(msg, "boom") {
		t.Fatalf("missing excerpt: %q", msg)
	}
	if strings.Contains(msg, "sk-secret") {
		t.Fatalf("key leaked: %q", msg)
	}
	if strings.Contains(msg, "Bearer") || strings.Contains(msg, "http://") || strings.Contains(msg, "https://") {
		t.Fatalf("error must not include headers or URLs: %q", msg)
	}
}

func TestRateLimitError_Shape(t *testing.T) {
	err := RateLimitError(CapabilityEmbedding, []byte("slow down"), "sk-secret")
	msg := err.Error()
	if !strings.Contains(msg, "rate limited (HTTP 429) after 5 retries") {
		t.Fatalf("unexpected rate-limit error: %q", msg)
	}
	if strings.Contains(msg, "sk-secret") {
		t.Fatalf("key leaked: %q", msg)
	}
}

func TestParseError_NamesProvider(t *testing.T) {
	err := ParseError(CapabilityEmbedding, errors.New("unexpected end of JSON input"))
	if !strings.Contains(err.Error(), "openai-compatible embedding malformed response") {
		t.Fatalf("parse error must name the provider: %q", err)
	}
}
