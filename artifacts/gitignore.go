package artifacts

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// reignoreLines is the bounded re-ignore block carried by the gitignore fix:
// the non-publishable runtime state and private configuration that the bare
// `dewey` pattern was keeping out of git before the anchor fix, expressed
// explicitly so anchoring never silently un-ignores them post-merge. The
// order is the fixed documented order from the plan vocabulary, not sorted.
var reignoreLines = []string{
	".uf/dewey/graph.db",
	".uf/dewey/graph.db-shm",
	".uf/dewey/graph.db-wal",
	".uf/dewey/dewey.log",
	".uf/dewey/dewey.lock",
	".uf/dewey/cache/",
	".uf/dewey/config.yaml",
	".uf/dewey/sources.yaml",
	".uf/dewey/knowledge-stores.yaml",
}

// probeGitignore runs one synthetic check-ignore probe per sanctioned
// artifact root (the three defaults plus each configured store path) and
// classifies the repository's gitignore posture for the plan.
//
// Classification vocabulary:
//   - ok: no probe path is ignored.
//   - bare-dewey-fix: a probe is ignored by a pattern that is exactly the
//     bare `dewey` line in a tracked, in-repo .gitignore — the known Dewey
//     layout bug. The bounded fix block (anchor `dewey` → `/dewey` plus
//     the re-ignore lines) is carried in the finding for the sync to apply.
//   - ignore-source-external: a probe is ignored by a pattern whose source
//     file is outside the tracked worktree (.git/info/exclude or a
//     core.excludesFile global). The fix is skipped and a warning is
//     surfaced.
//   - other-ignore-warning: a probe is ignored by an in-repo pattern that
//     is not the bare `dewey` line. A warning is surfaced.
//
// Bare-dewey-fix takes precedence when several probes disagree; the sync
// applies the fix only when the check-ignore-identified source file is
// present in the worktree.
func probeGitignore(repoRoot string, storeRels []walkRoot) (GitignoreFinding, []string, error) {
	var warnings []string
	bareDewey := false
	external := false
	otherIgnore := false

	for _, root := range sanctionedRoots(storeRels) {
		probe := root + "probe.md"
		out, err := run(repoRoot, "check-ignore", "-v", "--", probe)
		if err != nil {
			var ge *gitError
			if errors.As(err, &ge) && ge.exitCode == 1 {
				// Not ignored: the root is publishable as-is.
				continue
			}
			return GitignoreFinding{Status: GitignoreStatusOK}, nil, fmt.Errorf("check ignore status of %s: %w", probe, err)
		}
		source, lineNo, pattern, ok := parseCheckIgnore(string(out))
		if !ok {
			return GitignoreFinding{Status: GitignoreStatusOK}, nil, fmt.Errorf("parse check-ignore output for %s: malformed output %q", probe, string(out))
		}
		switch {
		case pattern == "dewey" && sourceIsInRepoGitignore(source):
			bareDewey = true
		case !sourceIsInRepoGitignore(source):
			external = true
			warnings = append(warnings, fmt.Sprintf(
				"gitignore: pattern %q in %s:%s ignores %s (source outside the tracked worktree — fix skipped)",
				pattern, source, lineNo, probe))
		default:
			otherIgnore = true
			warnings = append(warnings, fmt.Sprintf(
				"gitignore: pattern %q in %s:%s ignores %s",
				pattern, source, lineNo, probe))
		}
	}

	switch {
	case bareDewey:
		return GitignoreFinding{
			Status: GitignoreStatusBareDeweyFix,
			Fix: &GitignoreFix{
				File:     ".gitignore",
				Anchor:   GitignoreAnchor{From: "dewey", To: "/dewey"},
				Reignore: reignoreLines,
			},
		}, warnings, nil
	case external:
		return GitignoreFinding{Status: GitignoreStatusIgnoreSourceExternal}, warnings, nil
	case otherIgnore:
		return GitignoreFinding{Status: GitignoreStatusOtherIgnoreWarning}, warnings, nil
	default:
		return GitignoreFinding{Status: GitignoreStatusOK}, warnings, nil
	}
}

// parseCheckIgnore parses one line of `git check-ignore -v` output:
// `<source>:<linenum>:<pattern>\t<path>`. The metadata is split from the
// right so a source path containing colons still parses.
func parseCheckIgnore(out string) (source, lineNo, pattern string, ok bool) {
	out = strings.TrimSpace(out)
	tab := strings.IndexByte(out, '\t')
	if tab < 0 {
		return "", "", "", false
	}
	meta := out[:tab]
	last := strings.LastIndexByte(meta, ':')
	if last < 0 {
		return "", "", "", false
	}
	pattern = meta[last+1:]
	rest := meta[:last]
	second := strings.LastIndexByte(rest, ':')
	if second < 0 {
		return "", "", "", false
	}
	lineNo = rest[second+1:]
	source = rest[:second]
	if source == "" || lineNo == "" || pattern == "" {
		return "", "", "", false
	}
	return source, lineNo, pattern, true
}

// sourceIsInRepoGitignore reports whether a check-ignore source names a
// tracked, in-repo .gitignore file — the only source the bounded fix block
// can rewrite. `.git/info/exclude` lives inside the repository directory
// but is not a tracked worktree file, and core.excludesFile sources are
// absolute paths; both are external to the fix.
func sourceIsInRepoGitignore(source string) bool {
	if source == "" || filepath.IsAbs(source) {
		return false
	}
	if strings.HasPrefix(source, ".git/") {
		return false
	}
	if source == ".gitignore" {
		return true
	}
	return strings.HasSuffix(source, "/.gitignore")
}
