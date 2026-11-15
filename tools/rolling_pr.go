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
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"github.com/unbound-force/dewey/v3/gitref"
	"github.com/unbound-force/dewey/v3/sanitize"
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
	segment := normalizeThemeName(theme)
	sum := sha256.Sum256([]byte(theme))
	return segment + "-" + hex.EncodeToString(sum[:])[:hashSuffixLen]
}

// normalizeThemeName is the shared theme-naming base (data-model.md
// Theme.name: sanitized tag/topic): the first four SanitizeThemeBranch
// stages without the branch-only hash suffix. New themes name themselves
// with it, and branches derive from the same normalization, so a theme
// name and its branch segment can never disagree on vocabulary.
func normalizeThemeName(theme string) string {
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
	return segment
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

// --- Rolling PR orchestration (T029-T035; contracts/rolling-pr.md) ---

// GhRunner runs gh CLI invocations for the rolling-PR pipeline. The seam
// mirrors gitref.Runner — argv arrays, never a shell string — so tests
// drive PR state through a fake without the GitHub API.
type GhRunner interface {
	// Run executes gh with args in dir. Semantics match gitref.Runner:
	// err is non-nil only when gh could not run at all; a non-zero exit
	// is reported through code together with stderr.
	Run(ctx context.Context, dir string, args []string) (stdout, stderr string, code int, err error)
}

// GhExecRunner is the production GhRunner: the gh binary run in dir with
// exec.CommandContext, so the parent context's deadline kills a hung gh.
type GhExecRunner struct{}

// Run implements GhRunner by executing the gh binary in dir.
func (GhExecRunner) Run(ctx context.Context, dir string, args []string) (string, string, int, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && ctx.Err() == nil {
		// A non-zero exit is an answer from gh, not a failure to run it.
		return stdout.String(), stderr.String(), exitErr.ExitCode(), nil
	}
	if err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return "", stderr.String(), -1, fmt.Errorf("gh %s: %w", strings.Join(args, " "), cerr)
		}
		return "", stderr.String(), -1, fmt.Errorf("gh %s: %w", strings.Join(args, " "), err)
	}
	return stdout.String(), stderr.String(), 0, nil
}

// OpenDistillPR is one open pull request on a dewey-distill/* branch.
type OpenDistillPR struct {
	// Number is the GitHub pull-request number.
	Number int `json:"number"`
	// HeadRefName is the PR's head branch (dewey-distill/<sanitized>).
	HeadRefName string `json:"headRefName"`
}

// ListOpenDistillPRs lists the repository's open dewey-distill PRs
// (contracts/rolling-pr.md Input: gh pr list --label dewey-distill
// --state open). One call per run — every theme's create-or-update
// decision reads this snapshot, so the snapshot is taken after scope and
// grouping, as late as possible before the per-theme work.
func ListOpenDistillPRs(ctx context.Context, gh GhRunner, dir string) ([]OpenDistillPR, error) {
	if gh == nil {
		return nil, fmt.Errorf("distill rolling pr: gh runner is nil")
	}
	stdout, stderr, code, err := gh.Run(ctx, dir, []string{
		"pr", "list", "--label", "dewey-distill", "--state", "open",
		"--json", "number,headRefName", "--limit", "100",
	})
	if err != nil {
		return nil, fmt.Errorf("distill rolling pr: gh pr list: %w", err)
	}
	if code != 0 {
		return nil, fmt.Errorf("distill rolling pr: gh pr list exited %d: %s", code, strings.TrimSpace(stderr))
	}
	var prs []OpenDistillPR
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &prs); err != nil {
		return nil, fmt.Errorf("distill rolling pr: parse gh pr list output: %w", err)
	}
	return prs, nil
}

// Rolling PR outcome states (data-model.md RollingPR transitions).
const (
	// RollingPRCreated marks a theme whose PR was opened this run.
	RollingPRCreated = "created"
	// RollingPRUpdated marks a theme whose open PR received new commits.
	RollingPRUpdated = "updated"
	// RollingPRUntouched marks a theme whose open PR needed no changes —
	// no new learnings, no hygiene edits: no commit, no push, no body
	// edit (FR-011 edge case; SC-004 branch SHA stability).
	RollingPRUntouched = "untouched"
	// RollingPRConflict marks a theme whose rebase conflicted: the PR is
	// labeled needs-human-resolution, its body lists the conflict files,
	// and the remote branch is left at its pre-rebase state so no human
	// change is dropped (FR-015).
	RollingPRConflict = "conflict"
)

// distillCompiledBy is the constant compiled_by marker identifying
// rolling draft articles.
const distillCompiledBy = "dewey-distill-auto"

// RollingPROptions configures one rolling-PR run.
type RollingPROptions struct {
	// VaultDir is the vault root — the git repository whose branches
	// carry the rolling PRs.
	VaultDir string
	// GitRunner runs git; nil selects gitref.ExecRunner.
	GitRunner gitref.Runner
	// GhRunner runs gh; nil selects GhExecRunner.
	GhRunner GhRunner
	// Remote is the git remote holding dewey-distill branches; empty
	// selects "origin".
	Remote string
	// BaseSHA pins the base every branch is created from and rebased
	// onto; empty selects the vault's HEAD at run start.
	BaseSHA string
	// DistillerName is the commit author identifying machine commits;
	// commits by any other author are human commits whose history the
	// rebase must preserve (research.md D-005). Empty selects "Dewey
	// Distiller".
	DistillerName string
	// DistillerEmail pairs with DistillerName in commit authorship.
	DistillerEmail string
}

// RollingPRResult reports one theme's rolling-PR outcome: the branch and
// PR touched, the terminal state, and the invariants the run preserved.
type RollingPRResult struct {
	// Theme is the theme's name.
	Theme string `json:"theme"`
	// Branch is the full dewey-distill/<sanitized> reference.
	Branch string `json:"branch"`
	// PRNumber is the GitHub PR number (0 never happens in practice:
	// every non-conflict path ends with a PR).
	PRNumber int `json:"pr_number"`
	// State is one of RollingPRCreated, RollingPRUpdated,
	// RollingPRUntouched, RollingPRConflict.
	State string `json:"state"`
	// Labels lists the PR's labels after the run.
	Labels []string `json:"labels"`
	// HeadSHA is the branch tip after the run (the remote tip for
	// untouched and conflict states — the run pushed nothing).
	HeadSHA string `json:"head_sha"`
	// HumanCommitsPreserved reports whether every human commit present
	// before the run is still present after it (SC-006).
	HumanCommitsPreserved bool `json:"human_commits_preserved"`
	// ConflictFiles lists the files that conflicted during rebase
	// (conflict state only).
	ConflictFiles []string `json:"conflict_files,omitempty"`
	// SkippedFiles lists regenerated files the run refused to overwrite
	// because a human commit last touched them (FR-015 regeneration
	// guard).
	SkippedFiles []string `json:"skipped_files,omitempty"`
	// TierWritten is the tier the run wrote — always draft (FR-012);
	// the field makes the invariant auditable per result.
	TierWritten string `json:"tier_written"`
}

// RollingPRInput carries one run's grouping output plus the texts and
// lint findings the PRs render from.
type RollingPRInput struct {
	// Themes is the grouping output — one rolling PR per theme.
	Themes []Theme
	// Texts maps learning identity → full file content
	// (CollectGroupInput's Texts): the article render input and the
	// base content hygiene edits apply to.
	Texts map[string]string
	// Lint optionally maps theme name → lint finding lines for the
	// review queue body (#147 anchor_drift / embedding_gap).
	Lint map[string][]string
}

// gitEnv bundles one run's resolved options so every helper shares the
// same resolved seams (dependency injection, CS-007).
type gitEnv struct {
	dir        string
	git        gitref.Runner
	gh         GhRunner
	remote     string
	baseSHA    string
	baseBranch string // original branch name; "" when detached
	name       string
	email      string
}

// runGit runs one git command, translating a non-zero exit into an error
// carrying stderr.
func (e *gitEnv) runGit(ctx context.Context, args ...string) (string, error) {
	stdout, _, code, err := e.runGitCode(ctx, args...)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", fmt.Errorf("distill rolling pr: git %s exited %d", strings.Join(args, " "), code)
	}
	return stdout, nil
}

// runGitCode runs one git command and reports its exit code; err is
// non-nil only when git could not run at all. Callers that need to
// branch on the exit (push leases, rebase conflicts) use this form.
func (e *gitEnv) runGitCode(ctx context.Context, args ...string) (string, string, int, error) {
	stdout, stderr, code, err := e.git.Run(ctx, e.dir, args)
	if err != nil {
		return "", stderr, code, fmt.Errorf("distill rolling pr: git %s: %w", strings.Join(args, " "), err)
	}
	return stdout, stderr, code, nil
}

// ManageRollingPRs creates or updates one rolling PR per theme
// (contracts/rolling-pr.md Behavior): each theme maps to its
// dewey-distill/<sanitized> branch; a theme with an open PR is updated
// in place (rebase preserving human commits, regenerated draft articles,
// review-queue body, force-with-lease push), a theme without one gets a
// fresh branch, draft articles, and a human-gate PR. The vault's working
// tree must be clean of tracked modifications — the run checks out
// branches in the vault, so uncommitted edits would ride along — and the
// original branch is restored when the run ends. Empty updates leave
// their PR untouched (no commit, no push, no body edit — SC-004), and no
// path ever writes tier: validated (FR-012; GuardDistillTier asserts the
// draft tier at the article render).
func ManageRollingPRs(ctx context.Context, opts RollingPROptions, input RollingPRInput) ([]RollingPRResult, error) {
	env, err := newGitEnv(opts)
	if err != nil {
		return nil, err
	}
	restore, err := prepareDistillRun(ctx, env, opts)
	if err != nil {
		return nil, err
	}
	defer restore()

	openPRs, err := ListOpenDistillPRs(ctx, env.gh, env.dir)
	if err != nil {
		return nil, err
	}
	openByBranch := make(map[string]OpenDistillPR, len(openPRs))
	for _, pr := range openPRs {
		openByBranch[pr.HeadRefName] = pr
	}

	themes := append([]Theme(nil), input.Themes...)
	sort.Slice(themes, func(i, j int) bool { return themes[i].Name < themes[j].Name })
	branches := AssignThemeBranches(themeNames(themes))

	var results []RollingPRResult
	for _, theme := range themes {
		result, err := env.manageOneTheme(ctx, theme, branches[theme.Name], openByBranch, input)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

// newGitEnv validates the run's options and builds the resolved git
// environment: injected seams or their exec defaults, the remote, and
// the distiller's author identity.
func newGitEnv(opts RollingPROptions) (*gitEnv, error) {
	if opts.VaultDir == "" {
		return nil, fmt.Errorf("distill rolling pr: vault dir is empty")
	}
	info, err := os.Stat(opts.VaultDir)
	if err != nil {
		return nil, fmt.Errorf("distill rolling pr: stat vault dir: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("distill rolling pr: vault dir %s is not a directory", opts.VaultDir)
	}
	env := &gitEnv{
		dir:   opts.VaultDir,
		git:   opts.GitRunner,
		gh:    opts.GhRunner,
		name:  opts.DistillerName,
		email: opts.DistillerEmail,
	}
	if env.git == nil {
		env.git = gitref.ExecRunner{}
	}
	if env.gh == nil {
		env.gh = GhExecRunner{}
	}
	env.remote = opts.Remote
	if env.remote == "" {
		env.remote = "origin"
	}
	if env.name == "" {
		env.name = "Dewey Distiller"
	}
	if env.email == "" {
		env.email = "distiller@dewey.invalid"
	}
	return env, nil
}

// prepareDistillRun enforces the clean-tree precondition and resolves
// the run's base: the base SHA (explicit option or HEAD) and the
// original branch the returned restore closure checks out when the run
// ends. The run checks out branches in the vault: tracked but
// uncommitted edits would be carried or clobbered. Untracked files
// (run artifacts) do not block branch switches.
func prepareDistillRun(ctx context.Context, env *gitEnv, opts RollingPROptions) (func(), error) {
	if out, err := env.runGit(ctx, "status", "--porcelain", "--untracked-files=no"); err != nil {
		return nil, err
	} else if strings.TrimSpace(out) != "" {
		return nil, fmt.Errorf("distill rolling pr: vault has uncommitted tracked changes; commit or stash before distilling")
	}
	env.baseSHA = opts.BaseSHA
	if env.baseSHA == "" {
		out, err := env.runGit(ctx, "rev-parse", "HEAD")
		if err != nil {
			return nil, err
		}
		env.baseSHA = strings.TrimSpace(out)
	}
	origRef, err := env.runGit(ctx, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return nil, err
	}
	env.baseBranch = strings.TrimSpace(origRef)
	restore := func() {
		if env.baseBranch == "" || env.baseBranch == "HEAD" {
			_, _ = env.runGit(context.Background(), "checkout", env.baseSHA)
			return
		}
		_, _ = env.runGit(context.Background(), "checkout", env.baseBranch)
	}
	return restore, nil
}

// manageOneTheme dispatches one theme: the update path when its branch
// has an open PR, the create path otherwise.
func (e *gitEnv) manageOneTheme(ctx context.Context, theme Theme, branch string, openByBranch map[string]OpenDistillPR, input RollingPRInput) (RollingPRResult, error) {
	if pr, ok := openByBranch[branch]; ok {
		return e.updateThemePR(ctx, theme, branch, pr, input, false)
	}
	return e.createThemePR(ctx, theme, branch, input, false)
}

// createThemePR lands one theme with no open PR (contracts/rolling-pr.md
// step 3): branch from the base, draft articles plus hygiene, PR opened
// with the review-queue body and the human-gate label. A remote branch
// left over from a closed PR is force-replaced under a lease on its
// current tip — a fresh PR, never a reopen (step 5). A push whose lease
// goes stale (a racing run landed first) or a gh pr create that finds a
// PR the snapshot missed retries once through the update path (the
// contract's Concurrency clause); retried marks the second attempt.
func (e *gitEnv) createThemePR(ctx context.Context, theme Theme, branch string, input RollingPRInput, retried bool) (RollingPRResult, error) {
	// The lease baseline: the remote branch's current tip, or empty when
	// it does not exist (an empty expect means "must not exist").
	remoteSHA, err := e.remoteBranchSHA(ctx, branch)
	if err != nil {
		return RollingPRResult{}, err
	}
	if _, err := e.runGit(ctx, "checkout", "-B", branch, e.baseSHA); err != nil {
		return RollingPRResult{}, err
	}
	written, article, err := e.writeThemeFiles(theme, input, nil, nil)
	if err != nil {
		return RollingPRResult{}, err
	}
	committed, err := e.commitTheme(ctx, theme, written)
	if err != nil {
		return RollingPRResult{}, err
	}
	if !committed {
		// Every member dropped at the sanitization gate and no hygiene
		// edit applied: nothing citable, nothing to review. No branch
		// is pushed and no PR opened — the members stay in scope and
		// resurface next run until a human resolves them.
		scopeLogger.Warn("distill rolling pr: theme produced no citable content; skipped",
			"theme", theme.Name, "dropped", len(theme.Members))
		return RollingPRResult{
			Theme:       theme.Name,
			Branch:      branch,
			State:       RollingPRUntouched,
			TierWritten: DistillTierDraft,
		}, nil
	}
	return e.publishCreatedBranch(ctx, theme, branch, article, input, remoteSHA, retried)
}

// publishCreatedBranch pushes the committed branch and opens its PR.
// Both create races — a leased push that went stale because a racing
// run landed the branch first, and a gh create refused because the
// racing run's PR already exists — convert into a single retry through
// the update path against the winner's PR (the one-PR-per-theme
// invariant without looping). Any other failure, or a retry already
// spent, propagates as an error.
func (e *gitEnv) publishCreatedBranch(ctx context.Context, theme Theme, branch string, article ReviewQueueArticle, input RollingPRInput, remoteSHA string, retried bool) (RollingPRResult, error) {
	if err := e.pushBranch(ctx, branch, remoteSHA); err != nil {
		if retried || !isStalePush(err) {
			return RollingPRResult{}, err
		}
		// A racing run created the branch first: our checkout would
		// clobber nothing of theirs (our commit is local), so retry
		// through the update path against the winner's PR.
		return e.retryAsUpdate(ctx, theme, branch, input)
	}
	number, err := e.createPR(ctx, theme, branch, article)
	if err != nil {
		if retried || !isPRAlreadyExists(err) {
			return RollingPRResult{}, err
		}
		// gh refuses a second open PR for a branch: a racing run's PR
		// exists. Retry as update against it.
		return e.retryAsUpdate(ctx, theme, branch, input)
	}
	return e.createdResult(ctx, theme, branch, number)
}

// createdResult assembles the created-state result: the pushed branch
// tip, the PR number, and the run's labels.
func (e *gitEnv) createdResult(ctx context.Context, theme Theme, branch string, number int) (RollingPRResult, error) {
	head, err := e.runGit(ctx, "rev-parse", "HEAD")
	if err != nil {
		return RollingPRResult{}, err
	}
	scopeLogger.Info("distill rolling pr: opened", "theme", theme.Name, "branch", branch, "pr", number)
	return RollingPRResult{
		Theme:                 theme.Name,
		Branch:                branch,
		PRNumber:              number,
		State:                 RollingPRCreated,
		Labels:                []string{LabelHumanGate, LabelDeweyDistill},
		HeadSHA:               strings.TrimSpace(head),
		HumanCommitsPreserved: true,
		TierWritten:           DistillTierDraft,
	}, nil
}

// retryAsUpdate re-runs a theme whose create lost a race: re-list the
// open PRs, and where the winner's PR exists run the update path; the
// single retry keeps the racing-run invariant (one PR per theme) without
// looping.
func (e *gitEnv) retryAsUpdate(ctx context.Context, theme Theme, branch string, input RollingPRInput) (RollingPRResult, error) {
	openPRs, err := ListOpenDistillPRs(ctx, e.gh, e.dir)
	if err != nil {
		return RollingPRResult{}, err
	}
	for _, pr := range openPRs {
		if pr.HeadRefName == branch {
			scopeLogger.Warn("distill rolling pr: racing create detected, updating the winner's PR", "branch", branch)
			return e.updateThemePR(ctx, theme, branch, pr, input, true)
		}
	}
	return RollingPRResult{}, fmt.Errorf("distill rolling pr: push lease went stale but no open PR exists for %s", branch)
}

// updateThemePR updates a theme's open PR (contracts/rolling-pr.md step
// 2): fetch the branch, decide pre-checkout whether anything the run
// writes would change (nothing changes → untouched, SC-004), rebase onto
// the base preserving human commits, write the files the regeneration
// guard allows, commit, force-with-lease push, and edit the PR body. A
// rebase conflict aborts the rebase, labels the PR
// needs-human-resolution, and pushes nothing (FR-015).
func (e *gitEnv) updateThemePR(ctx context.Context, theme Theme, branch string, pr OpenDistillPR, input RollingPRInput, retried bool) (RollingPRResult, error) {
	if _, err := e.runGit(ctx, "fetch", e.remote, fmt.Sprintf("+%s:refs/remotes/%s/%s", branch, e.remote, branch)); err != nil {
		return RollingPRResult{}, err
	}
	tracking := fmt.Sprintf("refs/remotes/%s/%s", e.remote, branch)
	remoteSHAOut, err := e.runGit(ctx, "rev-parse", tracking)
	if err != nil {
		return RollingPRResult{}, err
	}
	remoteSHA := strings.TrimSpace(remoteSHAOut)

	// Pre-checkout analysis: the regeneration guard (which paths a human
	// commit last touched) and the emptiness check (would anything the
	// run writes differ from the branch's current content).
	plan, err := e.planThemeWrites(ctx, theme, input, tracking)
	if err != nil {
		return RollingPRResult{}, err
	}
	if !plan.hasChanges && len(plan.skipped) == 0 {
		// SC-004: no new learnings and no pending hygiene — the PR is
		// left untouched: no checkout, no commit, no push, no body edit.
		return RollingPRResult{
			Theme:                 theme.Name,
			Branch:                branch,
			PRNumber:              pr.Number,
			State:                 RollingPRUntouched,
			Labels:                []string{LabelHumanGate, LabelDeweyDistill},
			HeadSHA:               remoteSHA,
			HumanCommitsPreserved: true,
			TierWritten:           DistillTierDraft,
		}, nil
	}

	if _, err := e.runGit(ctx, "checkout", "-B", branch, tracking); err != nil {
		return RollingPRResult{}, err
	}
	humanBefore, err := e.humanCommitSubjects(ctx, tracking)
	if err != nil {
		return RollingPRResult{}, err
	}

	// Rebase onto the run's base, preserving every commit (human and
	// machine). A conflict surfaces the files and leaves the remote
	// branch untouched: the human resolves interactively, where the
	// conflict markers are actually usable — an unattended run must
	// never leave the vault mid-rebase with the MCP server holding the
	// store lock.
	if _, stderr, code, err := e.runGitCode(ctx, "rebase", e.baseSHA); err != nil {
		return RollingPRResult{}, err
	} else if code != 0 {
		return e.conflictPath(ctx, theme, branch, pr, input, plan, stderr, remoteSHA)
	}
	return e.finalizeThemeUpdate(ctx, theme, branch, pr, input, plan, remoteSHA, humanBefore, retried)
}

// finalizeThemeUpdate performs the update path's post-rebase tail:
// write the theme's files, commit — or treat nothing-staged as the
// untouched state with its skip notes — push under the lease (retrying
// once when a racing run moved the remote), verify the human commits
// survived the rebase, and edit the PR body.
func (e *gitEnv) finalizeThemeUpdate(ctx context.Context, theme Theme, branch string, pr OpenDistillPR, input RollingPRInput, plan themeWritePlan, remoteSHA string, humanBefore []string, retried bool) (RollingPRResult, error) {
	written, article, err := e.writeThemeFiles(theme, input, plan.skipped, plan.existingCited)
	if err != nil {
		return RollingPRResult{}, err
	}
	result := RollingPRResult{
		Theme:        theme.Name,
		Branch:       branch,
		PRNumber:     pr.Number,
		Labels:       []string{LabelHumanGate, LabelDeweyDistill},
		SkippedFiles: sortedBoolKeys(plan.skipped),
		TierWritten:  DistillTierDraft,
	}
	committed, err := e.commitTheme(ctx, theme, written)
	if err != nil {
		return RollingPRResult{}, err
	}

	// Nothing staged after the rebase merged base changes: the content
	// the run writes already matches the branch. No push — the PR's
	// head stays at remoteSHA (SC-004: an unchanged render must not
	// move the SHA). A regeneration-guard skip still needs its review
	// surface: the body notes the refusal and the PR gains the
	// needs-human-resolution label (FR-015).
	if !committed {
		result.State = RollingPRUntouched
		result.HumanCommitsPreserved = true
		result.HeadSHA = remoteSHA
		if len(plan.skipped) > 0 {
			result.Labels = append(result.Labels, LabelNeedsHumanResolution)
			if err := e.editPRBody(ctx, pr.Number, article, true); err != nil {
				return RollingPRResult{}, err
			}
		}
		return result, nil
	}

	retry, err := e.pushUpdatedBranch(ctx, branch, remoteSHA, retried)
	if err != nil {
		return RollingPRResult{}, err
	}
	if retry {
		return e.updateThemePR(ctx, theme, branch, pr, input, true)
	}

	humanAfter, err := e.humanCommitSubjects(ctx, "HEAD")
	if err != nil {
		return RollingPRResult{}, err
	}
	head, err := e.runGit(ctx, "rev-parse", "HEAD")
	if err != nil {
		return RollingPRResult{}, err
	}
	result.State = RollingPRUpdated
	result.HeadSHA = strings.TrimSpace(head)
	result.HumanCommitsPreserved = subjectsPreserved(humanBefore, humanAfter)

	needsHuman := len(plan.skipped) > 0
	if err := e.editPRBody(ctx, pr.Number, article, needsHuman); err != nil {
		return RollingPRResult{}, err
	}
	if needsHuman {
		result.Labels = append(result.Labels, LabelNeedsHumanResolution)
	}
	scopeLogger.Info("distill rolling pr: updated", "theme", theme.Name, "branch", branch, "pr", pr.Number,
		"human_commits_preserved", result.HumanCommitsPreserved)
	return result, nil
}

// pushUpdatedBranch pushes the updated branch under the lease. A stale
// lease with a retry still available reports retry=true so the caller
// re-runs the update against the racing run's tip (exactly once).
func (e *gitEnv) pushUpdatedBranch(ctx context.Context, branch, remoteSHA string, retried bool) (bool, error) {
	if err := e.pushBranch(ctx, branch, remoteSHA); err != nil {
		if !retried && isStalePush(err) {
			scopeLogger.Warn("distill rolling pr: stale lease on push, retrying as update", "branch", branch)
			return true, nil
		}
		return false, err
	}
	return false, nil
}

// conflictPath handles a rebase conflict (contracts/rolling-pr.md step
// 4; FR-015): collect the conflicted files, abort the rebase to restore
// the branch to its remote state, label the PR needs-human-resolution,
// and edit the body to list the conflict files. Nothing is pushed — the
// remote branch keeps every human commit exactly as it was.
func (e *gitEnv) conflictPath(ctx context.Context, theme Theme, branch string, pr OpenDistillPR, input RollingPRInput, plan themeWritePlan, stderr, remoteSHA string) (RollingPRResult, error) {
	filesOut, _, code, err := e.runGitCode(ctx, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return RollingPRResult{}, err
	}
	var conflictFiles []string
	for _, f := range strings.Split(strings.TrimSpace(filesOut), "\n") {
		if f != "" {
			conflictFiles = append(conflictFiles, f)
		}
	}
	if code != 0 && len(conflictFiles) == 0 {
		return RollingPRResult{}, fmt.Errorf("distill rolling pr: rebase conflicted but conflict files could not be listed (exit %d)", code)
	}
	// Abort restores the branch to its pre-rebase state; the remote was
	// never touched, so every human commit survives verbatim.
	if _, err := e.runGit(ctx, "rebase", "--abort"); err != nil {
		return RollingPRResult{}, fmt.Errorf("distill rolling pr: abort conflicted rebase: %w", err)
	}
	scopeLogger.Warn("distill rolling pr: rebase conflict; labeled needs-human-resolution, remote branch untouched",
		"branch", branch, "files", conflictFiles, "rebase_stderr", strings.TrimSpace(stderr))

	article := ReviewQueueArticle{
		Title:         theme.Name,
		Path:          themeArticlePath(theme),
		WhatChanged:   "rebase onto the new base conflicted; the branch was left at its previous state for manual resolution",
		NewlyCited:    theme.Members,
		Supersessions: renderSupersessionPairs(theme.Supersessions),
		Lint:          input.Lint[theme.Name],
		ConfidenceAvg: themeConfidenceAvg(theme.Decisions),
		Decisions:     theme.Decisions,
		ConflictFiles: conflictFiles,
		SkippedFiles:  sortedBoolKeys(plan.skipped),
	}
	if err := e.editPRBody(ctx, pr.Number, article, true); err != nil {
		return RollingPRResult{}, err
	}
	return RollingPRResult{
		Theme:                 theme.Name,
		Branch:                branch,
		PRNumber:              pr.Number,
		State:                 RollingPRConflict,
		Labels:                []string{LabelHumanGate, LabelDeweyDistill, LabelNeedsHumanResolution},
		HeadSHA:               remoteSHA,
		HumanCommitsPreserved: true,
		ConflictFiles:         conflictFiles,
		SkippedFiles:          sortedBoolKeys(plan.skipped),
		TierWritten:           DistillTierDraft,
	}, nil
}

// themeWritePlan is the pre-checkout analysis of one theme's update:
// which paths the regeneration guard protects, whether anything the run
// writes would differ from the branch's current content, and what the
// branch's article already cites (the body's newly-cited baseline).
type themeWritePlan struct {
	// skipped maps vault-relative path → true where a human commit last
	// touched the path on the PR branch (FR-015 regeneration guard: the
	// run refuses to overwrite human work).
	skipped map[string]bool
	// hasChanges reports whether the article render or any pending
	// hygiene edit differs from the branch's current content. False
	// means the PR is left untouched (SC-004).
	hasChanges bool
	// existingCited lists the sources the branch's article already
	// cites; members beyond these are the run's newly cited learnings.
	existingCited []string
}

// planThemeWrites runs the update path's pre-checkout analysis against
// the remote-tracking ref: the regeneration guard (last author per
// path), the emptiness check (would-write content vs branch content),
// and the existing-citations read. Read-only — nothing is checked out
// and nothing is written.
func (e *gitEnv) planThemeWrites(ctx context.Context, theme Theme, input RollingPRInput, tracking string) (themeWritePlan, error) {
	plan := themeWritePlan{skipped: make(map[string]bool)}
	articlePath := themeArticlePath(theme)

	type hygieneTarget struct{ target, successor string }
	var hygiene []hygieneTarget
	for _, link := range theme.Supersessions {
		if themeHasMember(theme, link.Target) {
			hygiene = append(hygiene, hygieneTarget{target: link.Target, successor: link.Successor})
		}
	}
	sort.Slice(hygiene, func(i, j int) bool {
		if hygiene[i].target != hygiene[j].target {
			return hygiene[i].target < hygiene[j].target
		}
		return hygiene[i].successor < hygiene[j].successor
	})

	probe := func(path string) {
		// A path no commit ever touched is new to this branch — no
		// human edit possible. A path whose last author is not the
		// distiller carries human work the run must not overwrite.
		if author := e.lastAuthorAtPath(ctx, tracking, path); author != "" && author != e.name {
			plan.skipped[path] = true
		}
	}
	probe(articlePath)
	hygienePaths := make(map[string]string, len(hygiene)) // path → successor
	for _, h := range hygiene {
		path := distillLearningsPrefix + h.target + ".md"
		hygienePaths[path] = h.successor
		probe(path)
	}

	articleContent, _, err := RenderDraftArticle(theme, input.Texts, DistillTierDraft)
	if err != nil {
		return plan, err
	}
	currentArticle := e.gitShow(ctx, tracking, articlePath)
	plan.existingCited = parseArticleSources(currentArticle)
	if articleContent != "" {
		// An empty render (all members dropped) never counts as a
		// content change: an existing article must not be emptied.
		plan.hasChanges = articleContent != currentArticle
	}
	for path, successor := range hygienePaths {
		if plan.skipped[path] {
			continue
		}
		if _, changed := RenderSupersededLearning(e.gitShow(ctx, tracking, path), successor); changed {
			plan.hasChanges = true
		}
	}
	return plan, nil
}

// writeThemeFiles writes the theme's draft article and pending learning
// hygiene into the working tree (the caller has the theme branch checked
// out), skipping paths the regeneration guard protects, and returns the
// written paths plus the theme's review-queue article section.
func (e *gitEnv) writeThemeFiles(theme Theme, input RollingPRInput, skip map[string]bool, existingCited []string) ([]string, ReviewQueueArticle, error) {
	articlePath := themeArticlePath(theme)
	content, dropped, err := RenderDraftArticle(theme, input.Texts, DistillTierDraft)
	if err != nil {
		return nil, ReviewQueueArticle{}, err
	}
	var written []string
	// An empty render (every member dropped at the sanitization gate)
	// writes no article: an existing article is never emptied — the
	// drops surface in the PR body instead.
	if content != "" && !skip[articlePath] {
		if err := writeFileInVault(e.dir, articlePath, content); err != nil {
			return nil, ReviewQueueArticle{}, err
		}
		written = append(written, articlePath)
	}
	hygieneWritten, err := e.applyLearningHygiene(theme, skip)
	if err != nil {
		return nil, ReviewQueueArticle{}, err
	}
	written = append(written, hygieneWritten...)

	droppedSet := make(map[string]bool, len(dropped))
	for _, id := range dropped {
		droppedSet[id] = true
	}
	existing := make(map[string]bool, len(existingCited))
	for _, id := range existingCited {
		existing[id] = true
	}
	var newlyCited []string
	for _, m := range theme.Members {
		if !existing[m] && !droppedSet[m] {
			newlyCited = append(newlyCited, m)
		}
	}
	cited := len(theme.Members) - len(dropped)
	whatChanged := fmt.Sprintf("new theme compiled from %d learnings", cited)
	if len(existingCited) > 0 {
		whatChanged = fmt.Sprintf("%d learnings added (%d cited total)", len(newlyCited), cited)
	}
	article := ReviewQueueArticle{
		Title:         theme.Name,
		Path:          articlePath,
		WhatChanged:   whatChanged,
		NewlyCited:    newlyCited,
		TotalCited:    cited,
		Supersessions: renderSupersessionPairs(theme.Supersessions),
		Lint:          input.Lint[theme.Name],
		ConfidenceAvg: themeConfidenceAvg(theme.Decisions),
		Decisions:     theme.Decisions,
		Dropped:       dropped,
		SkippedFiles:  sortedBoolKeys(skip),
	}
	return written, article, nil
}

// applyLearningHygiene applies the theme's supersession hygiene to the
// working tree's learning files (FR-012): each in-theme target gains
// validity: superseded and superseded_by markers when it does not
// already carry them — append-only, never edited away. Paths the
// regeneration guard protects are skipped; an unreadable learning file
// skips its edit (the supersession still surfaces in the PR body, and
// the next run retries). Returns the paths written.
func (e *gitEnv) applyLearningHygiene(theme Theme, skip map[string]bool) ([]string, error) {
	var written []string
	for _, link := range theme.Supersessions {
		if !themeHasMember(theme, link.Target) {
			continue
		}
		path := distillLearningsPrefix + link.Target + ".md"
		if skip[path] {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(e.dir, path))
		if err != nil {
			scopeLogger.Warn("distill rolling pr: learning file unreadable, hygiene edit skipped", "path", path)
			continue
		}
		if next, changed := RenderSupersededLearning(string(raw), link.Successor); changed {
			if err := writeFileInVault(e.dir, path, next); err != nil {
				return nil, err
			}
			written = append(written, path)
		}
	}
	return written, nil
}

// commitTheme stages and commits the written paths with the distiller's
// authorship. The boolean reports whether a commit landed: paths whose
// content already matches the index stage nothing, and "nothing to
// commit" is the untouched state, not an error.
func (e *gitEnv) commitTheme(ctx context.Context, theme Theme, written []string) (bool, error) {
	if len(written) == 0 {
		return false, nil
	}
	args := append([]string{"add", "--"}, written...)
	if _, err := e.runGit(ctx, args...); err != nil {
		return false, err
	}
	staged, err := e.runGit(ctx, "diff", "--cached", "--name-only")
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(staged) == "" {
		return false, nil
	}
	author := fmt.Sprintf("%s <%s>", e.name, e.email)
	message := fmt.Sprintf("distill: draft %s (%d learnings)", theme.Name, len(theme.Members))
	if _, err := e.runGit(ctx, "commit", "--author", author, "-m", message); err != nil {
		return false, err
	}
	return true, nil
}

// pushBranch pushes the checked-out branch with a force-with-lease CAS
// against expected — the remote tip the run observed. An empty expected
// means the branch must not already exist (the create race's guard).
// Every push the distiller makes is leased; a bare force is never used
// (agent boundary: "git push --force-with-lease" only).
func (e *gitEnv) pushBranch(ctx context.Context, branch, expected string) error {
	_, stderr, code, err := e.runGitCode(ctx, "push", "--force-with-lease="+branch+":"+expected, e.remote, branch)
	if err != nil {
		return err
	}
	if code != 0 {
		return &pushError{stderr: stderr, code: code}
	}
	return nil
}

// remoteBranchSHA returns the remote branch's current tip, or "" when
// the branch does not exist on the remote.
func (e *gitEnv) remoteBranchSHA(ctx context.Context, branch string) (string, error) {
	stdout, _, code, err := e.runGitCode(ctx, "ls-remote", e.remote, "refs/heads/"+branch)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", nil
	}
	fields := strings.Fields(stdout)
	if len(fields) == 0 {
		return "", nil
	}
	return fields[0], nil
}

// gitShow reads a path's content at ref; absent or unreadable paths
// return "" (the caller treats that as "not present on the branch").
func (e *gitEnv) gitShow(ctx context.Context, ref, path string) string {
	stdout, _, code, err := e.runGitCode(ctx, "show", ref+":"+path)
	if err != nil || code != 0 {
		return ""
	}
	return stdout
}

// lastAuthorAtPath returns the author name of the last commit that
// touched path at ref; "" when no commit ever touched it.
func (e *gitEnv) lastAuthorAtPath(ctx context.Context, ref, path string) string {
	stdout, _, code, err := e.runGitCode(ctx, "log", "-1", "--format=%an", ref, "--", path)
	if err != nil || code != 0 {
		return ""
	}
	return strings.TrimSpace(stdout)
}

// humanCommitSubjects lists the commit subjects at ref whose author is
// not the distiller — the human work the rebase must preserve (D-005:
// human commits detected via git log author not Dewey Distiller).
func (e *gitEnv) humanCommitSubjects(ctx context.Context, ref string) ([]string, error) {
	stdout, err := e.runGit(ctx, "log", ref, "--format=%an\x1f%s")
	if err != nil {
		return nil, err
	}
	var subjects []string
	for _, line := range strings.Split(strings.TrimRight(stdout, "\n"), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\x1f", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == e.name {
			continue
		}
		subjects = append(subjects, parts[1])
	}
	return subjects, nil
}

// subjectsPreserved reports whether every before-subject survives in
// after (with multiplicity) — the SC-006 rebase invariant.
func subjectsPreserved(before, after []string) bool {
	counts := make(map[string]int, len(after))
	for _, s := range after {
		counts[s]++
	}
	for _, s := range before {
		if counts[s] == 0 {
			return false
		}
		counts[s]--
	}
	return true
}

// runGh runs one gh invocation, translating a non-zero exit into a
// *ghError the caller can classify (racing creates, missing labels).
func (e *gitEnv) runGh(ctx context.Context, operation string, args ...string) (string, error) {
	stdout, stderr, code, err := e.gh.Run(ctx, e.dir, args)
	if err != nil {
		return "", fmt.Errorf("distill rolling pr: gh %s: %w", operation, err)
	}
	if code != 0 {
		return "", &ghError{operation: operation, stderr: strings.TrimSpace(stderr), code: code}
	}
	return stdout, nil
}

// createPR opens the theme's pull request with the review-queue body
// and the human-gate + dewey-distill labels, returning the PR number.
func (e *gitEnv) createPR(ctx context.Context, theme Theme, branch string, article ReviewQueueArticle) (int, error) {
	bodyPath, err := writeBodyFile(RenderReviewQueueBody([]ReviewQueueArticle{article}))
	if err != nil {
		return 0, err
	}
	defer os.Remove(bodyPath)
	args := []string{
		"pr", "create",
		"--title", "dewey-distill: " + theme.Name,
		"--body-file", bodyPath,
		"--label", LabelHumanGate,
		"--label", LabelDeweyDistill,
	}
	if e.baseBranch != "" && e.baseBranch != "HEAD" {
		args = append(args, "--base", e.baseBranch)
	}
	stdout, err := e.runGh(ctx, "pr create", args...)
	if err != nil {
		return 0, err
	}
	return parsePRNumber(stdout)
}

// editPRBody replaces the PR's body with the rendered review queue and,
// when needsHuman is set, adds the needs-human-resolution label.
func (e *gitEnv) editPRBody(ctx context.Context, number int, article ReviewQueueArticle, needsHuman bool) error {
	bodyPath, err := writeBodyFile(RenderReviewQueueBody([]ReviewQueueArticle{article}))
	if err != nil {
		return err
	}
	defer os.Remove(bodyPath)
	args := []string{"pr", "edit", strconv.Itoa(number), "--body-file", bodyPath}
	if needsHuman {
		args = append(args, "--add-label", LabelNeedsHumanResolution)
	}
	_, err = e.runGh(ctx, "pr edit", args...)
	return err
}

// writeBodyFile writes a PR body to a private temp file (gh reads bodies
// from files, keeping multi-kilobyte bodies out of argv).
func writeBodyFile(body string) (string, error) {
	f, err := os.CreateTemp("", "dewey-distill-body-*.md")
	if err != nil {
		return "", fmt.Errorf("distill rolling pr: body temp file: %w", err)
	}
	if _, err := f.WriteString(body); err != nil {
		_ = f.Close()
		return "", fmt.Errorf("distill rolling pr: write body temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("distill rolling pr: close body temp file: %w", err)
	}
	return f.Name(), nil
}

// parsePRNumber extracts the PR number from gh pr create's output (the
// new PR's URL, e.g. https://github.com/owner/repo/pull/12).
func parsePRNumber(out string) (int, error) {
	fields := strings.Fields(strings.TrimSpace(out))
	if len(fields) == 0 {
		return 0, fmt.Errorf("distill rolling pr: gh pr create returned no URL")
	}
	number, err := strconv.Atoi(filepath.Base(fields[len(fields)-1]))
	if err != nil {
		return 0, fmt.Errorf("distill rolling pr: parse PR number from %q: %w", out, err)
	}
	return number, nil
}

// pushError carries a non-zero git push exit for race classification.
type pushError struct {
	stderr string
	code   int
}

// Error implements error with the push's stderr.
func (p *pushError) Error() string {
	return fmt.Sprintf("distill rolling pr: git push exited %d: %s", p.code, strings.TrimSpace(p.stderr))
}

// isStalePush reports whether a push error is a lease rejection — the
// remote moved between the run's fetch and its push (a racing run landed
// first). Only these retry once as an update.
func isStalePush(err error) bool {
	var p *pushError
	if !errors.As(err, &p) {
		return false
	}
	return strings.Contains(p.stderr, "stale info") || strings.Contains(p.stderr, "rejected")
}

// ghError carries a non-zero gh exit for race classification.
type ghError struct {
	operation string
	stderr    string
	code      int
}

// Error implements error with gh's stderr.
func (g *ghError) Error() string {
	return fmt.Sprintf("distill rolling pr: gh %s exited %d: %s", g.operation, g.code, g.stderr)
}

// isPRAlreadyExists reports whether a gh error is the "a pull request
// already exists for this branch" refusal — a racing run created the PR
// between the run's list and its create.
func isPRAlreadyExists(err error) bool {
	var g *ghError
	if !errors.As(err, &g) {
		return false
	}
	return strings.Contains(g.stderr, "already exists")
}

// themeArticlePath resolves the theme's compiled-article path: the
// existing article the grouping assigned onto, or the theme's own
// compiled/<name>.md for a new theme.
func themeArticlePath(theme Theme) string {
	if theme.ExistingArticle != "" {
		return theme.ExistingArticle
	}
	return distillCompiledPrefix + theme.Name + ".md"
}

// themeHasMember reports whether identity is one of the theme's members.
func themeHasMember(theme Theme, identity string) bool {
	for _, m := range theme.Members {
		if m == identity {
			return true
		}
	}
	return false
}

// sortedBoolKeys lists a bool-map's keys in sorted order.
func sortedBoolKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// themeNames lists the themes' names in order for branch assignment.
func themeNames(themes []Theme) []string {
	names := make([]string, 0, len(themes))
	for _, t := range themes {
		names = append(names, t.Name)
	}
	return names
}

// renderSupersessionPairs renders supersession links as
// "<successor> supersedes <target>" strings for the review queue body.
func renderSupersessionPairs(links []SupersessionLink) []string {
	out := make([]string, 0, len(links))
	for _, l := range links {
		out = append(out, l.Successor+" supersedes "+l.Target)
	}
	return out
}

// themeConfidenceAvg computes the mean decision confidence; zero when
// the theme carries no decisions.
func themeConfidenceAvg(decisions []AssignmentDecision) float64 {
	if len(decisions) == 0 {
		return 0
	}
	sum := 0.0
	for _, d := range decisions {
		sum += d.Confidence
	}
	return sum / float64(len(decisions))
}

// RenderDraftArticle renders one theme's compiled article at draft tier
// (FR-012): YAML frontmatter (tier, compiled_by, sources, topic) plus
// one `## <identity>` section per kept member's information text. The
// render is deterministic — no timestamps — so the update path's
// emptiness check is a plain content comparison against the branch's
// article (SC-004: an unchanged render must not produce a commit).
//
// The sanitization gate (FR-013) scans every member's full text with
// sanitize.Scan; a member with critical/high findings — or with no
// text, or whose scrubbed body is empty — is dropped from the article
// and returned in dropped so the PR body can note it. Kept member
// content is scrubbed of HTML comments and data: URIs before it enters
// the article (the contract's synthesis-content rule: the compiled
// article is the synthesis output, so it omits exactly what a synthesis
// prompt would). A render where every member dropped returns "" —
// callers must not write that over an existing article.
func RenderDraftArticle(theme Theme, texts map[string]string, tier string) (string, []string, error) {
	if err := GuardDistillTier(tier); err != nil {
		return "", nil, err
	}
	var dropped []string
	var sections []string
	var cited []string
	for _, member := range theme.Members {
		text, ok := texts[member]
		if !ok {
			dropped = append(dropped, member)
			continue
		}
		scanned, err := sanitize.Scan(sanitize.ScanInput{
			Content:    text,
			SourceID:   "dewey-distill",
			DocumentID: distillLearningsPrefix + member + ".md",
			SourceType: "disk",
		}, sanitize.ScanConfig{Mode: sanitize.ModeStrict})
		if err != nil {
			return "", nil, fmt.Errorf("distill rolling pr: sanitize %s: %w", member, err)
		}
		if sanitize.HasBlockingFindings(scanned.Findings) {
			dropped = append(dropped, member)
			continue
		}
		body := stripUnsafeLearningContent(bodyAfterFrontmatter(text))
		if body == "" {
			dropped = append(dropped, member)
			continue
		}
		sections = append(sections, "## "+member+"\n\n"+body)
		cited = append(cited, member)
	}
	if len(sections) == 0 {
		return "", dropped, nil
	}
	var sb strings.Builder
	sb.WriteString("---\n")
	fmt.Fprintf(&sb, "tier: %s\n", tier)
	fmt.Fprintf(&sb, "compiled_by: %s\n", distillCompiledBy)
	sb.WriteString("sources:\n")
	for _, id := range cited {
		fmt.Fprintf(&sb, "  - %s\n", id)
	}
	fmt.Fprintf(&sb, "topic: %s\n", theme.Name)
	sb.WriteString("---\n\n")
	for _, s := range sections {
		sb.WriteString(s)
		sb.WriteString("\n\n")
	}
	return sb.String(), dropped, nil
}

// RenderSupersededLearning applies learning hygiene to one learning
// file's content (FR-012): the superseded target gains
// validity: "superseded" and superseded_by: "<successor>" frontmatter
// markers through the #146 setFrontmatterKeys writer. The edit is
// idempotent for a matching pair and append-only against recorded
// state: a file that already records a different successor is returned
// unchanged (changed=false) — a recorded supersession outranks the
// run's grouping claim, and the pair still surfaces in the PR body for
// human review.
func RenderSupersededLearning(content, successor string) (string, bool) {
	recorded, found := frontmatterScalar(content, "superseded_by")
	if found && recorded != "" && recorded != successor {
		return content, false
	}
	updated, err := setFrontmatterKeys(content, [][2]string{
		{"validity", "superseded"},
		{"superseded_by", successor},
	})
	if err != nil {
		// No frontmatter (or unterminated) — hygiene does not apply.
		return content, false
	}
	return updated, updated != content
}

// Review-queue body bounds (contracts/rolling-pr.md Review Queue Body:
// the assembled body caps at 65k chars; identity lists truncate at
// 100 items).
const (
	maxReviewQueueBodyChars   = 65 * 1024
	maxReviewQueueListItems   = 100
	reviewQueueTruncationNote = "\n\n(body truncated at the 65k character cap)\n"
)

// RenderReviewQueueBody renders a rolling PR's review-queue body
// (contracts/rolling-pr.md Review Queue Body): the human-gate preamble
// followed by one section per article — the escaped What changed,
// Newly cited, Supersessions, Lint, and Confidence lines from
// data-model.md's ReviewQueue — plus the sanitization-drop,
// regeneration-guard-skip, and conflict notes. Every interpolated field
// is escaped per the sanitization gate; identity lists truncate at 100
// items with a pointer to the article file; the assembled body caps at
// 65k characters without splitting a multi-byte rune.
func RenderReviewQueueBody(articles []ReviewQueueArticle) string {
	var sb strings.Builder
	sb.WriteString("This pull request was opened by the unattended weekly dewey-distill run. It carries the `human-gate` label: review and merge it yourself — nothing merges automatically, and no page here reads `tier: validated` until this merge vouches it.\n\n")
	for _, a := range articles {
		fmt.Fprintf(&sb, "## %s (`%s`)\n", escapeMarkdown(a.Title), escapeMarkdown(a.Path))
		if a.WhatChanged != "" {
			fmt.Fprintf(&sb, "**What changed**: %s\n", escapeMarkdown(a.WhatChanged))
		}
		writeNewlyCited(&sb, a)
		writeBacktickedList(&sb, "Supersessions", a.Supersessions)
		writeBacktickedList(&sb, "Lint", a.Lint)
		writeConfidence(&sb, a)
		writeDropped(&sb, a)
		for _, f := range a.SkippedFiles {
			fmt.Fprintf(&sb, "regeneration guard: skipped `%s` (human edit preserved)\n", escapeMarkdown(f))
		}
		for _, f := range a.ConflictFiles {
			fmt.Fprintf(&sb, "conflict: `%s` — rebase onto the new base conflicted; the branch was left at its previous state for manual resolution\n", escapeMarkdown(f))
		}
		sb.WriteString("\n")
	}
	return capReviewQueueBody(sb.String())
}

// writeNewlyCited renders the **Newly cited** line: `learning/<id>`
// items with a count, truncated at 100 items with a pointer to the
// article file (the article cites every kept member).
func writeNewlyCited(sb *strings.Builder, a ReviewQueueArticle) {
	if len(a.NewlyCited) == 0 {
		return
	}
	shown := a.NewlyCited
	if len(shown) > maxReviewQueueListItems {
		shown = shown[:maxReviewQueueListItems]
	}
	items := make([]string, 0, len(shown))
	for _, id := range shown {
		items = append(items, "`learning/"+escapeMarkdown(id)+"`")
	}
	line := fmt.Sprintf("**Newly cited**: %s (%d)", strings.Join(items, ", "), len(a.NewlyCited))
	if len(a.NewlyCited) > maxReviewQueueListItems {
		line += fmt.Sprintf(" — list truncated; the article `%s` cites every kept member", escapeMarkdown(a.Path))
	}
	sb.WriteString(line + "\n")
}

// writeBacktickedList renders one bold-label line of backticked,
// escaped items (Supersessions pairs, Lint kinds), truncated at 100.
func writeBacktickedList(sb *strings.Builder, label string, items []string) {
	if len(items) == 0 {
		return
	}
	shown := items
	truncated := false
	if len(shown) > maxReviewQueueListItems {
		shown = shown[:maxReviewQueueListItems]
		truncated = true
	}
	quoted := make([]string, 0, len(shown))
	for _, item := range shown {
		quoted = append(quoted, "`"+escapeMarkdown(item)+"`")
	}
	line := fmt.Sprintf("**%s**: %s", label, strings.Join(quoted, ", "))
	if truncated {
		line += " — list truncated"
	}
	sb.WriteString(line + "\n")
}

// writeConfidence renders the **Confidence** line: the decisions' mean
// plus per-decision confidence and source, truncated at 100 decisions.
func writeConfidence(sb *strings.Builder, a ReviewQueueArticle) {
	if len(a.Decisions) == 0 {
		return
	}
	shown := a.Decisions
	truncated := false
	if len(shown) > maxReviewQueueListItems {
		shown = shown[:maxReviewQueueListItems]
		truncated = true
	}
	parts := make([]string, 0, len(shown))
	for _, d := range shown {
		parts = append(parts, fmt.Sprintf("%s: %.2f (%s)", escapeMarkdown(d.Identity), d.Confidence, escapeMarkdown(d.Source)))
	}
	line := fmt.Sprintf("**Confidence**: %.2f avg — %s", a.ConfidenceAvg, strings.Join(parts, ", "))
	if truncated {
		line += " — list truncated"
	}
	sb.WriteString(line + "\n")
}

// writeDropped renders the sanitization-drop notes (FR-013:
// "sanitization: dropped <id> (critical)"), truncated at 100.
func writeDropped(sb *strings.Builder, a ReviewQueueArticle) {
	if len(a.Dropped) == 0 {
		return
	}
	shown := a.Dropped
	if len(shown) > maxReviewQueueListItems {
		shown = shown[:maxReviewQueueListItems]
	}
	for _, id := range shown {
		fmt.Fprintf(sb, "sanitization: dropped `%s` (critical)\n", escapeMarkdown(id))
	}
	if len(a.Dropped) > maxReviewQueueListItems {
		fmt.Fprintf(sb, "sanitization: %d more drops truncated — the run log lists every dropped member\n", len(a.Dropped)-maxReviewQueueListItems)
	}
}

// capReviewQueueBody truncates the assembled body at the 65k character
// cap without splitting a multi-byte rune, appending the truncation
// note when it fires.
func capReviewQueueBody(body string) string {
	if len(body) <= maxReviewQueueBodyChars {
		return body
	}
	cut := body[:maxReviewQueueBodyChars-len(reviewQueueTruncationNote)]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut + reviewQueueTruncationNote
}

// markdownEscaper escapes every Markdown-significant character the
// sanitization gate names, so learning-derived strings interpolated
// into a PR body cannot restructure it.
var markdownEscaper = strings.NewReplacer(
	"[", "\\[",
	"]", "\\]",
	"(", "\\(",
	")", "\\)",
	"!", "\\!",
	"#", "\\#",
	"*", "\\*",
	"_", "\\_",
	"`", "\\`",
	"<", "\\<",
	">", "\\>",
)

// escapeMarkdown escapes [ ] ( ) ! # * _ ` < > in s per the
// sanitization gate's interpolation rule.
func escapeMarkdown(s string) string {
	return markdownEscaper.Replace(s)
}

var (
	// htmlCommentPattern matches HTML comment blocks (possibly
	// multi-line) for the synthesis-content scrub.
	htmlCommentPattern = regexp.MustCompile(`(?s)<!--.*?-->`)
	// dataURIPattern matches data: URIs embedded in text.
	dataURIPattern = regexp.MustCompile(`data:[^\s)>"']*`)
)

// stripUnsafeLearningContent removes HTML comment blocks and data:
// URIs from learning content entering a compiled article — the
// contract's synthesis-content rule: the compiled article is the
// synthesis output, so it omits exactly what a synthesis prompt would.
func stripUnsafeLearningContent(text string) string {
	stripped := htmlCommentPattern.ReplaceAllString(text, "")
	stripped = dataURIPattern.ReplaceAllString(stripped, "")
	return strings.TrimSpace(stripped)
}

// bodyAfterFrontmatter strips a markdown file's YAML frontmatter
// block, returning only the body content (learning files carry their
// information text after the frontmatter). Content without frontmatter
// passes through unchanged; unterminated frontmatter is treated as
// body, mirroring frontmatterBlock's boundary rules.
func bodyAfterFrontmatter(content string) string {
	if !strings.HasPrefix(content, "---") {
		return content
	}
	idx := strings.Index(content[3:], "\n---")
	if idx < 0 {
		return content
	}
	return strings.TrimSpace(strings.TrimLeft(content[3+idx+4:], "\r\n"))
}

// parseArticleSources extracts the sources list from a compiled
// article's YAML frontmatter; absent or unparseable frontmatter and a
// missing sources key all yield nil.
func parseArticleSources(content string) []string {
	block, ok := frontmatterBlock(content)
	if !ok {
		return nil
	}
	var props map[string]any
	if err := yaml.Unmarshal([]byte(block), &props); err != nil {
		return nil
	}
	raw, ok := props["sources"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		if s, ok := r.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// writeFileInVault writes content to the vault-relative path, creating
// parent directories as needed. The cleaned path must stay under the
// vault root — absolute paths and .. escapes are rejected (SC-004
// traversal guard; defense in depth beneath the branch-slug
// sanitization, which already constrains the article path charset).
func writeFileInVault(dir, rel, content string) error {
	if rel == "" {
		return fmt.Errorf("distill rolling pr: empty write path")
	}
	cleaned := filepath.Clean(rel)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) || filepath.IsAbs(cleaned) {
		return fmt.Errorf("distill rolling pr: write path escapes vault root: %s", rel)
	}
	path := filepath.Join(dir, cleaned)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("distill rolling pr: create %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("distill rolling pr: write %s: %w", path, err)
	}
	return nil
}
