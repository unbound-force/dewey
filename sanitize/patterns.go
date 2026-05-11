package sanitize

import (
	"fmt"
	"regexp"
	"strings"
)

// DefaultPatternVersion is the current version of the default pattern
// database. Used by dewey lint to detect stale findings from older
// pattern versions. Incremented when patterns are added or modified.
const DefaultPatternVersion = 1

// PatternRule defines a single adversarial keyword pattern for injection
// scanning. Each rule contains a compiled regex, metadata for reporting,
// and a version number for staleness detection.
type PatternRule struct {
	// Regex is the compiled regular expression used to match the pattern
	// against document content.
	Regex *regexp.Regexp

	// Name is a human-readable identifier for the pattern (e.g.,
	// "instruction-override").
	Name string

	// Severity classifies the pattern's threat level: "critical", "high",
	// "medium", "low", or "info".
	Severity string

	// Description explains what the pattern detects and why it matters.
	Description string

	// Version is a monotonic integer for staleness detection. When
	// patterns are updated in a new release, the version increments.
	Version int
}

// DefaultPatterns is the default pattern database for adversarial keyword
// scanning. Patterns are grouped by severity:
//
//   - critical: High-specificity multi-word phrases that almost certainly
//     indicate prompt injection attempts (instruction overrides, system
//     prompt delimiters).
//   - high: Role reassignment phrases and delimiter injection that strongly
//     suggest adversarial intent.
//   - medium: Context phrases that may appear in legitimate security
//     documentation but warrant attention.
//
// Design decision: Single-word context phrases are classified at medium
// severity to reduce false positive noise on general technical documentation.
// Only multi-pattern combinations or high-specificity phrases warrant
// critical severity (per FR-SAN-002).
var DefaultPatterns = []PatternRule{
	// Critical: Instruction override phrases — high-specificity multi-word
	// patterns that almost certainly indicate prompt injection.
	{
		Regex:       regexp.MustCompile(`(?i)ignore (?:all )?(?:previous|prior|above) (?:instructions|context|rules)`),
		Name:        "instruction-override",
		Severity:    "critical",
		Description: "Detects attempts to override prior instructions, a common prompt injection technique.",
		Version:     1,
	},
	// Critical: System prompt delimiters — ChatML, Llama, and system
	// prompt markers that indicate raw prompt template injection.
	{
		Regex:       regexp.MustCompile(`<\|im_start\|>`),
		Name:        "chatml-delimiter",
		Severity:    "critical",
		Description: "Detects ChatML system prompt delimiters injected into content.",
		Version:     1,
	},
	{
		Regex:       regexp.MustCompile(`\[INST\]`),
		Name:        "llama-inst-delimiter",
		Severity:    "critical",
		Description: "Detects Llama [INST] prompt delimiters injected into content.",
		Version:     1,
	},
	{
		Regex:       regexp.MustCompile(`<<SYS>>`),
		Name:        "llama-sys-delimiter",
		Severity:    "critical",
		Description: "Detects Llama <<SYS>> system prompt delimiters injected into content.",
		Version:     1,
	},

	// High: Role reassignment phrases — strongly suggest adversarial
	// intent to change the model's behavior.
	{
		Regex:       regexp.MustCompile(`(?i)you are now`),
		Name:        "role-reassignment-now",
		Severity:    "high",
		Description: "Detects role reassignment attempts using 'you are now' phrasing.",
		Version:     1,
	},
	{
		Regex:       regexp.MustCompile(`(?i)pretend (?:to be|you are)`),
		Name:        "role-reassignment-pretend",
		Severity:    "high",
		Description: "Detects role reassignment attempts using 'pretend to be' or 'pretend you are' phrasing.",
		Version:     1,
	},
	// High: Delimiter injection — system/user/assistant role markers at
	// line start that mimic chat turn boundaries.
	{
		Regex:       regexp.MustCompile(`(?m)^(?:system|user|assistant):`),
		Name:        "delimiter-injection",
		Severity:    "high",
		Description: "Detects chat role delimiter injection (system:, user:, assistant:) at line start.",
		Version:     1,
	},

	// Medium: Context phrases — may appear in legitimate security docs
	// but warrant attention when found in external content.
	{
		Regex:       regexp.MustCompile(`(?i)act as if`),
		Name:        "context-act-as-if",
		Severity:    "medium",
		Description: "Detects 'act as if' context manipulation phrases.",
		Version:     1,
	},
	{
		Regex:       regexp.MustCompile(`(?i)system (?:prompt|message)`),
		Name:        "context-system-prompt",
		Severity:    "medium",
		Description: "Detects references to system prompt or system message that may indicate prompt extraction attempts.",
		Version:     1,
	},
	{
		Regex:       regexp.MustCompile(`(?i)###\s*(?:system|instruction)`),
		Name:        "context-markdown-system",
		Severity:    "medium",
		Description: "Detects Markdown heading markers used to inject system-level instructions.",
		Version:     1,
	},
}

// ScanInjectionPatterns scans content for known prompt injection patterns
// using compiled regular expressions. Returns a Finding for each match with
// the pattern name, line number, severity, and surrounding context (up to
// 200 characters).
//
// The function iterates over all patterns, finds all matches using
// FindAllStringIndex, computes line numbers from byte offsets, and extracts
// surrounding context for human review.
//
// When patterns is nil or empty, returns nil (no findings).
func ScanInjectionPatterns(content string, patterns []PatternRule) []Finding {
	if len(patterns) == 0 {
		return nil
	}

	var findings []Finding

	for _, rule := range patterns {
		if rule.Regex == nil {
			continue
		}

		matches := rule.Regex.FindAllStringIndex(content, -1)
		for _, loc := range matches {
			matchStart := loc[0]
			matchEnd := loc[1]

			// Compute 1-indexed line number by counting newlines before
			// the match start position.
			line := 1 + strings.Count(content[:matchStart], "\n")

			// Extract up to 200 characters of surrounding context centered
			// on the match for human review.
			context := extractContext(content, matchStart, matchEnd, 200)

			findings = append(findings, Finding{
				Pattern:  rule.Name,
				Line:     line,
				Severity: rule.Severity,
				Category: "injection",
				Context:  context,
				Message:  fmt.Sprintf("Injection pattern '%s' detected (severity: %s)", rule.Name, rule.Severity),
			})
		}
	}

	return findings
}

// extractContext extracts up to maxLen characters of surrounding content
// centered on the match region [matchStart, matchEnd). The context window
// expands equally before and after the match, clamped to content boundaries.
func extractContext(content string, matchStart, matchEnd, maxLen int) string {
	matchLen := matchEnd - matchStart

	if matchLen >= maxLen {
		// Match itself exceeds the context window — truncate the match.
		end := matchStart + maxLen
		if end > len(content) {
			end = len(content)
		}
		return content[matchStart:end]
	}

	// Distribute remaining budget equally before and after the match.
	remaining := maxLen - matchLen
	before := remaining / 2
	after := remaining - before

	start := matchStart - before
	if start < 0 {
		// Shift unused "before" budget to "after".
		after += -start
		start = 0
	}

	end := matchEnd + after
	if end > len(content) {
		// Shift unused "after" budget to "before".
		excess := end - len(content)
		end = len(content)
		start -= excess
		if start < 0 {
			start = 0
		}
	}

	return content[start:end]
}
