package sanitize

import (
	"fmt"
	"regexp"
	"strings"
)

// invisibleRunes defines the set of invisible Unicode characters that are
// scanned during structure validation. These characters are commonly used
// to hide payloads in Markdown content because they render as zero-width
// or invisible in most editors and renderers.
//
// Design decision: Using a map for O(1) lookup per rune rather than a
// sorted slice with binary search. The set is small (13 entries) so memory
// overhead is negligible, and the constant-time lookup simplifies the
// scanning loop. Chose explicit enumeration over range checks to avoid
// false positives on valid Unicode characters between the ranges.
var invisibleRunes = map[rune]string{
	'\u200B': "zero-width space",
	'\u200C': "zero-width non-joiner",
	'\u200D': "zero-width joiner",
	'\u200E': "left-to-right mark",
	'\u200F': "right-to-left mark",
	'\uFEFF': "byte order mark",
	'\u2060': "word joiner",
	'\u2061': "function application",
	'\u2062': "invisible times",
	'\u2063': "invisible separator",
	'\u2064': "invisible plus",
}

// Compiled regexes for structure validation checks. Package-level
// compilation avoids recompilation on every ValidateStructure call.
var (
	// dataURIImageRe matches Markdown image syntax with data: URIs.
	// Example: ![alt](data:text/html;base64,abc)
	dataURIImageRe = regexp.MustCompile(`(?i)!\[.*?\]\(data:`)

	// dataURILinkRe matches Markdown link syntax with data: URIs.
	// Example: [link](data:text/html;base64,abc)
	dataURILinkRe = regexp.MustCompile(`(?i)\[.*?\]\(data:`)

	// headingDepthRe matches heading markers with more than 6 # characters
	// at the start of a line. Valid Markdown headings use 1-6 # characters;
	// anything deeper is invalid and may indicate content manipulation.
	headingDepthRe = regexp.MustCompile(`(?m)^#{7,}\s`)

	// suspiciousTagRe matches HTML tags that could execute code or embed
	// external content: <script>, <iframe>, <object>, <embed>, <form>.
	// Case-insensitive to catch obfuscation attempts.
	suspiciousTagRe = regexp.MustCompile(`(?i)<(script|iframe|object|embed|form)[\s>]`)

	// eventHandlerRe matches HTML event handler attributes (onclick,
	// onerror, onload, etc.). These can execute JavaScript when the
	// Markdown is rendered as HTML.
	eventHandlerRe = regexp.MustCompile(`(?i)on\w+\s*=`)
)

// ValidateStructure inspects Markdown content for structural anomalies that
// may indicate hidden or injected payloads. It checks for:
//   - Invisible Unicode characters exceeding the threshold
//   - Embedded data URIs in Markdown image/link syntax
//   - Heading depth exceeding 6 levels
//   - Suspicious HTML tags and event handler attributes
//
// The invisibleThreshold parameter controls how many invisible Unicode
// characters are allowed before a finding is generated. A threshold of 0
// or negative disables the invisible character check.
//
// Returns an empty slice (not nil) when no findings are detected, ensuring
// callers can safely iterate without nil checks.
func ValidateStructure(content string, invisibleThreshold int) []Finding {
	var findings []Finding

	// Check 1: Invisible Unicode characters.
	findings = append(findings, detectInvisibleChars(content, invisibleThreshold)...)

	// Check 2: Data URI detection.
	findings = append(findings, detectDataURIs(content)...)

	// Check 3: Heading depth validation.
	findings = append(findings, detectHeadingDepth(content)...)

	// Check 4: Suspicious HTML detection.
	findings = append(findings, detectSuspiciousHTML(content)...)

	// Return empty slice instead of nil for consistent caller behavior.
	if findings == nil {
		findings = []Finding{}
	}

	return findings
}

// detectInvisibleChars scans content for invisible Unicode characters and
// returns a finding if the count exceeds the threshold. Reports the total
// count in the finding message.
func detectInvisibleChars(content string, threshold int) []Finding {
	if threshold <= 0 {
		return nil
	}

	count := 0
	for _, r := range content {
		if _, ok := invisibleRunes[r]; ok {
			count++
		}
	}

	if count > threshold {
		return []Finding{
			{
				Pattern:  "invisible-unicode",
				Severity: "medium",
				Category: "structure",
				Message:  fmt.Sprintf("found %d invisible Unicode characters (threshold: %d)", count, threshold),
			},
		}
	}

	return nil
}

// detectDataURIs scans for data: URIs in Markdown image and link syntax.
// Each match produces a separate finding with the line number where it
// was detected.
func detectDataURIs(content string) []Finding {
	var findings []Finding
	lines := strings.Split(content, "\n")

	for i, line := range lines {
		lineNum := i + 1 // 1-indexed

		if dataURIImageRe.MatchString(line) {
			findings = append(findings, Finding{
				Pattern:  "data-uri",
				Line:     lineNum,
				Severity: "high",
				Category: "structure",
				Message:  "data URI detected in Markdown image syntax",
				Context:  truncateContext(line),
			})
		} else if dataURILinkRe.MatchString(line) {
			// Only check link pattern if image pattern didn't match,
			// since ![...](data:...) also matches [...]( via the
			// link regex. This avoids duplicate findings on the same line.
			findings = append(findings, Finding{
				Pattern:  "data-uri",
				Line:     lineNum,
				Severity: "high",
				Category: "structure",
				Message:  "data URI detected in Markdown link syntax",
				Context:  truncateContext(line),
			})
		}
	}

	return findings
}

// detectHeadingDepth scans for Markdown headings with more than 6 # characters.
// Each match produces a finding with the line number.
func detectHeadingDepth(content string) []Finding {
	var findings []Finding
	lines := strings.Split(content, "\n")

	for i, line := range lines {
		lineNum := i + 1
		if headingDepthRe.MatchString(line) {
			findings = append(findings, Finding{
				Pattern:  "heading-depth",
				Line:     lineNum,
				Severity: "low",
				Category: "structure",
				Message:  "heading depth exceeds maximum of 6 levels",
				Context:  truncateContext(line),
			})
		}
	}

	return findings
}

// detectSuspiciousHTML scans for HTML tags and event handlers that could
// execute code when rendered. Each match produces a separate finding.
func detectSuspiciousHTML(content string) []Finding {
	var findings []Finding
	lines := strings.Split(content, "\n")

	for i, line := range lines {
		lineNum := i + 1

		if locs := suspiciousTagRe.FindAllStringIndex(line, -1); locs != nil {
			for range locs {
				findings = append(findings, Finding{
					Pattern:  "suspicious-html",
					Line:     lineNum,
					Severity: "high",
					Category: "structure",
					Message:  "suspicious HTML tag detected",
					Context:  truncateContext(line),
				})
			}
		}

		if locs := eventHandlerRe.FindAllStringIndex(line, -1); locs != nil {
			for range locs {
				findings = append(findings, Finding{
					Pattern:  "html-event-handler",
					Line:     lineNum,
					Severity: "high",
					Category: "structure",
					Message:  "HTML event handler attribute detected",
					Context:  truncateContext(line),
				})
			}
		}
	}

	return findings
}

// truncateContext returns up to 200 characters of the input string for
// inclusion in finding context fields. This matches the FR-SAN-007
// requirement for context length limits.
func truncateContext(s string) string {
	if len(s) <= 200 {
		return s
	}
	return s[:200]
}
