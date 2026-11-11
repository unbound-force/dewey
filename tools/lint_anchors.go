package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/unbound-force/dewey/v3/store"
)

// Anchor checks (issue #147): staleness from code drift, not age.
//
// A learning pinned with `as_of: <repo>@<sha>` (issue #146) whose evidence
// cites `<repo>:<path>` for the same repository is checked against local
// git: the path must exist at the pin (anchor_missing otherwise) and must
// not have changed between the pin and HEAD (anchor_drift otherwise).
//
// Design decision: git access sits behind anchorChecker so tests can
// exercise failure paths; production uses local `git -C <vault>` only —
// `<repo>#<n>` locators are not resolved because that needs the network.

// anchorLineSuffix matches a trailing `:<line>` or `:<line>-<line>` on a
// locator path.
var anchorLineSuffix = regexp.MustCompile(`:\d+(-\d+)?$`)

// anchorCommitSHA matches a hexadecimal commit name. as_of can arrive
// through re-ingested, untrusted frontmatter and is passed to git as a
// revision, so anything else (for example "--output=x") is never a pin.
var anchorCommitSHA = regexp.MustCompile(`^[0-9a-fA-F]+$`)

const (
	// anchorGitTimeout bounds each git subprocess run by the anchor checker.
	anchorGitTimeout = 5 * time.Second
	// anchorCheckBudget bounds one whole anchor-check pass. Evidence can
	// arrive through re-ingested frontmatter that bypassed store_learning's
	// limits, so the pass is bounded in total, not only per call.
	anchorCheckBudget = 60 * time.Second
)

// errAnchorNoRepo reports that the vault is not a git repository with a
// commit, which skips the anchor checks rather than failing them.
var errAnchorNoRepo = errors.New("vault is not a git repository")

// Anchor-check outcomes reported in the lint result's anchor_checks.
const (
	anchorChecked = "checked"
	anchorSkipped = "skipped"
	anchorFailed  = "failed"
)

// anchorChecker answers the git questions the anchor checks need. Every
// call honors ctx: a cancelled or expired context stops the pass.
type anchorChecker interface {
	Head(ctx context.Context) (string, error)
	// Repo names the repository the way as_of pins do: the base name of
	// its top-level directory (see productionAsOfResolver).
	Repo(ctx context.Context) (string, error)
	CommitExists(ctx context.Context, sha string) (bool, error)
	PathExists(ctx context.Context, sha, path string) (bool, error)
	PathChanged(ctx context.Context, fromSHA, toSHA, path string) (bool, error)
}

// gitAnchorChecker implements anchorChecker with `git -C dir`. Each git
// run is bounded by timeout, or anchorGitTimeout when it is zero.
type gitAnchorChecker struct {
	dir     string
	timeout time.Duration
}

// Head returns the full HEAD commit of the repository, or errAnchorNoRepo
// when git runs but finds no repository or no commit.
func (g gitAnchorChecker) Head(ctx context.Context) (string, error) {
	out, code, err := g.run(ctx, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", errAnchorNoRepo
	}
	return strings.TrimSpace(out), nil
}

// Repo returns the base name of the repository's top-level directory.
func (g gitAnchorChecker) Repo(ctx context.Context) (string, error) {
	out, code, err := g.run(ctx, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", errAnchorNoRepo
	}
	return filepath.Base(strings.TrimSpace(out)), nil
}

// CommitExists reports whether sha names a commit in the repository.
func (g gitAnchorChecker) CommitExists(ctx context.Context, sha string) (bool, error) {
	_, code, err := g.run(ctx, "cat-file", "-e", sha+"^{commit}")
	return code == 0, err
}

// PathExists reports whether path exists in the tree of commit sha.
func (g gitAnchorChecker) PathExists(ctx context.Context, sha, path string) (bool, error) {
	_, code, err := g.run(ctx, "cat-file", "-e", sha+":"+path)
	return code == 0, err
}

// PathChanged reports whether path differs between two commits (deletion
// counts as a change). `git diff --quiet` exits 1 on a difference. External
// diff drivers and textconv filters are disabled: they come from repository
// config and would run commands chosen by whoever controls the vault. The
// path is passed as a literal pathspec, so glob characters or pathspec
// magic in a file name match only that file.
func (g gitAnchorChecker) PathChanged(ctx context.Context, fromSHA, toSHA, path string) (bool, error) {
	_, code, err := g.run(ctx, "diff", "--quiet", "--no-ext-diff", "--no-textconv", fromSHA, toSHA, "--", ":(literal)"+path)
	if err != nil {
		return false, err
	}
	if code > 1 {
		return false, fmt.Errorf("git diff %s..%s -- %s exited %d", fromSHA, toSHA, path, code)
	}
	return code == 1, nil
}

// run runs git under ctx, bounded by the checker's timeout, and returns
// its stdout and exit code. An error is returned only when git could not
// be run (not found, timeout, cancelled), never for a non-zero exit.
func (g gitAnchorChecker) run(ctx context.Context, args ...string) (string, int, error) {
	timeout := g.timeout
	if timeout <= 0 {
		timeout = anchorGitTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", append([]string{"-C", g.dir}, args...)...).Output()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && ctx.Err() == nil {
		return string(out), exitErr.ExitCode(), nil
	}
	if err != nil {
		return "", -1, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return string(out), 0, nil
}

// splitAsOf splits an `<repo>@<sha>` pin. ok is false when either part is
// missing.
func splitAsOf(asOf string) (repo, sha string, ok bool) {
	i := strings.LastIndex(asOf, "@")
	if i <= 0 || i == len(asOf)-1 {
		return "", "", false
	}
	return asOf[:i], asOf[i+1:], true
}

// parseAnchor returns the repository-relative path of a `<repo>:<path>`
// locator for repo, without a trailing line suffix, or "" when the locator
// is for another repository or is not a path locator.
func parseAnchor(locator, repo string) string {
	prefix := repo + ":"
	if repo == "" || !strings.HasPrefix(locator, prefix) {
		return ""
	}
	path := anchorLineSuffix.ReplaceAllString(strings.TrimPrefix(locator, prefix), "")
	return strings.TrimPrefix(path, "./")
}

// learningAnchors returns the pin commit and distinct anchor paths of a
// learning eligible for anchor checks against repository repo: it has an
// as_of pin to repo, at least one `<repo>:<path>` anchor, and a validity
// other than superseded or historical (historical knowledge is immune to
// code evolution). A learning pinned to another repository is not checked
// here, even when its commit is absent, and neither is one whose evidence
// cites no path in repo: there is nothing in this repository to compare.
//
// Evidence is re-validated against store_learning's limits here because
// re-ingested learnings bypass them: a learning with more than
// maxEvidenceEntries locators, or any locator over maxEvidenceLen, is not
// checked at all.
func learningAnchors(p *store.Page, repo string) (string, []string, bool) {
	var props map[string]any
	if err := json.Unmarshal([]byte(p.Properties), &props); err != nil {
		return "", nil, false
	}
	if v, _ := props["validity"].(string); v == "superseded" || v == "historical" {
		return "", nil, false
	}
	asOf, _ := props["as_of"].(string)
	pinRepo, sha, ok := splitAsOf(asOf)
	if !ok || pinRepo != repo || !anchorCommitSHA.MatchString(sha) {
		return "", nil, false
	}
	evidence, _ := props["evidence"].([]any)
	if len(evidence) > maxEvidenceEntries {
		return "", nil, false
	}
	var paths []string
	seen := make(map[string]bool)
	for _, e := range evidence {
		locator, _ := e.(string)
		if utf8.RuneCountInString(locator) > maxEvidenceLen {
			return "", nil, false
		}
		if path := parseAnchor(locator, repo); path != "" && !seen[path] {
			seen[path] = true
			paths = append(paths, path)
		}
	}
	return sha, paths, len(paths) > 0
}

// shortSHA abbreviates a commit for messages.
func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// anchorFinding builds an anchor finding for learning page p.
func anchorFinding(kind string, p *store.Page, description, remediation string) Finding {
	return Finding{
		Type:        kind,
		Severity:    "warning",
		Identity:    strings.TrimPrefix(p.Name, store.LearningPrefix),
		Page:        p.Name,
		Description: description,
		Remediation: remediation,
	}
}

// anchorReport is the outcome of one anchor-check pass, reported to MCP
// clients as anchor_checks so "checked and clean" is distinguishable from
// "never checked".
type anchorReport struct {
	Status string `json:"status"`           // checked, skipped, or failed
	Reason string `json:"reason,omitempty"` // why the pass was skipped or failed
}

// checkAnchors reports anchor_missing and anchor_drift findings for every
// eligible learning, within l.anchorBudget (anchorCheckBudget when zero). It is skipped when there is
// no checker or the vault is not a git repository, and fails as a whole
// (no findings) when git cannot answer or the budget runs out.
func (l *Lint) checkAnchors(ctx context.Context) ([]Finding, anchorReport, error) {
	if l.anchors == nil {
		return nil, anchorReport{Status: anchorSkipped, Reason: "no vault path"}, nil
	}
	budget := l.anchorBudget
	if budget <= 0 {
		budget = anchorCheckBudget
	}
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	head, err := l.anchors.Head(ctx)
	var repo string
	if err == nil {
		repo, err = l.anchors.Repo(ctx)
	}
	if errors.Is(err, errAnchorNoRepo) {
		lintLogger.Debug("anchor checks skipped", "reason", err)
		return nil, anchorReport{Status: anchorSkipped, Reason: err.Error()}, nil
	}
	if err != nil {
		return nil, anchorReport{Status: anchorFailed, Reason: err.Error()}, err
	}
	pages, err := l.store.ListLearningPages()
	if err != nil {
		err = fmt.Errorf("list learning pages: %w", err)
		return nil, anchorReport{Status: anchorFailed, Reason: err.Error()}, err
	}
	var findings []Finding
	for _, p := range pages {
		sha, paths, ok := learningAnchors(p, repo)
		if !ok {
			continue
		}
		pageFindings, err := checkLearningAnchors(ctx, l.anchors, p, sha, head, paths)
		if err != nil {
			return nil, anchorReport{Status: anchorFailed, Reason: err.Error()}, err
		}
		findings = append(findings, pageFindings...)
	}
	return findings, anchorReport{Status: anchorChecked}, nil
}

// checkLearningAnchors checks one learning's anchors against its pin and
// HEAD. An unknown pin yields a single anchor_missing finding.
func checkLearningAnchors(ctx context.Context, c anchorChecker, p *store.Page, sha, head string, paths []string) ([]Finding, error) {
	known, err := c.CommitExists(ctx, sha)
	if err != nil {
		return nil, err
	}
	if !known {
		return []Finding{anchorFinding("anchor_missing", p,
			fmt.Sprintf("Learning '%s' is pinned to commit %s, which is not in the repository.", strings.TrimPrefix(p.Name, store.LearningPrefix), shortSHA(sha)),
			"Fetch the commit, or store an updated learning pinned to the current HEAD.")}, nil
	}
	var findings []Finding
	for _, path := range paths {
		present, err := c.PathExists(ctx, sha, path)
		if err != nil {
			return nil, err
		}
		if !present {
			findings = append(findings, anchorFinding("anchor_missing", p,
				fmt.Sprintf("Learning '%s' cites %s, which does not exist at %s.", strings.TrimPrefix(p.Name, store.LearningPrefix), path, shortSHA(sha)),
				"Correct the evidence locator in an updated learning that supersedes this one."))
			continue
		}
		changed, err := c.PathChanged(ctx, sha, head, path)
		if err != nil {
			return nil, err
		}
		if changed {
			findings = append(findings, anchorFinding("anchor_drift", p,
				fmt.Sprintf("Learning '%s' cites %s, which changed or was removed between %s and HEAD %s.", strings.TrimPrefix(p.Name, store.LearningPrefix), path, shortSHA(sha), shortSHA(head)),
				fmt.Sprintf("Re-verify the learning against the current code; store an updated learning with supersedes: %s, or run lint with fix to mark it stale-unverified.", strings.TrimPrefix(p.Name, store.LearningPrefix))))
		}
	}
	return findings, nil
}

// fixAnchorDrift marks every learning with an anchor_drift finding as
// validity: stale-unverified. It returns the number of learnings marked in
// the store, and a warning for each learning that could not be marked or
// whose markdown backup could not be updated after the store was.
func (l *Lint) fixAnchorDrift(findings []Finding) (int, []string) {
	marked := 0
	var warnings []string
	seen := make(map[string]bool)
	for _, f := range findings {
		if f.Type != "anchor_drift" || seen[f.Page] {
			continue
		}
		seen[f.Page] = true
		warning, err := l.markStaleUnverified(f.Page)
		if err != nil {
			lintLogger.Warn("failed to mark learning stale-unverified", "page", f.Page, "err", err)
			warnings = append(warnings, fmt.Sprintf("failed to mark %s stale-unverified: %v", f.Page, err))
			continue
		}
		marked++
		if warning != "" {
			lintLogger.Warn("learning backup not updated", "page", f.Page, "warning", warning)
			warnings = append(warnings, warning)
		}
	}
	return marked, warnings
}

// refuseStaleBackup is the lint file leg's refusal policy
// (serialize-supersession-writes design D3): the fix must never overwrite
// a backup that already records a successor — a superseded_by value, or a
// validity of superseded — with stale-unverified. Such a backup is
// pre-fix residue (the store leg already skips a learning whose validity
// became superseded), so the file leg leaves the durable record unchanged
// and the refusal is surfaced as a warning instead.
func refuseStaleBackup(content string) string {
	if recorded, found := frontmatterScalar(content, "superseded_by"); found && recorded != "" {
		return fmt.Sprintf("backup already records superseded_by %q", recorded)
	}
	if validity, found := frontmatterScalar(content, "validity"); found && strings.Trim(validity, `"' `) == "superseded" {
		return "backup already records validity superseded"
	}
	return ""
}

// markStaleUnverified sets validity: stale-unverified on a learning in the
// store and in its markdown backup when one exists. Tier and content are
// untouched. It fails closed before writing anything when the identity is
// unsafe to embed in a path or the stored properties are unreadable, so
// corrupt properties are never replaced wholesale. The store leg and the
// file leg run under a single withLearningPropsLock hold, so a concurrent
// supersession cannot land between them and have this fix's late file leg
// overwrite its validity in the durable backup. The file leg is guarded by
// refuseStaleBackup: a backup that already records a successor or
// validity: superseded is left unchanged, the refusal reported as a
// warning like any other backup failure. Once the store is written
// the learning counts as marked: a backup failure after that is returned
// as a warning, not an error. A learning that became superseded or
// historical after the check is left unchanged and reported as an error.
func (l *Lint) markStaleUnverified(pageName string) (string, error) {
	identity := strings.TrimPrefix(pageName, store.LearningPrefix)
	if !ValidLearningIdentity(identity) {
		return "", fmt.Errorf("unsafe learning identity: %q", identity)
	}
	var warning string
	// Validity is re-read under the caller-level hold: a learning
	// superseded (or made historical) since the check ran keeps that
	// validity, in the store and in the backup alike.
	err := withLearningPropsLock(func() error {
		if err := updateLearningProperties(l.store, pageName, func(props map[string]any) error {
			if v, _ := props["validity"].(string); v == "superseded" || v == "historical" {
				return fmt.Errorf("learning is now %s", v)
			}
			props["validity"] = "stale-unverified"
			return nil
		}); err != nil {
			return err
		}
		if err := updateLearningBackup(l.vaultPath, identity, [][2]string{{"validity", "stale-unverified"}}, refuseStaleBackup); err != nil {
			warning = fmt.Sprintf("marked %s stale-unverified in the store, but not in its markdown backup: %v", identity, err)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return warning, nil
}
