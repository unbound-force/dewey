package artifacts

import (
	"regexp"
	"sort"
	"strings"

	"github.com/unbound-force/dewey/v3/sanitize"
)

// credentialRule is one credential-detection rule applied by plan-time
// content screening. The rules live in this package (not in sanitize) because
// they are publish-gate specific: they decide whether an artifact may enter a
// chore PR, whereas sanitize.DefaultPatterns guard against prompt injection.
type credentialRule struct {
	regex    *regexp.Regexp
	name     string
	severity string
	message  string
}

// credentialPatterns are the credential detectors for plan-time screening,
// implemented with the standard library regexp engine. Per the spec, private
// key blocks and credential-bearing connection strings are critical; the key
// prefixes (sk-, ghp_, xoxb-, AKIA) are high.
var credentialPatterns = []credentialRule{
	{
		regex:    regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----`),
		name:     "private-key-block",
		severity: SeverityCritical,
		message:  "Private key material detected.",
	},
	{
		regex:    regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^\s/:@]+:[^\s/@]+@`),
		name:     "credential-connection-string",
		severity: SeverityCritical,
		message:  "Connection string with embedded credentials detected.",
	},
	{
		regex:    regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{16,}\b`),
		name:     "openai-key-prefix",
		severity: SeverityHigh,
		message:  "OpenAI-style secret key prefix detected.",
	},
	{
		regex:    regexp.MustCompile(`\bghp_[A-Za-z0-9]{36}\b`),
		name:     "github-pat-prefix",
		severity: SeverityHigh,
		message:  "GitHub personal access token prefix detected.",
	},
	{
		regex:    regexp.MustCompile(`\bxoxb-[A-Za-z0-9-]{10,}\b`),
		name:     "slack-bot-token",
		severity: SeverityHigh,
		message:  "Slack bot token prefix detected.",
	},
	{
		regex:    regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
		name:     "aws-access-key-id",
		severity: SeverityHigh,
		message:  "AWS access key ID detected.",
	},
}

// screenContent runs plan-time content screening over one artifact's content:
// the sanitize package's injection patterns (DefaultPatterns) plus this
// package's credential patterns. It returns the findings as the plan JSON
// Finding shape — the sanitize Finding's Context field (a matched-content
// excerpt) is deliberately dropped so blocked findings never re-deliver their
// payload into the plan JSON or the human gate.
//
// Findings are sorted by scanner, then pattern, then line for deterministic
// output regardless of rule evaluation order.
func screenContent(content string) []Finding {
	var findings []Finding
	for _, f := range sanitize.ScanInjectionPatterns(content, sanitize.DefaultPatterns) {
		findings = append(findings, Finding{
			Scanner:  ScannerInjection,
			Pattern:  f.Pattern,
			Severity: f.Severity,
			Line:     f.Line,
			Message:  f.Message,
		})
	}

	lines := strings.Split(content, "\n")
	for _, rule := range credentialPatterns {
		for i, line := range lines {
			if rule.regex.MatchString(line) {
				findings = append(findings, Finding{
					Scanner:  ScannerCredential,
					Pattern:  rule.name,
					Severity: rule.severity,
					Line:     i + 1,
					Message:  rule.message,
				})
			}
		}
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Scanner != findings[j].Scanner {
			return findings[i].Scanner < findings[j].Scanner
		}
		if findings[i].Pattern != findings[j].Pattern {
			return findings[i].Pattern < findings[j].Pattern
		}
		return findings[i].Line < findings[j].Line
	})
	return findings
}

// blocksPublish reports whether a screening finding blocks the artifact (or,
// inside a conflict, the variant) from entering the publish set: critical and
// high severities block; lower severities ride along as artifact findings.
func blocksPublish(f Finding) bool {
	return f.Severity == SeverityCritical || f.Severity == SeverityHigh
}
