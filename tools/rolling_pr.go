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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
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

// --- Durable run record (FR-021; contracts/scope.md Durable No-Op Record) ---

// distillRunsDir is the vault-relative directory holding one JSON run
// artifact per distill run — the audit surface that exists even when no
// PR does.
const distillRunsDir = ".uf/dewey/distill-runs"

// NoOpReasonEmptyScope is the FR-021 no-op reason for an empty in-scope
// set: the run wrote its artifact and touched nothing else.
const NoOpReasonEmptyScope = "empty scope"

// DistillRunRecord is the durable run artifact written to
// .uf/dewey/distill-runs/<YYYYMMDD-HHMMSS>.json (FR-021). On no-op paths
// — no PR, no comment — this artifact is the run's sole audit surface, so
// every distill run writes exactly one.
type DistillRunRecord struct {
	// RunAt is the run's start time (UTC).
	RunAt time.Time `json:"run_at"`
	// ScopeSize is len(in_scope) at scope computation time.
	ScopeSize int `json:"scope_size"`
	// Themes lists the theme names the run grouped, sorted.
	Themes []string `json:"themes"`
	// PRsTouched lists the rolling PR branches the run created or
	// updated, sorted.
	PRsTouched []string `json:"prs_touched"`
	// NoOpReason carries the FR-021 reason on no-op runs; empty on runs
	// that opened or updated PRs.
	NoOpReason string `json:"no_op_reason,omitempty"`
}

// WriteDistillRunArtifact writes record to the vault's distill-runs
// directory as <YYYYMMDD-HHMMSS>.json (record.RunAt, UTC) and returns the
// written path. Run artifacts are append-only audit history: a
// same-second collision appends a -2, -3, … suffix rather than
// overwriting the earlier record.
func WriteDistillRunArtifact(vaultDir string, record DistillRunRecord) (string, error) {
	dir := filepath.Join(vaultDir, distillRunsDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("distill run artifact: create dir: %w", err)
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return "", fmt.Errorf("distill run artifact: marshal: %w", err)
	}
	base := record.RunAt.UTC().Format("20060102-150405")
	for n := 1; ; n++ {
		name := base + ".json"
		if n > 1 {
			name = fmt.Sprintf("%s-%d.json", base, n)
		}
		path := filepath.Join(dir, name)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			if os.IsExist(err) {
				continue // same-second collision: next suffix
			}
			return "", fmt.Errorf("distill run artifact: create: %w", err)
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			_ = f.Close()
			return "", fmt.Errorf("distill run artifact: write: %w", err)
		}
		if err := f.Close(); err != nil {
			return "", fmt.Errorf("distill run artifact: close: %w", err)
		}
		return path, nil
	}
}

// RecordNoOpDistillRun writes the FR-021 no-op artifact for an empty
// scope and returns the artifact path. The pipeline's empty-scope exit:
// the caller computes scope, sees InScope == ∅, records this artifact,
// and exits without opening or updating any PR and without commenting on
// the distill issue. Refuses a non-empty scope fail-closed — recording a
// no-op while learnings wait in scope would hide them from every audit
// surface at once.
func RecordNoOpDistillRun(vaultDir string, scope ScopeSet) (string, error) {
	if !scope.Empty {
		return "", fmt.Errorf("distill run artifact: no-op record for non-empty scope (%d in scope)", len(scope.InScope))
	}
	record := DistillRunRecord{
		RunAt:      time.Now().UTC(),
		ScopeSize:  0,
		Themes:     []string{},
		PRsTouched: []string{},
		NoOpReason: NoOpReasonEmptyScope,
	}
	return WriteDistillRunArtifact(vaultDir, record)
}
