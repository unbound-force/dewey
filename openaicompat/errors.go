package openaicompat

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	// MaxExcerptBytes caps the response-body excerpt in an error (FR-020).
	MaxExcerptBytes = 512
	// truncationMarker is appended when the excerpt was capped.
	truncationMarker = "... (truncated)"
	// redactionToken replaces every occurrence of the resolved key.
	redactionToken = "[REDACTED]"
)

// FormatError builds the FR-020 error for a non-success HTTP response:
// "openai-compatible <capability> error (HTTP <status>): <excerpt>". The
// excerpt is sanitized, has every occurrence of key replaced with
// [REDACTED] before truncation, and is capped at MaxExcerptBytes.
func FormatError(capability Capability, status int, body []byte, key string) error {
	return fmt.Errorf(
		"openai-compatible %s error (HTTP %d): %s",
		capability, status, Excerpt(body, key),
	)
}

// RateLimitError builds the FR-012 error returned when the retry budget
// is exhausted on HTTP 429 responses.
func RateLimitError(capability Capability, body []byte, key string) error {
	return fmt.Errorf(
		"openai-compatible %s rate limited (HTTP 429) after %d retries: %s",
		capability, MaxRetries, Excerpt(body, key),
	)
}

// ParseError wraps a response-decoding failure with the provider name so
// malformed gateway payloads are attributable (wire contract error
// conditions).
func ParseError(capability Capability, err error) error {
	return fmt.Errorf("openai-compatible %s malformed response: %w", capability, err)
}

// Excerpt returns a bounded, credential-free, log-safe excerpt of a
// response body. Control characters and ESC-initiated escape sequences
// are stripped first, then every occurrence of key is redacted, then the
// result is capped at MaxExcerptBytes with a truncation marker. The
// cap backs up to the last UTF-8 rune boundary so the excerpt is always
// valid UTF-8.
//
// Redaction precedes truncation deliberately: a key straddling the cap
// boundary cannot leak a prefix (FR-020).
func Excerpt(body []byte, key string) string {
	cleaned := Sanitize(string(body))
	if key != "" {
		cleaned = strings.ReplaceAll(cleaned, key, redactionToken)
	}
	if len(cleaned) > MaxExcerptBytes {
		// Back the cut up to the last rune boundary so truncation
		// cannot split a multi-byte UTF-8 sequence into invalid
		// bytes (mojibake in logs).
		cut := MaxExcerptBytes
		for cut > 0 && !utf8.RuneStart(cleaned[cut]) {
			cut--
		}
		cleaned = cleaned[:cut] + truncationMarker
	}
	return cleaned
}

// Sanitize strips C0 control characters (including newlines, tabs,
// carriage returns, and backspaces), DEL, and ESC-initiated escape
// sequences from s so gateway-controlled bytes cannot forge log lines or
// corrupt terminal rendering (FR-020).
func Sanitize(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == 0x1b: // ESC: skip the whole escape sequence.
			i += escapeSequenceLength(s[i:])
		case c < 0x20 || c == 0x7f:
			// Remaining C0 controls and DEL are dropped.
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// escapeSequenceLength returns the index (relative to the start of s,
// which begins with ESC) of the last byte belonging to the escape
// sequence. The caller adds it to its loop index.
func escapeSequenceLength(s string) int {
	if len(s) < 2 {
		return 0
	}
	switch s[1] {
	case '[': // CSI: parameter/intermediate bytes then a final byte.
		for i := 2; i < len(s); i++ {
			if s[i] >= 0x40 && s[i] <= 0x7e {
				return i
			}
		}
		return len(s) - 1
	case ']': // OSC: terminated by BEL or ST (ESC \).
		for i := 2; i < len(s); i++ {
			if s[i] == 0x07 {
				return i
			}
			if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' {
				return i + 1
			}
		}
		return len(s) - 1
	default: // Two-byte escape sequence.
		return 1
	}
}
