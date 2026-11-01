package artifacts

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// enumerateCommits sweeps every unpushed commit — commits reachable from any
// local branch or a detached HEAD but from no remote-tracking ref
// (`git rev-list --branches HEAD --not --remotes`, newest first) — for
// knowledge artifacts. Stash refs are excluded because stashes are enumerated
// by their own mechanism; tag-reachable unpushed content is out of scope.
// Every git invocation here is read-only.
//
// For each path the content is taken from the NEWEST touching commit: the
// rev-list order is newest-first, so the first tree scan that decides a path
// wins, for candidates and skip warnings alike. A path whose newest touching
// state is a symbolic link stays skipped even when older commits carry it as
// a regular file.
//
// repoRoot is the repository root (as resolved by resolveRepoRoot). storePaths
// are knowledge-store paths resolved by the CLI layer, validated with the
// same rules as the working-tree walk.
//
// It returns the artifacts found (sorted by path, then by source; in practice
// each path appears exactly once) and the surfaced warnings.
func enumerateCommits(repoRoot string, storePaths []string) ([]candidate, []string, error) {
	resolvedRepo, err := filepath.EvalSymlinks(repoRoot)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve repository root %s: %w", repoRoot, err)
	}
	var storeRels []walkRoot
	for _, sp := range storePaths {
		wr, err := validateStorePath(repoRoot, resolvedRepo, sp)
		if err != nil {
			return nil, nil, err
		}
		storeRels = append(storeRels, wr)
	}
	sort.Slice(storeRels, func(i, j int) bool { return storeRels[i].rel < storeRels[j].rel })

	out, err := run(repoRoot, "rev-list", "--branches", "HEAD", "--not", "--remotes")
	if err != nil {
		return nil, nil, fmt.Errorf("list unpushed commits: %w", err)
	}

	var candidates []candidate
	var warnings []string
	decided := make(map[string]bool)
	for _, line := range strings.Split(string(out), "\n") {
		sha := strings.TrimSpace(line)
		if sha == "" {
			continue
		}
		if !isHex40(sha) {
			return nil, nil, fmt.Errorf("list unpushed commits: non-hex sha %q", sha)
		}
		records, err := scanTreeRef(repoRoot, sha, SourceCommitPrefix+sha, storeRels)
		if err != nil {
			return nil, nil, err
		}
		for _, r := range records {
			if decided[r.path] {
				continue
			}
			decided[r.path] = true
			if r.cand != nil {
				candidates = append(candidates, *r.cand)
			} else {
				warnings = append(warnings, r.warning)
			}
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Path != candidates[j].Path {
			return candidates[i].Path < candidates[j].Path
		}
		return candidates[i].Source < candidates[j].Source
	})
	return candidates, warnings, nil
}
