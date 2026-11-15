// Package tools — scope computation foundation for the unattended weekly
// distill (specs/018-weekly-distill-automation, issue #193).
//
// This file carries the feature's data model (data-model.md) and the two
// enumeration legs every distill run composes (research.md D-002): the
// primary git leg — one batch `git ls-tree -r -z` over the learnings and
// compiled prefixes at the resolved default-branch ref, reusing the
// gitref package's battle-tested invocation contract — and the
// filesystem fallback leg for non-git vaults, which scans the same tree
// directly and reports a degraded provenance. Scope is deterministic
// from git alone (FR-005): no running server is required, and the same
// ref plus policy always yields the same set.
package tools

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"gopkg.in/yaml.v3"

	"github.com/unbound-force/dewey/v3/gitref"
	"github.com/unbound-force/dewey/v3/store"
)

// scopeLogger is the package-level structured logger for distill scope
// computation (per-file logger convention; the only package-level variable
// pattern this repo allows).
var scopeLogger = log.NewWithOptions(os.Stderr, log.Options{
	Prefix:          "dewey/scope",
	ReportTimestamp: true,
	TimeFormat:      "2006-01-02T15:04:05.000Z07:00",
})

// Distill pathspecs (vault-relative, contracts/scope.md Input). Both legs
// enumerate these exact prefixes so git and filesystem scopes agree.
const (
	// distillLearningsPrefix is the vault-relative path of the file-backed
	// learnings tree.
	distillLearningsPrefix = ".uf/dewey/learnings/"
	// distillCompiledPrefix is the vault-relative path of the compiled
	// articles tree.
	distillCompiledPrefix = ".uf/dewey/compiled/"
)

// ScopeProvenanceDegraded marks a scope computed from the filesystem
// fallback rather than the git object DB (contracts/scope.md Fallback):
// no blob-equality guarantees, run proceeds with a warning.
const ScopeProvenanceDegraded = "filesystem:degraded"

// --- Data model (data-model.md Entities) ---

// ScopeSet is the deterministic result of one scope computation: every
// learning identity the ref holds, the subset already cited by compiled
// articles' frontmatter sources, and the in-scope remainder after policy
// exclusions (data-model.md ScopeSet). Sets are sorted identity slices so
// the JSON result is byte-stable for a given ref and policy (FR-005).
type ScopeSet struct {
	// AllLearnings lists every learning identity at the enumerated source,
	// sorted.
	AllLearnings []string `json:"all_learnings"`
	// Cited lists every learning identity referenced by at least one
	// compiled article's frontmatter sources, sorted.
	Cited []string `json:"cited"`
	// InScope lists the uncited, non-excluded learning identities — the
	// distill run's working set — sorted.
	InScope []string `json:"in_scope"`
	// Empty reports InScope == ∅, the FR-021 no-op signal: no PR, no
	// comment, one durable run artifact.
	Empty bool `json:"empty"`
	// Provenance records where the set came from: "git:<ref>:<sha>" for
	// the primary leg, ScopeProvenanceDegraded for the filesystem
	// fallback.
	Provenance string `json:"provenance"`
}

// NewScopeSet builds a ScopeSet from unsorted identity sets, deriving
// Empty and normalizing every set to sorted order so equal inputs always
// produce equal ScopeSets (FR-005 determinism).
func NewScopeSet(all, cited, inScope map[string]struct{}, provenance string) ScopeSet {
	s := ScopeSet{
		AllLearnings: sortedIdentities(all),
		Cited:        sortedIdentities(cited),
		InScope:      sortedIdentities(inScope),
		Empty:        len(inScope) == 0,
		Provenance:   provenance,
	}
	return s
}

// sortedIdentities converts a set into its sorted slice form.
func sortedIdentities(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// DistillPolicy is the optional exclusion policy from
// .uf/dewey/distill-policy.yaml (data-model.md DistillPolicy; FR-007).
// A missing file means no exclusions. Tag globs match the learning's tag
// namespace; store names scope the curation leg, not the learning set.
type DistillPolicy struct {
	// ExcludeTags lists tag globs (path.Match subset: *, ?, [seq]; no
	// recursive **) whose matching learnings leave the distill scope.
	ExcludeTags []string `json:"exclude_tags,omitempty" yaml:"exclude_tags"`
	// ExcludeStores lists knowledge store names excluded from distill
	// curation.
	ExcludeStores []string `json:"exclude_stores,omitempty" yaml:"exclude_stores"`
}

// ExcludesIdentity reports whether any exclude_tags glob matches the
// learning identity's tag namespace (contracts/scope.md Behavior:
// filtered = raw - policy.exclude_tags.matches(raw)). path.Match is
// case-sensitive and its * does not cross "/", matching the normalized
// [a-z0-9-] tag grammar exactly.
func (p DistillPolicy) ExcludesIdentity(identity string) bool {
	tag := IdentityTag(identity)
	for _, glob := range p.ExcludeTags {
		if matched, err := path.Match(glob, tag); err == nil && matched {
			return true
		}
	}
	return false
}

// IdentityTag extracts the tag namespace from a learning identity without
// touching page properties: the current "{tag}-{YYYYMMDDTHHMMSS}-{author}"
// form (including collision-suffixed variants like
// "{tag}-{ts}-{author}-2") yields the segments before the timestamp, and
// the legacy "{tag}-{sequence}" form yields the segments before the
// numeric tail. Anything else is treated as a bare tag — mirroring
// extractTagFromIdentity's no-properties fallback so both parses agree on
// tool-created identities. Pure string shape only: the same identity
// always yields the same tag, which is what scope determinism (FR-005)
// requires when properties are not enumerable from git paths alone.
func IdentityTag(identity string) string {
	segs := strings.Split(identity, "-")
	for i, seg := range segs {
		if i > 0 && isTimestampSegment(seg) {
			return strings.Join(segs[:i], "-")
		}
	}
	if last := len(segs) - 1; last >= 1 && isDigitRun(segs[last]) {
		return strings.Join(segs[:last], "-")
	}
	return identity
}

// IdentityTimestamp extracts the creation timestamp embedded in a
// current-format learning identity "{tag}-{YYYYMMDDTHHMMSS}-{author}".
// It walks the same segments IdentityTag walks, so the two parses agree
// on which identity shapes carry a timestamp; a segment that matches the
// YYYYMMDDTHHMMSS shape but is not a real calendar time (e.g. month 13)
// reports no timestamp rather than an error — the filter treats it like
// a legacy identity. Pure string shape: no page properties are read.
func IdentityTimestamp(identity string) (time.Time, bool) {
	segs := strings.Split(identity, "-")
	for i, seg := range segs {
		if i > 0 && isTimestampSegment(seg) {
			ts, err := time.Parse("20060102T150405", seg)
			if err != nil {
				return time.Time{}, false
			}
			return ts, true
		}
	}
	return time.Time{}, false
}

// FilterScopeByBacklogDate narrows a ScopeSet's InScope to identities
// created on or before the cutoff date (the /dewey-distill-auto
// --backlog-from flag, FR-020 backlog bootstrap). Identities embed UTC
// timestamps; a learning is in the backlog window when its timestamp is
// strictly before the end of from's day. Legacy identities without an
// embedded timestamp always pass — age data does not exist for them, and
// the bootstrap must not silently drop pre-migration learnings. All,
// Cited, and Provenance pass through untouched: they describe the ref,
// not the window.
func FilterScopeByBacklogDate(scope ScopeSet, from time.Time) ScopeSet {
	limit := from.Add(24 * time.Hour) // exclusive end of the cutoff date
	filtered := make([]string, 0, len(scope.InScope))
	for _, id := range scope.InScope {
		if ts, ok := IdentityTimestamp(id); ok && !ts.Before(limit) {
			continue // created after the cutoff date — outside this run's window
		}
		filtered = append(filtered, id)
	}
	out := ScopeSet{
		AllLearnings: scope.AllLearnings,
		Cited:        scope.Cited,
		InScope:      filtered,
		Empty:        len(filtered) == 0,
		Provenance:   scope.Provenance,
	}
	return out
}

// SupersessionLink is one explicit supersede pair that must co-locate in
// a single theme (FR-009; data-model.md Theme.supersessions).
type SupersessionLink struct {
	// Successor is the learning identity that supersedes Target.
	Successor string `json:"successor"`
	// Target is the superseded learning identity.
	Target string `json:"target"`
}

// AssignmentDecision records the provenance of one learning→theme
// assignment (FR-010; data-model.md AssignmentDecision). Every decision
// names its source, carries a 0..1 confidence, and details the evidence —
// embedding distance, rule name, or judge identity — so a human reviewer
// can audit the grouping without re-running it.
type AssignmentDecision struct {
	// Identity is the assigned learning identity.
	Identity string `json:"identity"`
	// TargetTheme is the theme name the learning joined.
	TargetTheme string `json:"target_theme"`
	// Source names the decision mechanism: one of the Decision*
	// constants.
	Source string `json:"source"`
	// Confidence is the decision's confidence in [0,1].
	Confidence float64 `json:"confidence"`
	// Details carries the evidence: distance value, rule name, or judge
	// identity.
	Details string `json:"details,omitempty"`
}

// Assignment decision sources (data-model.md AssignmentDecision.source).
const (
	// DecisionEmbeddingDistance marks an embedding-cosine assignment
	// against an existing compiled article.
	DecisionEmbeddingDistance = "embedding_distance"
	// DecisionTagFallback marks a tag-equality assignment used when
	// embeddings are unavailable (degraded grouping mode).
	DecisionTagFallback = "tag_fallback"
	// DecisionSupersessionCoercion marks a must-colocate assignment: the
	// learning rides its supersession edge (FR-009).
	DecisionSupersessionCoercion = "supersession_coercion"
	// DecisionTieBreaker marks a deterministic closest-centroid
	// resolution of a narrow-margin ambiguity (FR-010).
	DecisionTieBreaker = "tie_breaker"
	// DecisionLLMJudge marks a configured LLM-judge assignment (behind a
	// flag; every decision logged).
	DecisionLLMJudge = "llm_judge"
)

// Theme is one grouped cluster of in-scope learnings, 1:1 with a rolling
// PR (data-model.md Theme). Members and decisions are per-theme; the
// supersession links record the edges that forced co-location.
type Theme struct {
	// Name is the sanitized tag/topic naming the theme and its branch.
	Name string `json:"name"`
	// ExistingArticle is the vault-relative path of the compiled article
	// this theme updates, when one exists.
	ExistingArticle string `json:"existing_article,omitempty"`
	// Members lists the assigned learning identities, sorted.
	Members []string `json:"members"`
	// Supersessions lists the explicit supersedes pairs whose members
	// must co-locate here.
	Supersessions []SupersessionLink `json:"supersessions,omitempty"`
	// Decisions records the per-member assignment provenance (FR-010).
	Decisions []AssignmentDecision `json:"decisions"`
}

// Rolling PR states (data-model.md RollingPR.state).
const (
	// PRStateOpen is a rolling PR currently awaiting human review.
	PRStateOpen = "open"
	// PRStateClosed is a rolling PR closed without merging; the next run
	// for its theme creates a fresh PR, never a reopen.
	PRStateClosed = "closed"
	// PRStateMerged is a merged rolling PR — the human vouch (#203).
	PRStateMerged = "merged"
)

// Rolling PR labels (contracts/rolling-pr.md, FR-014).
const (
	// LabelDeweyDistill marks the weekly distill issue and rolling PRs.
	LabelDeweyDistill = "dewey-distill"
	// LabelHumanGate hands the PR off from tooling: agents and bots must
	// not approve, merge, or force it through.
	LabelHumanGate = "human-gate"
	// LabelNeedsHumanResolution marks a PR whose rebase or regeneration
	// conflict requires a human decision (FR-015).
	LabelNeedsHumanResolution = "needs-human-resolution"
)

// RollingPR is the per-theme pull request state (data-model.md
// RollingPR). Transitions: no open PR → create; open → rebase/update;
// closed → create fresh; empty scope → no-op. The distiller only ever
// writes tier draft articles (TierWritten; FR-012).
type RollingPR struct {
	// Branch is the dewey-distill/<sanitized> branch name.
	Branch string `json:"branch"`
	// PRNumber is the GitHub pull request number; nil before creation.
	PRNumber *int `json:"pr_number,omitempty"`
	// State is one of the PRState* constants.
	State string `json:"state"`
	// Labels always includes LabelHumanGate and may carry
	// LabelNeedsHumanResolution.
	Labels []string `json:"labels"`
	// HeadSHA is the branch tip after this run's push.
	HeadSHA string `json:"head_sha"`
	// HumanCommitsPreserved reports whether the rebase retained every
	// human (non-distiller) commit (FR-015).
	HumanCommitsPreserved bool `json:"human_commits_preserved"`
	// Body is the rendered ReviewQueue (per-article review surface).
	Body string `json:"body"`
	// TierWritten is the article tier this PR writes: always draft in
	// distiller mode (FR-012; server guard rejects validated writes).
	TierWritten string `json:"tier_written"`
}

// ReviewQueueArticle is one per-article section of a rolling PR's review
// queue body (data-model.md ReviewQueue; contracts/rolling-pr.md Review
// Queue Body). The renderer (rolling_pr.go) escapes every interpolated
// field per the sanitization gate and caps the assembled body.
type ReviewQueueArticle struct {
	// Title is the article's display title.
	Title string `json:"title"`
	// Path is the vault-relative article path.
	Path string `json:"path"`
	// WhatChanged is the one-line theme delta.
	WhatChanged string `json:"what_changed"`
	// NewlyCited lists the learning identities this article newly cites.
	NewlyCited []string `json:"newly_cited"`
	// Supersessions lists rendered "<successor> supersedes <target>"
	// pairs.
	Supersessions []string `json:"supersessions"`
	// Lint lists the lint finding kinds affecting this article
	// (anchor_drift, embedding_gap).
	Lint []string `json:"lint"`
	// ConfidenceAvg is the mean assignment confidence for the theme.
	ConfidenceAvg float64 `json:"confidence_avg"`
	// Decisions carries the per-decision distances behind
	// ConfidenceAvg.
	Decisions []AssignmentDecision `json:"decisions,omitempty"`
	// TotalCited is the article's total citation count after this
	// run's render (members kept by the sanitization gate).
	TotalCited int `json:"total_cited,omitempty"`
	// Dropped lists member identities the sanitization gate removed
	// from the article (FR-013: critical/high findings, missing text);
	// the PR body notes each drop.
	Dropped []string `json:"dropped,omitempty"`
	// SkippedFiles lists vault-relative paths the regeneration guard
	// protected this run (FR-015: human edits never overwritten).
	SkippedFiles []string `json:"skipped_files,omitempty"`
	// ConflictFiles lists paths that conflicted during the rebase
	// (FR-015); the PR is labeled needs-human-resolution and the
	// remote branch is left untouched.
	ConflictFiles []string `json:"conflict_files,omitempty"`
}

// --- Policy loading (FR-007; contracts/scope.md Policy Exclusions) ---

// Policy file bounds (contracts/scope.md): over any limit the whole file
// is ignored — no exclusions — with a warning, and the run proceeds.
const (
	// maxPolicyFileBytes bounds the policy file (16 KiB).
	maxPolicyFileBytes = 16 * 1024
	// maxPolicyDepth bounds YAML nesting depth.
	maxPolicyDepth = 3
	// maxPolicyArrayEntries bounds each policy array.
	maxPolicyArrayEntries = 100
	// maxPolicyEntryChars bounds each policy entry.
	maxPolicyEntryChars = 200
	// maxPolicyStoreNameChars bounds a store name ([a-z0-9-]{1,50}).
	maxPolicyStoreNameChars = 50
)

// LoadDistillPolicy reads the optional distill policy at path. It never
// fails the run: a missing file yields an empty policy with no warnings
// (the default — all uncited learnings in scope), and every other problem
// — unreadable file, unparseable YAML, over-limit shape, invalid or
// overly broad entries, unknown keys — yields the valid subset plus
// human-readable warnings the caller surfaces in the run report
// (FR-007's warn-and-skip contract; malformed policy is never fatal).
//
// The bounds are enforced before any entry is used: a file over 16 KiB,
// deeper than 3 levels, or with an array over 100 entries is ignored in
// full ("over limit, ignored"), because a partially trusted policy could
// starve or flood scope in ways a human did not review.
func LoadDistillPolicy(policyPath string) (DistillPolicy, []string) {
	var warnings []string
	data, err := os.ReadFile(policyPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Missing policy is the documented default: no exclusions.
			return DistillPolicy{}, nil
		}
		warn := fmt.Sprintf("distill-policy: unreadable (%v), no exclusions applied", err)
		scopeLogger.Warn(warn, "path", policyPath)
		return DistillPolicy{}, []string{warn}
	}
	if len(data) > maxPolicyFileBytes {
		warn := fmt.Sprintf("distill-policy: over limit (%d bytes > %d), ignored", len(data), maxPolicyFileBytes)
		scopeLogger.Warn(warn, "path", policyPath)
		return DistillPolicy{}, []string{warn}
	}

	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		warn := fmt.Sprintf("distill-policy: unparseable (%v), no exclusions applied", err)
		scopeLogger.Warn(warn, "path", policyPath)
		return DistillPolicy{}, []string{warn}
	}
	if d := yamlDepth(raw); d > maxPolicyDepth {
		warn := fmt.Sprintf("distill-policy: over limit (depth %d > %d), ignored", d, maxPolicyDepth)
		scopeLogger.Warn(warn, "path", policyPath)
		return DistillPolicy{}, []string{warn}
	}

	policy := DistillPolicy{}
	for key, value := range raw {
		switch key {
		case "exclude_tags":
			entries, warns := appendPolicyEntries(value, validTagExclusion)
			policy.ExcludeTags = append(policy.ExcludeTags, entries...)
			warnings = append(warnings, warns...)
		case "exclude_stores":
			entries, warns := appendPolicyEntries(value, validStoreExclusion)
			policy.ExcludeStores = append(policy.ExcludeStores, entries...)
			warnings = append(warnings, warns...)
		default:
			warn := fmt.Sprintf("distill-policy: unknown key %q ignored", key)
			scopeLogger.Warn(warn, "path", policyPath)
			warnings = append(warnings, warn)
		}
	}
	return policy, warnings
}

// appendPolicyEntries validates one policy array's entries and returns
// the valid ones plus one warning per skipped entry (warn + skip, never
// fatal). An array over the entry-count bound is ignored in full — the
// over-limit rule — rather than truncated, so a human cannot mistake a
// truncated policy for the reviewed one.
func appendPolicyEntries(value any, valid func(string) (bool, string)) (entries, warnings []string) {
	list, ok := value.([]any)
	if !ok {
		warn := "distill-policy: value is not a list, ignored"
		scopeLogger.Warn(warn)
		return nil, []string{warn}
	}
	if len(list) > maxPolicyArrayEntries {
		warn := fmt.Sprintf("distill-policy: over limit (%d entries > %d), ignored", len(list), maxPolicyArrayEntries)
		scopeLogger.Warn(warn)
		return nil, []string{warn}
	}
	for _, item := range list {
		entry, ok := item.(string)
		if !ok {
			warn := "distill-policy: non-string entry skipped"
			scopeLogger.Warn(warn)
			warnings = append(warnings, warn)
			continue
		}
		if len(entry) > maxPolicyEntryChars {
			warn := fmt.Sprintf("distill-policy: entry over %d chars skipped", maxPolicyEntryChars)
			scopeLogger.Warn(warn)
			warnings = append(warnings, warn)
			continue
		}
		if ok, reason := valid(entry); !ok {
			warn := fmt.Sprintf("distill-policy: %s, entry skipped", reason)
			scopeLogger.Warn(warn, "entry", entry)
			warnings = append(warnings, warn)
			continue
		}
		entries = append(entries, entry)
	}
	return entries, warnings
}

// validTagExclusion validates one exclude_tags entry against the tag-glob
// grammar (contracts/scope.md schema): a path.Match-compatible pattern
// over the [a-z0-9-] tag namespace — no recursive ** and no "/" (tags
// carry neither) — that contains at least one non-wildcard character so a
// purely wildcard entry ("*", "*/*", "**") can never starve scope to
// empty (the overly-broad denial).
func validTagExclusion(entry string) (bool, string) {
	if entry == "" {
		return false, "empty entry"
	}
	if strings.Contains(entry, "**") {
		return false, "recursive glob not allowed"
	}
	if strings.Contains(entry, "/") {
		return false, "tag globs cannot contain '/'"
	}
	// path.Match returns ErrBadPattern for malformed globs; matching
	// against "" exercises the pattern without needing a real tag.
	if _, err := path.Match(entry, ""); err != nil {
		return false, "malformed glob"
	}
	if strings.Trim(entry, "*?[]") == "" {
		return false, "overly broad exclusion ignored"
	}
	return true, ""
}

// validStoreExclusion validates one exclude_stores entry against the
// knowledge-store name grammar [a-z0-9-]{1,50}. Membership in
// knowledge-stores.yaml is checked at the consumption site (curation),
// not here — the loader stays pure and testable without the store config.
func validStoreExclusion(entry string) (bool, string) {
	if entry == "" || len(entry) > maxPolicyStoreNameChars {
		return false, "store name must be 1-50 chars"
	}
	for i := 0; i < len(entry); i++ {
		c := entry[i]
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return false, "store name allows only [a-z0-9-]"
		}
	}
	return true, ""
}

// yamlDepth computes the nesting depth of a decoded YAML document:
// scalars are 0, and each mapping or sequence level adds one. The valid
// policy (map → list → scalar) is depth 2, so the bound of 3 tolerates
// one extra level and rejects anything deeper.
func yamlDepth(node any) int {
	switch typed := node.(type) {
	case map[string]any:
		max := 0
		for _, v := range typed {
			if d := yamlDepth(v); d > max {
				max = d
			}
		}
		return 1 + max
	case []any:
		max := 0
		for _, v := range typed {
			if d := yamlDepth(v); d > max {
				max = d
			}
		}
		return 1 + max
	default:
		return 0
	}
}

// --- Enumeration legs (research.md D-002; T010 shared helpers) ---

// gitDistillTree is the git leg's enumeration: the blob map for both
// distill prefixes at the resolved default-branch ref, from one batch
// `git ls-tree -r -z` invocation. Paths are vault-relative because the
// Runner executes git with the vault directory as cwd and ls-tree reports
// tree entries relative to it — the same property the gitref trust flow
// relies on.
type gitDistillTree struct {
	// blobs maps vault-relative path → blob object ID for every file
	// under the learnings and compiled prefixes.
	blobs map[string]string
}

// learningIdentities derives the learning identities from the enumerated
// learnings blob paths (filename minus .md, path stripped).
func (t *gitDistillTree) learningIdentities() []string {
	var ids []string
	for p := range t.blobs {
		if !strings.HasPrefix(p, distillLearningsPrefix) || !strings.HasSuffix(p, ".md") {
			continue
		}
		ids = append(ids, learningIdentityFromPath(p))
	}
	sort.Strings(ids)
	return ids
}

// compiledPaths lists the enumerated compiled-article paths, sorted —
// deterministic iteration order for the citation pass.
func (t *gitDistillTree) compiledPaths() []string {
	var paths []string
	for p := range t.blobs {
		if strings.HasPrefix(p, distillCompiledPrefix) && strings.HasSuffix(p, ".md") {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	return paths
}

// enumerateDistillTreeGit runs the git leg: one batch ls-tree over both
// distill pathspecs at the resolved ref through the shared gitref Runner
// seam (argv arrays, bounded timeouts, flag-injection guards). The
// boolean reports a healthy enumeration; false means git could not answer
// (non-git vault, unresolved ref, failed invocation) and the caller takes
// the filesystem fallback — a degraded mode, not an error.
func enumerateDistillTreeGit(ctx context.Context, run gitref.Runner, vaultDir string, res gitref.RefResolution) (*gitDistillTree, bool) {
	// An unresolved ref is a fallback signal, not an empty healthy scope:
	// reporting success here would exit the run no-op with scope_size 0
	// and mask the missing ref behind FR-021's empty-scope semantics.
	if !res.Resolved() || res.FullRef == "" {
		return nil, false
	}
	if run == nil {
		run = gitref.ExecRunner{}
	}
	mapped := gitref.MapDefaultBranchBlobsResult(ctx, run, vaultDir, res, []string{
		distillLearningsPrefix, distillCompiledPrefix,
	})
	if mapped.BatchFailed {
		return nil, false
	}
	return &gitDistillTree{blobs: mapped.Blobs}, true
}

// blobTimeout bounds each blob read invocation (lint_anchors.go
// precedent).
const blobTimeout = 5 * time.Second

// blobContent reads one blob's content via `git cat-file blob <sha>` —
// the blob read for the citation pass. The sha comes from the ls-tree
// enumeration and is hex-40-validated by the gitref parser, so it can
// carry no flag or rev syntax; the invocation is one bounded subprocess
// through the shared Runner. The boolean reports success; a failed read
// yields no citation data and the caller records a warning rather than
// failing the run.
func blobContent(ctx context.Context, run gitref.Runner, vaultDir, sha string) (string, bool) {
	if run == nil || sha == "" {
		return "", false
	}
	ctx, cancel := context.WithTimeout(ctx, blobTimeout)
	defer cancel()
	out, _, code, err := run.Run(ctx, vaultDir, []string{"cat-file", "blob", sha})
	if err != nil || code != 0 {
		return "", false
	}
	return out, true
}

// fsDistillTree is the filesystem leg's enumeration for non-git vaults:
// learning identities and compiled citations scanned directly from the
// .uf/dewey tree, plus the warnings a degraded run surfaces
// (contracts/scope.md Fallback: provenance filesystem:degraded, no
// panic).
type fsDistillTree struct {
	// LearningIdentities lists every learning file's identity, sorted.
	LearningIdentities []string
	// CompiledCitations maps compiled path → its frontmatter sources.
	CompiledCitations map[string][]string
	// Warnings collects per-file problems: unreadable files and
	// unparseable frontmatter degrade to no citations, visibly.
	Warnings []string
}

// scanDistillTreeFS runs the filesystem leg: scan the learnings and
// compiled directories directly. Learnings contribute identities from
// filenames exactly as the git leg derives them; compiled files are read
// and their frontmatter sources parsed, so both legs produce the same
// citation union. A missing directory is an empty set, not an error — an
// empty vault scopes to empty and exits no-op (FR-021).
func scanDistillTreeFS(vaultDir string) *fsDistillTree {
	tree := &fsDistillTree{
		CompiledCitations: make(map[string][]string),
	}
	for _, name := range listMarkdownFiles(filepath.Join(vaultDir, distillLearningsPrefix)) {
		tree.LearningIdentities = append(tree.LearningIdentities, learningIdentityFromPath(distillLearningsPrefix+name))
	}
	sort.Strings(tree.LearningIdentities)
	for _, name := range listMarkdownFiles(filepath.Join(vaultDir, distillCompiledPrefix)) {
		rel := distillCompiledPrefix + name
		data, err := os.ReadFile(filepath.Join(vaultDir, rel))
		if err != nil {
			warn := fmt.Sprintf("distill: compiled article unreadable in degraded mode: %v", err)
			tree.Warnings = append(tree.Warnings, warn)
			continue
		}
		sources, ok := parseCompiledSources(string(data))
		if !ok {
			// Over-scope safely: an unparseable article contributes no
			// citations, so its learnings stay in scope for a human to
			// see, and the warning records why.
			warn := fmt.Sprintf("distill: compiled article frontmatter unparseable in degraded mode, treated as uncited: %s", rel)
			tree.Warnings = append(tree.Warnings, warn)
			continue
		}
		tree.CompiledCitations[rel] = sources
	}
	return tree
}

// learningIdentityFromPath derives the learning identity from a
// vault-relative learnings path: strip the prefix, strip the .md
// extension.
func learningIdentityFromPath(p string) string {
	_, base := path.Split(p)
	return strings.TrimSuffix(base, ".md")
}

// listMarkdownFiles lists the .md files directly inside dir (the
// learnings and compiled trees are flat), tolerating a missing directory.
func listMarkdownFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		names = append(names, e.Name())
	}
	return names
}

// parseCompiledSources extracts the learning identities cited by a
// compiled article's YAML frontmatter sources block — the union that
// defines the cited set (contracts/scope.md Behavior). The boolean
// reports parseability: missing frontmatter, unparseable YAML, or a
// non-list sources value yields false and the caller treats the article
// as uncited with a warning (over-scope, human-visible). The block format
// is the one buildCompiledArticle and buildStoredArticle write:
//
//	---
//	tier: draft
//	sources:
//	  - auth-1
//	  - auth-3
//	---
func parseCompiledSources(content string) ([]string, bool) {
	block, ok := frontmatterBlock(content)
	if !ok {
		return nil, false
	}
	var props map[string]any
	if err := yaml.Unmarshal([]byte(block), &props); err != nil {
		return nil, false
	}
	rawSources, ok := props["sources"].([]any)
	if !ok {
		if _, present := props["sources"]; present {
			// A non-list sources value is malformed frontmatter.
			return nil, false
		}
		// No sources key: a compiled article citing nothing is valid and
		// cites nothing.
		return nil, true
	}
	ids := make([]string, 0, len(rawSources))
	for _, s := range rawSources {
		if id, ok := s.(string); ok && id != "" {
			ids = append(ids, id)
		}
	}
	return ids, true
}

// frontmatterBlock extracts the YAML text between the leading --- fences
// of a markdown document, mirroring vault.parseFrontmatter's boundary
// rules (split on the first line-starting --- after the opening fence,
// CRLF-tolerant; the tools-side copy keeps the scope computation
// self-contained and unexported).
func frontmatterBlock(content string) (string, bool) {
	if !strings.HasPrefix(content, "---") {
		return "", false
	}
	parts := strings.SplitN(content[3:], "\n---", 2)
	if len(parts) < 2 {
		return "", false
	}
	block := strings.TrimPrefix(parts[0], "\n")
	block = strings.TrimPrefix(block, "\r\n")
	return block, true
}

// --- Scope computation (T016/T017; contracts/scope.md Behavior) ---

// LearningPageNames yields the learning page identities held by the
// persistent store — the SQLite half of the filesystem fallback's union
// (contracts/scope.md Fallback). It is a function seam so ComputeScope
// stays testable without a database; production callers pass
// StoreLearningPageNames.
type LearningPageNames func() ([]string, error)

// StoreLearningPageNames adapts a *store.Store to the LearningPageNames
// seam. ListDraftCandidatePages already enumerates the learning/ namespace
// index-backed and NOCASE-ranged (learnings clamp to draft on every write
// route, so the tier filter matches the namespace's steady state); the
// adapter keeps only learning/ names and strips the prefix to identities.
// A nil store yields a nil seam — no store, no union, FS scan only.
func StoreLearningPageNames(s *store.Store) LearningPageNames {
	if s == nil {
		return nil
	}
	return func() ([]string, error) {
		pages, err := s.ListDraftCandidatePages()
		if err != nil {
			return nil, fmt.Errorf("list learning pages: %w", err)
		}
		const prefixLen = len(store.LearningPrefix)
		ids := make([]string, 0, len(pages))
		for _, p := range pages {
			if len(p.Name) > prefixLen && strings.EqualFold(p.Name[:prefixLen], store.LearningPrefix) {
				ids = append(ids, p.Name[prefixLen:])
			}
		}
		return ids, nil
	}
}

// ComputeScope computes one distill run's scope over vaultDir
// (contracts/scope.md Behavior): every learning identity at the resolved
// default-branch ref, minus the identities cited by compiled articles'
// frontmatter sources, minus policy tag exclusions. Deterministic from git
// alone (FR-005) and never interactive (FR-006): ambiguity — an unreadable
// blob, unparseable frontmatter, an invalid policy entry — degrades to
// warn-and-skip, never a question, so the same ref and policy always yield
// the same set.
//
// The git leg needs a resolved ref and one healthy batch ls-tree; when
// either is missing (non-git vault, unresolved ref, failed invocation) the
// computation falls back to scanning .uf/dewey directly, unioned with the
// SQLite learning/ names when learningPages is non-nil, and reports
// ScopeProvenanceDegraded (contracts/scope.md Fallback: degraded mode, no
// panic). A vaultDir that does not exist is the one hard error — a
// misconfigured vault must not masquerade as an empty no-op scope.
func ComputeScope(ctx context.Context, vaultDir string, learningPages LearningPageNames, policy DistillPolicy) (ScopeSet, error) {
	if vaultDir == "" {
		return ScopeSet{}, fmt.Errorf("distill scope: vault dir is empty")
	}
	info, err := os.Stat(vaultDir)
	if err != nil {
		return ScopeSet{}, fmt.Errorf("distill scope: stat vault dir: %w", err)
	}
	if !info.IsDir() {
		return ScopeSet{}, fmt.Errorf("distill scope: vault dir %s is not a directory", vaultDir)
	}

	res := gitref.New(gitref.Options{VaultDir: vaultDir}).Resolve(ctx)
	if res.Resolved() {
		if tree, ok := enumerateDistillTreeGit(ctx, nil, vaultDir, res); ok {
			return scopeFromGitTree(ctx, vaultDir, tree, res, policy), nil
		}
		scopeLogger.Warn("distill scope: git enumeration failed at resolved ref, falling back to filesystem scan",
			"ref", res.Ref, "reason", res.Reason)
	} else {
		scopeLogger.Warn("distill scope: default-branch ref unresolved, falling back to filesystem scan",
			"ref", res.Ref, "reason", res.Reason)
	}
	return scopeFromFilesystem(vaultDir, learningPages, policy), nil
}

// scopeFromGitTree applies the citation pass over the enumerated git tree:
// each compiled blob is read at its ls-tree sha and its frontmatter
// sources join the cited union. A blob that cannot be read or parsed
// contributes no citations — over-scope, human-visible — because silently
// dropping its learnings from scope would hide them from review.
func scopeFromGitTree(ctx context.Context, vaultDir string, tree *gitDistillTree, res gitref.RefResolution, policy DistillPolicy) ScopeSet {
	all := make(map[string]struct{})
	for _, id := range tree.learningIdentities() {
		all[id] = struct{}{}
	}
	cited := make(map[string]struct{})
	run := gitref.ExecRunner{}
	for _, p := range tree.compiledPaths() {
		content, ok := blobContent(ctx, run, vaultDir, tree.blobs[p])
		if !ok {
			scopeLogger.Warn("distill scope: compiled blob unreadable at ref, treated as uncited", "path", p)
			continue
		}
		sources, ok := parseCompiledSources(content)
		if !ok {
			scopeLogger.Warn("distill scope: compiled frontmatter unparseable at ref, treated as uncited", "path", p)
			continue
		}
		for _, id := range sources {
			cited[id] = struct{}{}
		}
	}
	return NewScopeSet(all, cited, filterScope(all, cited, policy), gitProvenance(res))
}

// scopeFromFilesystem computes the degraded scope (contracts/scope.md
// Fallback): learning identities scanned from .uf/dewey/learnings unioned
// with the SQLite learning/ names when the seam is provided, citations
// parsed from the compiled files on disk.
func scopeFromFilesystem(vaultDir string, learningPages LearningPageNames, policy DistillPolicy) ScopeSet {
	fsTree := scanDistillTreeFS(vaultDir)
	all := make(map[string]struct{})
	for _, id := range fsTree.LearningIdentities {
		all[id] = struct{}{}
	}
	if learningPages != nil {
		names, err := learningPages()
		if err != nil {
			scopeLogger.Warn("distill scope: learning pages unavailable in degraded mode", "err", err)
		} else {
			for _, id := range names {
				all[id] = struct{}{}
			}
		}
	}
	cited := make(map[string]struct{})
	for _, sources := range fsTree.CompiledCitations {
		for _, id := range sources {
			cited[id] = struct{}{}
		}
	}
	return NewScopeSet(all, cited, filterScope(all, cited, policy), ScopeProvenanceDegraded)
}

// filterScope subtracts the cited set and the policy's tag exclusions from
// the learning set (contracts/scope.md Behavior: raw = learnings − cited;
// filtered = raw − policy.exclude_tags.matches(raw)).
func filterScope(all, cited map[string]struct{}, policy DistillPolicy) map[string]struct{} {
	inScope := make(map[string]struct{}, len(all))
	for id := range all {
		if _, isCited := cited[id]; isCited {
			continue
		}
		if policy.ExcludesIdentity(id) {
			continue
		}
		inScope[id] = struct{}{}
	}
	return inScope
}

// gitProvenance renders the git leg's provenance string, git:<ref>:<sha>.
func gitProvenance(res gitref.RefResolution) string {
	return fmt.Sprintf("git:%s:%s", res.Ref, res.SHA)
}
