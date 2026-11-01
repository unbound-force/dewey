package artifacts

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

// choreBranchPrefix is the branch namespace for dewey artifact publish chores.
const choreBranchPrefix = "chore/dewey-artifacts-"

// createSyncWorktree creates the sync worktree: an OS-tempdir directory
// registered as a detached git worktree at baseSHA, carrying a new
// chore/dewey-artifacts-YYYYMMDD branch (design D7 steps 2-3). The tempdir
// placement is structural — outside the vault and every configured source
// path, so the live server watcher never observes sync activity.
//
// On failure the returned error names the worktree path when one was created:
// recovery is removing the directory and running `git worktree prune` from the
// repository (design deliberately leaves failed worktrees in place rather than
// destroying evidence).
func createSyncWorktree(repoRoot, baseSHA string, now time.Time) (string, string, error) {
	dir, err := os.MkdirTemp("", "dewey-artifacts-")
	if err != nil {
		return "", "", fmt.Errorf("create worktree temp dir: %w", err)
	}
	resolved, err := resolveRepoRoot(repoRoot)
	if err != nil {
		return dir, "", fmt.Errorf("resolve repository root: %w", err)
	}
	if withinDir(dir, resolved) {
		return dir, "", fmt.Errorf("worktree path %s is inside repository %s", dir, resolved)
	}
	if _, err := run(repoRoot, "worktree", "add", "--detach", dir, baseSHA); err != nil {
		return dir, "", fmt.Errorf("git worktree add at %s: %w (worktree dir left at %s; remove it and run `git worktree prune`)", baseSHA, err, dir)
	}
	branch, err := nextChoreBranch(repoRoot, now)
	if err != nil {
		return dir, "", fmt.Errorf("choose chore branch: %w (worktree dir left at %s; remove it and run `git worktree prune`)", err, dir)
	}
	if _, err := run(dir, "checkout", "-b", branch); err != nil {
		return dir, "", fmt.Errorf("git checkout -b %s: %w (worktree dir left at %s; remove it and run `git worktree prune`)", branch, err, dir)
	}
	return dir, branch, nil
}

// nextChoreBranch derives the chore branch name for the sync-time date
// (chore/dewey-artifacts-YYYYMMDD, UTC) and appends the first free numeric
// suffix (-2, -3, …) when earlier syncs already created branches that day.
func nextChoreBranch(repoRoot string, now time.Time) (string, error) {
	base := choreBranchPrefix + now.UTC().Format("20060102")
	for i := 1; ; i++ {
		name := base
		if i > 1 {
			name = fmt.Sprintf("%s-%d", base, i)
		}
		if !branchExists(repoRoot, name) {
			return name, nil
		}
	}
}

// branchExists reports whether a local branch is already present. Branch refs
// are shared between the main repository and its worktrees, so probing against
// repoRoot sees chore branches from earlier syncs.
func branchExists(repoRoot, name string) bool {
	_, err := run(repoRoot, "rev-parse", "--verify", "--quiet", "refs/heads/"+name)
	return err == nil
}

// sha256Hex returns the hex-encoded SHA-256 digest of b.
func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// loadPlan reads and parses the plan JSON file, returning the plan and the
// SHA-256 of the consumed file bytes (the plan_sha256 audit value bound into
// the chore commit).
func loadPlan(planPath string) (*Plan, string, error) {
	raw, err := os.ReadFile(planPath)
	if err != nil {
		return nil, "", fmt.Errorf("read plan %s: %w", planPath, err)
	}
	var plan Plan
	if err := json.Unmarshal(raw, &plan); err != nil {
		return nil, "", fmt.Errorf("parse plan: %w", err)
	}
	return &plan, sha256Hex(raw), nil
}

// planWrite is one planned file to materialize in the sync worktree.
type planWrite struct {
	path    string
	sha256  string
	sources []string
}

// plannedWrites derives the write set from the plan: every artifact plus every
// conflict resolved through the path-keyed resolutions map. A conflict without
// a resolution, or a resolution naming a sha that is not one of the conflict's
// variants, fails closed.
func plannedWrites(plan *Plan, resolutions map[string]string) ([]planWrite, error) {
	var writes []planWrite
	for _, art := range plan.Artifacts {
		writes = append(writes, planWrite{path: art.Path, sha256: art.SHA256, sources: art.Sources})
	}
	for _, conf := range plan.Conflicts {
		sha, ok := resolutions[conf.Path]
		if !ok {
			return nil, fmt.Errorf("conflict at %s lacks a resolution", conf.Path)
		}
		var sources []string
		for _, v := range conf.Variants {
			if v.SHA256 == sha {
				sources = v.Sources
			}
		}
		if sources == nil {
			return nil, fmt.Errorf("resolution for %s names sha256 %s, which is not a variant of the conflict", conf.Path, sha)
		}
		writes = append(writes, planWrite{path: conf.Path, sha256: sha, sources: sources})
	}
	sort.Slice(writes, func(i, j int) bool { return writes[i].path < writes[j].path })
	return writes, nil
}

// gitObjectContent fetches a blob from the object store: `git show sha:path`.
func gitObjectContent(repoRoot, sha, path string) ([]byte, error) {
	return run(repoRoot, "show", sha+":"+path)
}

// stashContent fetches a stash-sourced blob by its RECORDED immutable sha —
// never the mutable stash@{N} label — verifying first that the recorded sha
// still exists in the object store (a dropped or gc'd stash fails closed with
// a re-run-plan instruction). Untracked sources key the stash's ^3 tree, so
// their store check peels to a tree rather than a commit.
func stashContent(repoRoot, sha, path string, untracked bool) ([]byte, error) {
	peel := sha + "^{commit}"
	if untracked {
		peel = sha + "^{tree}"
	}
	if _, err := run(repoRoot, "cat-file", "-e", peel); err != nil {
		return nil, fmt.Errorf("recorded stash sha %s missing from object store — re-run plan", sha)
	}
	return gitObjectContent(repoRoot, sha, path)
}

// acquireContent produces the planned bytes for one write. Working-tree
// sources are re-validated as regular files and re-hashed (a symlink swapped
// in since plan time, or content drift, fails closed as a stale plan); stash
// sources are fetched by recorded sha; commit sources by their commit sha.
func acquireContent(repoRoot string, w planWrite, plan *Plan) ([]byte, error) {
	if containsString(w.sources, SourceWorkingTree) {
		src := filepath.Join(repoRoot, filepath.FromSlash(w.path))
		info, err := os.Lstat(src)
		if err != nil {
			return nil, fmt.Errorf("working-tree source %s: %w", w.path, err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("working-tree source %s is no longer a regular file", w.path)
		}
		content, err := os.ReadFile(src)
		if err != nil {
			return nil, fmt.Errorf("working-tree source %s: %w", w.path, err)
		}
		if got := sha256Hex(content); got != w.sha256 {
			return nil, fmt.Errorf("stale plan: %s changed since plan (sha256 mismatch)", w.path)
		}
		return content, nil
	}
	for _, source := range w.sources {
		if sha, untracked, ok := recordedStashSHA(source, plan); ok {
			return stashContent(repoRoot, sha, w.path, untracked)
		}
	}
	for _, source := range w.sources {
		if strings.HasPrefix(source, SourceCommitPrefix) {
			return gitObjectContent(repoRoot, strings.TrimPrefix(source, SourceCommitPrefix), w.path)
		}
	}
	return nil, fmt.Errorf("plan entry for %s has no usable source label", w.path)
}

// recordedStashSHA maps a stash source label to the recorded immutable sha.
// Tracked labels ("stash:stash@{N}") key the stash commit; untracked labels
// ("stash-untracked:stash@{N}") key the label's ^3 untracked tree. The
// untracked flag selects the object-store peel type.
func recordedStashSHA(source string, plan *Plan) (sha string, untracked, ok bool) {
	if strings.HasPrefix(source, SourceStashUntrackedPrefix) {
		label := strings.TrimPrefix(source, SourceStashUntrackedPrefix)
		sha, ok := plan.Repo.StashRefs[label+"^3"]
		return sha, true, ok
	}
	if strings.HasPrefix(source, SourceStashPrefix) {
		label := strings.TrimPrefix(source, SourceStashPrefix)
		sha, ok := plan.Repo.StashRefs[label]
		return sha, false, ok
	}
	return "", false, false
}

// scanDestinationSymlinks walks the planned path's ancestor components inside
// the checked-out worktree and fails closed naming the first symlink found: a
// write through a symlinked destination escapes the worktree.
func scanDestinationSymlinks(worktree, rel string) error {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	cur := worktree
	for _, part := range parts {
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if err != nil {
			if os.IsNotExist(err) {
				return nil // not yet created — remaining components are ours to mkdir
			}
			return fmt.Errorf("scan destination %s: %w", rel, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("planned destination %s traverses symlink %s", rel, cur)
		}
	}
	return nil
}

// materializePlan writes exactly the planned files into the sync worktree
// (design D7 step 4): symlink-scanned destinations, source-verified content,
// and a universal post-write sha256 re-verification. It returns the sorted
// repo-relative paths written and whether the gitignore fix was applied.
func materializePlan(repoRoot, worktree string, plan *Plan, resolutions map[string]string) ([]string, bool, error) {
	writes, err := plannedWrites(plan, resolutions)
	if err != nil {
		return nil, false, err
	}
	for _, w := range writes {
		if err := scanDestinationSymlinks(worktree, w.path); err != nil {
			return nil, false, err
		}
	}
	var filesWritten []string
	for _, w := range writes {
		content, err := acquireContent(repoRoot, w, plan)
		if err != nil {
			return nil, false, err
		}
		dest := filepath.Join(worktree, filepath.FromSlash(w.path))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return nil, false, fmt.Errorf("create destination dir for %s: %w", w.path, err)
		}
		if err := os.WriteFile(dest, content, 0o644); err != nil {
			return nil, false, fmt.Errorf("write %s: %w", w.path, err)
		}
		filesWritten = append(filesWritten, w.path)
	}
	for _, w := range writes {
		dest := filepath.Join(worktree, filepath.FromSlash(w.path))
		content, err := os.ReadFile(dest)
		if err != nil {
			return nil, false, fmt.Errorf("post-write verification for %s: %w", w.path, err)
		}
		if got := sha256Hex(content); got != w.sha256 {
			return nil, false, fmt.Errorf("post-write verification failed for %s", w.path)
		}
	}
	fixApplied, err := applyGitignoreFix(worktree, plan.Gitignore.Fix)
	if err != nil {
		return nil, false, err
	}
	sort.Strings(filesWritten)
	return filesWritten, fixApplied, nil
}

// applyGitignoreFix applies the flagged gitignore fix inside the worktree:
// replacing the exact bare anchor line with the anchored form and appending
// the runtime-state re-ignore lines. When the pattern's source file is absent
// from the worktree (external ignore source), the fix is skipped without
// error — the plan warning stands.
func applyGitignoreFix(worktree string, fix *GitignoreFix) (bool, error) {
	if fix == nil {
		return false, nil
	}
	if fix.File != ".gitignore" {
		return false, fmt.Errorf("gitignore fix names unsupported file %q", fix.File)
	}
	fixPath := filepath.Join(worktree, filepath.FromSlash(fix.File))
	raw, err := os.ReadFile(fixPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil // external ignore source — skip the fix, keep the warning
		}
		return false, fmt.Errorf("read %s: %w", fix.File, err)
	}
	lines := strings.Split(string(raw), "\n")
	existing := make(map[string]bool, len(lines))
	for i, line := range lines {
		if line == fix.Anchor.From {
			lines[i] = fix.Anchor.To
		}
		existing[line] = true
	}
	for _, extra := range fix.Reignore {
		if !existing[extra] {
			lines = append(lines, extra)
			existing[extra] = true
		}
	}
	if err := os.WriteFile(fixPath, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		return false, fmt.Errorf("write %s: %w", fix.File, err)
	}
	return true, nil
}
