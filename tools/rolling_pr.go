// Package tools — rolling per-theme pull request management for the
// unattended weekly distill (specs/018-weekly-distill-automation, issue
// #193; contracts/rolling-pr.md).
//
// This file lands the branch-naming vocabulary (FR-011): theme names
// derive from attacker-influenced tags, embedding clusters, and an
// optional LLM judge, so the theme→branch mapping MUST define charset,
// length, and collision handling before any branch is created
// (contracts/rolling-pr.md Branch Sanitization). The PR create/update
// orchestrator grows from this foundation in later phases.
package tools

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// DistillBranchPrefix namespaces every rolling distill branch
// (data-model.md RollingPR.branch: dewey-distill/<theme>).
const DistillBranchPrefix = "dewey-distill/"

// Branch sanitization bounds (contracts/rolling-pr.md Branch
// Sanitization): the base truncates to 50 chars, the 8-hex hash suffix
// brings the sanitized segment to at most 59 chars, and the full
// dewey-distill/<sanitized> reference stays under 80.
const (
	// maxSanitizedBaseChars bounds the theme-derived base of a branch
	// name before the hash suffix.
	maxSanitizedBaseChars = 50
	// hashSuffixLen is the length of the sha256(theme)[:8] hex suffix.
	hashSuffixLen = 8
	// maxSanitizedSegmentChars bounds the sanitized segment: base + "-"
	// + hash. A collision suffix may extend it by 2.
	maxSanitizedSegmentChars = maxSanitizedBaseChars + 1 + hashSuffixLen
	// maxDistillBranchChars bounds the full branch reference including
	// the prefix.
	maxDistillBranchChars = 80
	// untaggedThemeFallback replaces a theme whose sanitization is empty.
	untaggedThemeFallback = "untagged"
)

// SanitizeThemeBranch maps a theme name to its sanitized branch segment —
// the <sanitized> of dewey-distill/<sanitized> (FR-011):
//
//   - lowercase, then replace every rune outside [a-z0-9] with "-"
//     (control characters, Unicode, and separators all collapse),
//   - collapse consecutive "-" and trim leading/trailing "-",
//   - fall back to "untagged" when nothing survives,
//   - truncate the base to 50 chars (trimming a trailing "-" the
//     truncation may leave),
//   - append "-" + the first 8 hex chars of sha256(theme), computed over
//     the ORIGINAL theme so near-identical sanitized names stay
//     distinct: "Auth/SSO..Flow" → "auth-sso-flow-<hash>", "/" →
//     "untagged-<hash>", "../../etc/passwd" → "etc-passwd-<hash>", a
//     250-char tag → "<50 chars>-<hash>".
//
// The result always matches [a-z0-9-]+ and never contains "/" or "..",
// so it cannot traverse out of the branch namespace.
func SanitizeThemeBranch(theme string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(theme) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	segment := collapseDashes(b.String())
	if segment == "" {
		segment = untaggedThemeFallback
	}
	if len(segment) > maxSanitizedBaseChars {
		segment = strings.TrimRight(segment[:maxSanitizedBaseChars], "-")
		if segment == "" {
			segment = untaggedThemeFallback
		}
	}
	sum := sha256.Sum256([]byte(theme))
	return segment + "-" + hex.EncodeToString(sum[:])[:hashSuffixLen]
}

// collapseDashes collapses runs of "-" to a single "-" and trims
// leading/trailing "-".
func collapseDashes(s string) string {
	var b strings.Builder
	prevDash := false
	for i := 0; i < len(s); i++ {
		if s[i] == '-' {
			if !prevDash {
				b.WriteByte('-')
			}
			prevDash = true
			continue
		}
		b.WriteByte(s[i])
		prevDash = false
	}
	return strings.Trim(b.String(), "-")
}

// AssignThemeBranches maps every theme name to a distinct
// dewey-distill/<sanitized> branch. Duplicate theme strings are one
// theme and share one branch. The 8-hex hash suffix makes collisions
// between distinct themes negligible, but two themes can still collide —
// a 4-byte hash collision — and on collision the second and later
// themes, in lexicographic order, gain a "-2", "-3", ... suffix
// (contracts/rolling-pr.md Branch Sanitization); the suffix may push the
// segment two chars past the nominal 59-char cap while the full branch
// stays under the 80-char bound.
func AssignThemeBranches(themes []string) map[string]string {
	return assignThemeBranches(themes, SanitizeThemeBranch)
}

// assignThemeBranches is AssignThemeBranches' testable core: segment
// computes each theme's sanitized branch segment, so tests can force the
// hash-collision path without mining a real sha256 collision.
func assignThemeBranches(themes []string, segment func(string) string) map[string]string {
	sorted := append([]string(nil), themes...)
	sort.Strings(sorted)
	assigned := make(map[string]string, len(sorted))
	taken := make(map[string]int, len(sorted)) // segment → collision count
	for _, theme := range sorted {
		if _, done := assigned[theme]; done {
			// Duplicate theme string: the same theme keeps its branch.
			continue
		}
		s := segment(theme)
		if n, collided := taken[s]; collided {
			n++
			taken[s] = n
			// The first taker is unsuffixed, so collisions number from
			// -2 (contracts/rolling-pr.md).
			s = fmt.Sprintf("%s-%d", s, n+1)
		} else {
			taken[s] = 0
		}
		assigned[theme] = DistillBranchPrefix + s
	}
	return assigned
}

// ValidateThemeBranch checks a full dewey-distill/<sanitized> branch
// reference against the FR-011 grammar: the prefix must be present, the
// sanitized segment must be 1..59 chars of [a-z0-9-] (a trailing
// collision suffix may extend it to 61), the full reference must stay
// under 80 chars, and any "/" or ".." in the segment — a traversal
// attempt — is rejected outright.
func ValidateThemeBranch(branch string) error {
	segment, ok := strings.CutPrefix(branch, DistillBranchPrefix)
	if !ok {
		return fmt.Errorf("branch %q must start with %q", branch, DistillBranchPrefix)
	}
	if segment == "" {
		return fmt.Errorf("branch %q has an empty theme segment", branch)
	}
	if len(branch) > maxDistillBranchChars {
		return fmt.Errorf("branch %q exceeds %d chars", branch, maxDistillBranchChars)
	}
	limit := maxSanitizedSegmentChars + 2 // collision suffix allowance
	if len(segment) > limit {
		return fmt.Errorf("branch %q theme segment exceeds %d chars", branch, limit)
	}
	for i := 0; i < len(segment); i++ {
		c := segment[i]
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return fmt.Errorf("branch %q segment contains %q outside [a-z0-9-]", branch, string(c))
		}
	}
	if strings.Contains(segment, "..") {
		return fmt.Errorf("branch %q segment contains a traversal", branch)
	}
	return nil
}
