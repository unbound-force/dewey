// Package artifacts implements the deterministic core of the dewey-publish
// sweep: it enumerates Dewey knowledge artifacts stranded in local git state
// (the working tree, the tracked and untracked parts of stashes, and unpushed
// commits), screens them for adversarial-injection and credential content,
// and materializes an approved publish plan in a temporary git worktree, all
// without mutating the user's checkout, index, or stash stack.
//
// The package is the mechanical layer behind `dewey artifacts plan` and
// `dewey artifacts sync` (design D1 of the dewey-publish change): judgment
// (human gate approval and collision resolutions) lives in the command layer
// and arrives here as explicit parameters.
//
// Dependency contract (design D2): the package imports only Go
// standard-library packages plus the internal sanitize package for
// injection-pattern screening. It must never import store, curate, llm,
// embed, or source; it never opens graph.db and never touches dewey.lock,
// so the sweep runs while a Dewey server holds the exclusive flock on
// .uf/dewey/dewey.lock.
//
// types.go pins the plan and sync JSON contracts (design D5) and the
// internal enumeration candidate shared by every pipeline stage.
package artifacts

// --- Schema version ---

// PlanSchemaVersion is the schema version of the plan JSON document that
// `dewey artifacts plan` emits and `dewey artifacts sync` consumes. Sync
// validates the version before taking any effect and fails closed on plans
// carrying a version it does not support.
const PlanSchemaVersion = 1

// --- Vocabulary constants (design D5, pinned by the plan golden test) ---

// Source-label vocabulary for the sources[] arrays of Artifact and Variant
// entries. Every label enumerators emit and sync validates comes from this
// vocabulary.
const (
	// SourceWorkingTree labels content found in the working tree,
	// regardless of its tracked, ignored, or staged state.
	SourceWorkingTree = "working-tree"

	// SourceStashPrefix prefixes labels for content found in a stash's
	// tracked part: SourceStashPrefix plus the stash reflog label, e.g.
	// "stash:stash@{0}".
	SourceStashPrefix = "stash:"

	// SourceStashUntrackedPrefix prefixes labels for content found in a
	// stash's untracked part (the ^3 commit of a stash created with -u or
	// -a): SourceStashUntrackedPrefix plus the stash reflog label, e.g.
	// "stash-untracked:stash@{0}".
	SourceStashUntrackedPrefix = "stash-untracked:"

	// SourceCommitPrefix prefixes labels for content found in an unpushed
	// commit: SourceCommitPrefix plus the 40-hex commit sha, e.g.
	// "commit:<40-hex>". Commit shas are immutable, so the label is too.
	SourceCommitPrefix = "commit:"
)

// Kind vocabulary for Artifact.Kind.
const (
	// KindLearning marks a stored learning from .uf/dewey/learnings/.
	KindLearning = "learning"

	// KindCompiled marks a compiled knowledge article from
	// .uf/dewey/compiled/.
	KindCompiled = "compiled"

	// KindKnowledge marks curated knowledge-store content from
	// .uf/dewey/knowledge/ or a configured store path.
	KindKnowledge = "knowledge"
)

// Status vocabulary for Artifact.Status.
const (
	// StatusNew marks an artifact whose content is absent from (or differs
	// from) the remote base tree; it belongs to the publish set.
	StatusNew = "new"

	// StatusUnchanged marks an artifact whose path and content sha256
	// already match the remote base tree; it is listed for observability
	// but excluded from the publish set.
	StatusUnchanged = "unchanged"
)

// Gitignore status vocabulary for GitignoreFinding.Status (designs D5, D8).
const (
	// GitignoreStatusOK marks artifact roots not ignored by any pattern.
	GitignoreStatusOK = "ok"

	// GitignoreStatusBareDeweyFix marks artifact roots ignored by an
	// unanchored `dewey` pattern whose source is an in-repo .gitignore;
	// the bounded fix block is carried and applied by sync.
	GitignoreStatusBareDeweyFix = "bare-dewey-fix"

	// GitignoreStatusOtherIgnoreWarning marks artifact roots ignored by
	// some other pattern, including a wholesale `.uf/` ignore that
	// shadows the bare `dewey` line (check-ignore reports only the last
	// matching pattern, a documented degradation). Surfaced at the gate;
	// no fix carried.
	GitignoreStatusOtherIgnoreWarning = "other-ignore-warning"

	// GitignoreStatusIgnoreSourceExternal marks a bare `dewey` pattern
	// whose source file lies outside the repository (a global excludesfile
	// or .git/info/exclude). The fix is skipped and a warning naming the
	// external source is surfaced; the sweep does not error.
	GitignoreStatusIgnoreSourceExternal = "ignore-source-external"
)

// Scanner vocabulary for Finding.Scanner.
const (
	// ScannerInjection marks findings produced by the sanitize package's
	// injection-pattern scan.
	ScannerInjection = "injection"

	// ScannerCredential marks findings produced by the artifacts package's
	// credential-detection pattern set.
	ScannerCredential = "credential"
)

// Severity vocabulary for Finding.Severity, mirroring the sanitize package's
// severity discipline. The blocking threshold is critical or high: candidates
// (or conflict variants) carrying such findings are excluded from the publish
// set into the blocked collection; lower severities ride along as
// non-blocking warnings on the artifact entry.
const (
	// SeverityCritical marks findings that block publication.
	SeverityCritical = "critical"

	// SeverityHigh marks findings that block publication.
	SeverityHigh = "high"

	// SeverityMedium marks advisory findings that do not block
	// publication.
	SeverityMedium = "medium"

	// SeverityLow marks advisory findings that do not block publication.
	SeverityLow = "low"
)

// --- Plan document (design D5) ---

// Plan is the top-level plan JSON document emitted by
// `dewey artifacts plan --json` and consumed, after human gate approval, by
// `dewey artifacts sync`. It is the complete, deterministic record of every
// stranded knowledge artifact in the repository's truly-local git universe,
// the conflicts and blocked paths among them, and the gitignore bug
// detection result.
//
// Canonical serialization: Go struct field order is deterministic under
// encoding/json, and the fields are declared in the exact order of design
// D5's JSON sketch. Every slice is sorted according to its element type's
// documented sort rule, StashRefs keys are emitted in sorted order by
// encoding/json's map marshaling, and no timestamps, temporary paths, or
// other run-volatile data appear anywhere in the document, so two plan runs
// over an unchanged repository marshal byte-identically (the twice-test).
// Producers must initialize collection fields to empty (non-nil) values so
// they render as [] and {} rather than null; the sole conditional field is
// GitignoreFinding.Fix, present if and only if the gitignore status is
// GitignoreStatusBareDeweyFix.
type Plan struct {
	// SchemaVersion is the plan schema version; sync accepts exactly
	// PlanSchemaVersion.
	SchemaVersion int `json:"schema_version"`

	// Repo records the resolved git geometry the plan was computed
	// against.
	Repo Repo `json:"repo"`

	// Gitignore reports the gitignore bug detection result.
	Gitignore GitignoreFinding `json:"gitignore"`

	// Artifacts lists every deduplicated artifact entry, sorted by path.
	// Conflicted and blocked paths never appear here; they appear only in
	// their own collections.
	Artifacts []Artifact `json:"artifacts"`

	// Conflicts lists every same-path/different-content collision across
	// stranded-state sources, sorted by path. Only paths with at least one
	// clean (unblocked) variant appear here.
	Conflicts []Conflict `json:"conflicts"`

	// Blocked lists every path excluded from the publish set by
	// critical-or-high screening findings, sorted by path.
	Blocked []BlockedArtifact `json:"blocked"`

	// Warnings carries human-readable, non-fatal enumeration warnings
	// (skipped symlinks and non-regular files, skipped unsafe basenames,
	// external ignore sources), sorted lexicographically for canonical
	// output.
	Warnings []string `json:"warnings"`

	// Summary is the arithmetic rollup over the collections above.
	Summary Summary `json:"summary"`

	// RenderedSummary is the tool-rendered human summary: the itemized
	// tag-grouped artifact listing, per-conflict bounded unified diffs,
	// blocked entries, the gitignore finding, and warnings. It is rendered
	// by this package, carried in the plan JSON, and relayed verbatim by
	// the command layer (DE-003); `dewey artifacts plan` without --json
	// prints the same string to stdout.
	RenderedSummary string `json:"rendered_summary"`
}

// Repo records the resolved git repository geometry the plan was computed
// against, resolved strictly from local refs: the plan is offline and
// deterministic, and the branch date is resolved at sync time, not here.
type Repo struct {
	// BaseBranch is the resolved default branch name of the publish
	// remote (e.g., "main").
	BaseBranch string `json:"base_branch"`

	// BaseSHA is the 40-hex sha of the fetched base tree the publish set
	// is compared against.
	BaseSHA string `json:"base_sha"`

	// PublishRemote is the remote the chore PR targets: `upstream` when
	// that remote is configured, else `origin`.
	PublishRemote string `json:"publish_remote"`

	// PushRemote is the remote the chore branch is pushed to: `origin`
	// when the publish remote is `upstream` (fork layout), else the
	// publish remote itself.
	PushRemote string `json:"push_remote"`

	// ArtifactRoots lists the sanctioned artifact roots, the default trio
	// plus configured store paths, as slash-terminated paths relative to
	// the repository root, sorted lexicographically. The recorded roots
	// are informational: sync re-derives the sanctioned set at execution
	// time and cross-checks it against this list, failing closed on any
	// mismatch.
	ArtifactRoots []string `json:"artifact_roots"`

	// StashRefs maps every enumerated stash label (both `stash@{N}` and
	// its `stash@{N}^3` untracked part) to its resolved 40-hex commit
	// sha. Stash labels renumber when stashes are pushed or dropped, so
	// sync fetches stash content by the recorded sha, never the mutable
	// label. Map keys are emitted in sorted order by encoding/json.
	StashRefs map[string]string `json:"stash_refs"`
}

// GitignoreFinding reports the plan's gitignore bug detection result
// (design D8): whether the sanctioned artifact roots are ignored, and, when
// the bare `dewey` pattern is the culprit in an in-repo .gitignore, the
// bounded fix block sync applies in the chore branch.
type GitignoreFinding struct {
	// Status is one of the GitignoreStatus* vocabulary values.
	Status string `json:"status"`

	// Fix is the bounded fix block, present if and only if Status is
	// GitignoreStatusBareDeweyFix (omitted otherwise, never null). A
	// recorded fix whose File is not the literal ".gitignore" fails
	// closed at sync.
	Fix *GitignoreFix `json:"fix,omitempty"`
}

// GitignoreFix is the bounded gitignore fix block carried by the plan when
// the bare `dewey` pattern is detected. Anchoring the pattern alone would
// un-ignore the whole .uf/dewey/ tree post-merge, so the block also appends
// explicit re-ignore lines keeping non-publishable runtime state and private
// configuration ignored.
type GitignoreFix struct {
	// File is the in-repo file the fix applies to; always the literal
	// ".gitignore".
	File string `json:"file"`

	// Anchor rewrites the exact bare `dewey` pattern line to its anchored
	// form.
	Anchor GitignoreAnchor `json:"anchor"`

	// Reignore lists the runtime-state and private-configuration paths
	// appended as explicit re-ignore lines (graph.db and its -shm/-wal
	// sidecars, dewey.log, dewey.lock, cache/, config.yaml, sources.yaml,
	// knowledge-stores.yaml). The list is a constant block serialized in
	// its fixed documented order, not sorted.
	Reignore []string `json:"reignore"`
}

// GitignoreAnchor is the single-line pattern rewrite at the heart of the
// gitignore fix: the exact bare `dewey` line becomes `/dewey`, anchoring the
// match to the repository root so .uf/dewey/ artifact paths are no longer
// ignored at any depth.
type GitignoreAnchor struct {
	// From is the exact offending pattern line ("dewey").
	From string `json:"from"`

	// To is the anchored replacement ("/dewey").
	To string `json:"to"`
}

// Artifact is one deduplicated knowledge-artifact entry in the plan. Path is
// the dedup key: entries with the same path and identical content sha256
// collapse to a single entry carrying every source in provenance, while
// differing content at the same path across sources becomes a Conflict
// instead.
//
// Plans sort the artifacts collection by path.
type Artifact struct {
	// Path is the repository-relative, slash-separated path: the dedup,
	// collision, conflict, and resolution key throughout the pipeline.
	Path string `json:"path"`

	// Identity is display and PR-grouping metadata only: the filename for
	// learnings and compiled articles, the store path for knowledge-store
	// content.
	Identity string `json:"identity"`

	// Tag is the topic-namespace grouping key rendered in the summary and
	// PR body (e.g., "auth").
	Tag string `json:"tag"`

	// Kind is one of the Kind* vocabulary values.
	Kind string `json:"kind"`

	// SHA256 is the lowercase 64-hex sha256 of the artifact content.
	SHA256 string `json:"sha256"`

	// Sources lists the stranded-state provenance labels that produced
	// this path with identical content, sorted lexicographically.
	Sources []string `json:"sources"`

	// Status is StatusNew or StatusUnchanged (a base-tree match, excluded
	// from the publish set).
	Status string `json:"status"`

	// Findings carries non-blocking lower-severity screening findings;
	// an artifact carrying only such findings remains publishable, with
	// the findings surfaced in the summary. Critical-or-high findings move
	// the path to the blocked collection instead, so this array never
	// carries them.
	Findings []Finding `json:"findings"`
}

// Conflict is a same-path/different-content collision across stranded-state
// enumeration sources. A path whose content differs from its base-tree entry
// is not a collision: it is a plain new Artifact (updated content
// republished). Resolutions arrive as sync parameters keyed by path.
//
// Plans sort the conflicts collection by path, and each conflict's variants
// by sha256.
type Conflict struct {
	// Path is the repository-relative collision key.
	Path string `json:"path"`

	// Identity is display and PR-grouping metadata, as on Artifact.
	Identity string `json:"identity"`

	// Variants lists every content variant keyed by sha256 and source.
	// Only conflicts with at least one clean variant appear in the plan:
	// a path whose every variant is blocked appears only in the blocked
	// collection.
	Variants []Variant `json:"variants"`
}

// Variant is one content variant of a Conflict: the artifact's content as it
// exists in one or more enumeration sources, identified by its sha256.
//
// Conflicts sort variants by sha256; within a variant, sources are sorted
// lexicographically.
type Variant struct {
	// SHA256 is the lowercase 64-hex sha256 of this variant's content.
	SHA256 string `json:"sha256"`

	// Sources lists the stranded-state provenance labels carrying this
	// exact content, sorted lexicographically.
	Sources []string `json:"sources"`

	// Findings carries this variant's screening findings (empty when
	// clean). A variant with critical-or-high findings is a blocked
	// variant: never offered as, or accepted as, a resolution choice.
	Findings []Finding `json:"findings"`
}

// BlockedArtifact is a path excluded from the publish set because screening
// produced critical-or-high findings, including a conflicted path whose
// every variant is blocked. Blocked artifacts are never written by sync.
//
// Plans sort the blocked collection by path.
type BlockedArtifact struct {
	// Path is the repository-relative path key.
	Path string `json:"path"`

	// Identity is display and PR-grouping metadata, as on Artifact.
	Identity string `json:"identity"`

	// Findings carries the blocking findings that excluded this path; for
	// an all-variants-blocked conflict, the findings across every variant.
	// Matched-content excerpts are deliberately absent (see Finding).
	Findings []Finding `json:"findings"`
}

// Finding is one plan-time screening finding, produced by the injection
// scanner (the sanitize package's pattern scan) or the credential scanner
// (the artifacts package's pattern set).
//
// The sanitize package's matched-content excerpt (its Finding.Context field)
// is deliberately excluded from this shape so blocked findings never
// re-deliver payload into the plan JSON or the human gate.
type Finding struct {
	// Scanner is ScannerInjection or ScannerCredential.
	Scanner string `json:"scanner"`

	// Pattern names the matched pattern or check that produced the
	// finding.
	Pattern string `json:"pattern"`

	// Severity is one of the Severity* vocabulary values; critical and
	// high block publication.
	Severity string `json:"severity"`

	// Line is the 1-indexed line number of the match; zero when not
	// applicable.
	Line int `json:"line"`

	// Message is the human-readable description of the finding.
	Message string `json:"message"`
}

// Summary is the arithmetic rollup over the plan's collections. Its
// invariants: Total == len(Artifacts) == New + Unchanged;
// Conflicts == len(Conflicts); Blocked == len(Blocked). Conflicted and
// blocked paths appear only in their own arrays, never in Artifacts.
type Summary struct {
	// Total is the number of deduplicated artifact entries.
	Total int `json:"total"`

	// New counts artifacts absent from (or differing from) the remote
	// base tree.
	New int `json:"new"`

	// Unchanged counts artifacts whose path and content sha256 already
	// match the remote base tree.
	Unchanged int `json:"unchanged"`

	// Conflicts is the number of collision paths.
	Conflicts int `json:"conflicts"`

	// Blocked is the number of screening-excluded paths.
	Blocked int `json:"blocked"`
}

// --- Sync result (design D5) ---

// SyncResult is the JSON document emitted by `dewey artifacts sync`,
// reporting what the approved plan's materialization produced. Unlike the
// plan, it is a single-run result and carries run-specific values (the
// worktree path, the commit sha); the determinism requirements apply to the
// plan, not the sync report.
type SyncResult struct {
	// Branch is the chore branch name (chore/dewey-artifacts-YYYYMMDD,
	// with the first free numeric suffix on same-day collision); empty
	// when nothing was published.
	Branch string `json:"branch"`

	// Worktree is the absolute path of the temporary git worktree under
	// the OS temp directory, outside the vault and every configured disk
	// source path; empty when nothing was published. The command layer
	// removes it after the PR; failure paths report it for manual cleanup
	// (git worktree prune recovers stale registrations).
	Worktree string `json:"worktree"`

	// Commit is the 40-hex sha of the chore commit; empty when nothing
	// was published.
	Commit string `json:"commit"`

	// FilesWritten lists the repository-relative planned artifact paths
	// written into the worktree, sorted by path. The optional .gitignore
	// fix is reported by GitignoreFixApplied, not listed here.
	FilesWritten []string `json:"files_written"`

	// GitignoreFixApplied reports whether the bounded gitignore fix block
	// was applied in the chore branch.
	GitignoreFixApplied bool `json:"gitignore_fix_applied"`

	// NothingToPublish is true when the plan's publish set was empty:
	// sync exits 0 creating no branch, commit, worktree, or PR
	// (idempotence).
	NothingToPublish bool `json:"nothing_to_publish"`

	// PRBody is the tool-rendered, tag-grouped PR-body markdown (artifact
	// identities under sorted tags, per-kind counts, the non-destructive
	// note, and the optional gitignore fix), relayed verbatim by the
	// command layer (DE-003).
	PRBody string `json:"pr_body"`

	// ApprovedBy is the charset-validated approver identity recorded as
	// the Approved-by commit trailer and here in the sync JSON (from
	// --approved-by, falling back to git config user.name).
	ApprovedBy string `json:"approved_by"`

	// PlanSHA256 is the sha256 of the consumed plan file, binding the
	// sync result and the commit's Plan-sha256 trailer to the exact plan
	// content executed.
	PlanSHA256 string `json:"plan_sha256"`
}

// --- Internal enumeration candidate ---

// candidate is the internal enumeration unit shared by the working-tree,
// stash, and commit enumerators and consumed by dedupe, collision detection,
// and screening. Each enumerator emits one candidate per
// (repository-relative path, source); the pipeline then merges candidates
// sharing a path and content sha256 into a single Artifact carrying every
// source, and routes same-path/different-sha256 candidates into a Conflict.
//
// It is unexported deliberately: it carries content bytes and lives only in
// the plan pipeline's memory. The serialized contract is the exported JSON
// vocabulary above.
type candidate struct {
	// Path is the repository-relative, slash-separated path: the dedup,
	// collision, conflict, and resolution key, identical in meaning to
	// Artifact.Path.
	Path string

	// Identity is display and PR-grouping metadata, as on Artifact.
	Identity string

	// Tag is the topic-namespace grouping key, as on Artifact.
	Tag string

	// Kind is one of the Kind* vocabulary values, as on Artifact.
	Kind string

	// SHA256 is the lowercase 64-hex sha256 of Content, computed by the
	// enumerator at read time.
	SHA256 string

	// Source is the single stranded-state provenance label (the D5
	// source-label vocabulary) this candidate was enumerated from; dedupe
	// merges sources into the multi-source Artifact and Variant entries.
	Source string

	// Content is the artifact's bytes, retained so plan-time screening
	// can scan it without re-reading; never serialized.
	Content []byte
}
