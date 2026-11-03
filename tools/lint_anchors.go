package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

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

// anchorGitTimeout bounds each git subprocess run by the anchor checker.
const anchorGitTimeout = 5 * time.Second

// anchorChecker answers the git questions the anchor checks need.
type anchorChecker interface {
	Head() (string, error)
	CommitExists(sha string) (bool, error)
	PathExists(sha, path string) (bool, error)
	PathChanged(fromSHA, toSHA, path string) (bool, error)
}

// gitAnchorChecker implements anchorChecker with `git -C dir`.
type gitAnchorChecker struct {
	dir string
}

// Head returns the full HEAD commit of the repository.
func (g gitAnchorChecker) Head() (string, error) {
	return gitOutput(g.dir, "rev-parse", "HEAD")
}

// CommitExists reports whether sha names a commit in the repository.
func (g gitAnchorChecker) CommitExists(sha string) (bool, error) {
	code, err := g.exitCode("cat-file", "-e", sha+"^{commit}")
	return code == 0, err
}

// PathExists reports whether path exists in the tree of commit sha.
func (g gitAnchorChecker) PathExists(sha, path string) (bool, error) {
	code, err := g.exitCode("cat-file", "-e", sha+":"+path)
	return code == 0, err
}

// PathChanged reports whether path differs between two commits (deletion
// counts as a change). `git diff --quiet` exits 1 on a difference.
func (g gitAnchorChecker) PathChanged(fromSHA, toSHA, path string) (bool, error) {
	code, err := g.exitCode("diff", "--quiet", fromSHA, toSHA, "--", path)
	if err != nil {
		return false, err
	}
	if code > 1 {
		return false, fmt.Errorf("git diff %s..%s -- %s exited %d", fromSHA, toSHA, path, code)
	}
	return code == 1, nil
}

// exitCode runs git and returns its exit code. An error is returned only
// when git could not be run (not found, timeout), never for a non-zero exit.
func (g gitAnchorChecker) exitCode(args ...string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), anchorGitTimeout)
	defer cancel()
	err := exec.CommandContext(ctx, "git", append([]string{"-C", g.dir}, args...)...).Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && ctx.Err() == nil {
		return exitErr.ExitCode(), nil
	}
	if err != nil {
		return -1, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return 0, nil
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

// learningAnchors returns the pin commit and anchor paths of a learning
// eligible for anchor checks: it has an as_of pin, at least one anchor for
// the pinned repository, and a validity other than superseded or
// historical (historical knowledge is immune to code evolution).
func learningAnchors(p *store.Page) (string, []string, bool) {
	var props map[string]any
	if err := json.Unmarshal([]byte(p.Properties), &props); err != nil {
		return "", nil, false
	}
	if v, _ := props["validity"].(string); v == "superseded" || v == "historical" {
		return "", nil, false
	}
	asOf, _ := props["as_of"].(string)
	repo, sha, ok := splitAsOf(asOf)
	if !ok {
		return "", nil, false
	}
	evidence, _ := props["evidence"].([]any)
	var paths []string
	for _, e := range evidence {
		locator, _ := e.(string)
		if path := parseAnchor(locator, repo); path != "" {
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

// checkAnchors reports anchor_missing and anchor_drift findings for every
// eligible learning. Skipped (no findings, no error) when there is no
// checker or the vault is not a git repository.
func (l *Lint) checkAnchors() ([]Finding, error) {
	if l.anchors == nil {
		return nil, nil
	}
	head, err := l.anchors.Head()
	if err != nil {
		lintLogger.Debug("anchor checks skipped: vault is not a git repository", "err", err)
		return nil, nil
	}
	pages, err := l.store.ListLearningPages()
	if err != nil {
		return nil, fmt.Errorf("list learning pages: %w", err)
	}
	var findings []Finding
	for _, p := range pages {
		sha, paths, ok := learningAnchors(p)
		if !ok {
			continue
		}
		pageFindings, err := checkLearningAnchors(l.anchors, p, sha, head, paths)
		if err != nil {
			return nil, err
		}
		findings = append(findings, pageFindings...)
	}
	return findings, nil
}

// checkLearningAnchors checks one learning's anchors against its pin and
// HEAD. An unknown pin yields a single anchor_missing finding.
func checkLearningAnchors(c anchorChecker, p *store.Page, sha, head string, paths []string) ([]Finding, error) {
	known, err := c.CommitExists(sha)
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
		present, err := c.PathExists(sha, path)
		if err != nil {
			return nil, err
		}
		if !present {
			findings = append(findings, anchorFinding("anchor_missing", p,
				fmt.Sprintf("Learning '%s' cites %s, which does not exist at %s.", strings.TrimPrefix(p.Name, store.LearningPrefix), path, shortSHA(sha)),
				"Correct the evidence locator in an updated learning that supersedes this one."))
			continue
		}
		changed, err := c.PathChanged(sha, head, path)
		if err != nil {
			return nil, err
		}
		if changed {
			findings = append(findings, anchorFinding("anchor_drift", p,
				fmt.Sprintf("Learning '%s' cites %s, which changed between %s and HEAD %s.", strings.TrimPrefix(p.Name, store.LearningPrefix), path, shortSHA(sha), shortSHA(head)),
				fmt.Sprintf("Re-verify the learning against the current code; store an updated learning with supersedes: %s, or run lint with fix to mark it stale-unverified.", strings.TrimPrefix(p.Name, store.LearningPrefix))))
		}
	}
	return findings, nil
}

// fixAnchorDrift marks every learning with an anchor_drift finding as
// validity: stale-unverified. Returns the number of learnings marked.
func (l *Lint) fixAnchorDrift(findings []Finding) int {
	marked := 0
	seen := make(map[string]bool)
	for _, f := range findings {
		if f.Type != "anchor_drift" || seen[f.Page] {
			continue
		}
		seen[f.Page] = true
		if err := l.markStaleUnverified(f.Page); err != nil {
			lintLogger.Warn("failed to mark learning stale-unverified", "page", f.Page, "err", err)
			continue
		}
		marked++
	}
	return marked
}

// markStaleUnverified sets validity: stale-unverified on a learning in the
// store and in its markdown backup when one exists. Tier and content are
// untouched.
func (l *Lint) markStaleUnverified(pageName string) error {
	page, err := l.store.GetPage(pageName)
	if err != nil {
		return err
	}
	if page == nil {
		return fmt.Errorf("page not found: %s", pageName)
	}
	props := map[string]any{}
	_ = json.Unmarshal([]byte(page.Properties), &props)
	props["validity"] = "stale-unverified"
	data, _ := json.Marshal(props) // a map of JSON-decoded values always marshals
	page.Properties = string(data)
	if err := l.store.UpdatePage(page); err != nil {
		return err
	}
	if l.vaultPath == "" {
		return nil
	}
	path := filepath.Join(l.vaultPath, deweyWorkspaceDir, "learnings", strings.TrimPrefix(pageName, store.LearningPrefix)+".md")
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	updated, err := setFrontmatterKeys(string(content), [][2]string{{"validity", "stale-unverified"}})
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(updated), 0o644)
}
