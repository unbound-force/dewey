package artifacts

import (
	"fmt"
	"os"
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
