package artifacts

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// trioKinds maps the default artifact directory names to their artifact kinds.
var trioKinds = map[string]string{
	dirLearnings: KindLearning,
	dirCompiled:  KindCompiled,
	dirKnowledge: KindKnowledge,
}

// treeModeSymlink is the git tree-entry mode for symbolic links.
const treeModeSymlink = "120000"

// enumerateStashes sweeps every stash on the stash stack — tracked parts and
// the untracked parts captured by `git stash -u` — for knowledge artifacts,
// without mutating the stash stack in any way (no pop, apply, or drop; every
// git invocation here is read-only).
//
// repoRoot is the repository root (as resolved by resolveRepoRoot). storePaths
// are repository-relative-or-absolute knowledge-store paths resolved by the
// CLI layer; each is validated with the same rules as the working-tree walk.
//
// It returns the artifacts found (sorted by path, then by source), the
// stashRefs map (every enumerated stash label — `stash@{N}` and, when the
// stash has an untracked part, `stash@{N}^3` — to its resolved 40-hex sha,
// because labels renumber as the stack changes while shas are stable), and
// the surfaced warnings.
func enumerateStashes(repoRoot string, storePaths []string) ([]candidate, map[string]string, []string, error) {
	resolvedRepo, err := filepath.EvalSymlinks(repoRoot)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("resolve repository root %s: %w", repoRoot, err)
	}
	var storeRels []walkRoot
	for _, sp := range storePaths {
		wr, err := validateStorePath(repoRoot, resolvedRepo, sp)
		if err != nil {
			return nil, nil, nil, err
		}
		storeRels = append(storeRels, wr)
	}
	sort.Slice(storeRels, func(i, j int) bool { return storeRels[i].rel < storeRels[j].rel })

	out, err := run(repoRoot, "stash", "list", "--format=%gd")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("list stashes: %w", err)
	}

	var candidates []candidate
	var warnings []string
	stashRefs := make(map[string]string)
	for _, line := range strings.Split(string(out), "\n") {
		label := strings.TrimSpace(line)
		if label == "" {
			continue
		}

		sha, present, err := probeRef(repoRoot, label)
		if err != nil {
			return nil, nil, nil, err
		}
		if !present {
			return nil, nil, nil, fmt.Errorf("stash ref %q vanished between listing and resolution", label)
		}
		stashRefs[label] = sha

		cs, ws, err := enumerateTreeRef(repoRoot, label, SourceStashPrefix+label, storeRels)
		if err != nil {
			return nil, nil, nil, err
		}
		candidates = append(candidates, cs...)
		warnings = append(warnings, ws...)

		untrackedRef := label + "^3"
		sha3, present, err := probeRef(repoRoot, untrackedRef)
		if err != nil {
			return nil, nil, nil, err
		}
		if present {
			stashRefs[untrackedRef] = sha3
			cs, ws, err := enumerateTreeRef(repoRoot, untrackedRef, SourceStashUntrackedPrefix+label, storeRels)
			if err != nil {
				return nil, nil, nil, err
			}
			candidates = append(candidates, cs...)
			warnings = append(warnings, ws...)
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Path != candidates[j].Path {
			return candidates[i].Path < candidates[j].Path
		}
		return candidates[i].Source < candidates[j].Source
	})
	return candidates, stashRefs, warnings, nil
}

// probeRef resolves ref to its 40-hex sha via `git rev-parse --verify
// --quiet`. The documented expected-absence contract of the git runner makes
// exit code 1 mean "ref does not exist", reported as present=false with a nil
// error; any other failure fails closed.
func probeRef(repoRoot, ref string) (sha string, present bool, err error) {
	out, err := run(repoRoot, "rev-parse", "--verify", "--quiet", ref)
	if err != nil {
		var ge *gitError
		if errors.As(err, &ge) && ge.exitCode == 1 {
			return "", false, nil
		}
		return "", false, fmt.Errorf("resolve ref %s: %w", ref, err)
	}
	sha = strings.TrimSpace(string(out))
	if sha == "" {
		return "", false, fmt.Errorf("resolve ref %s: git printed no sha", ref)
	}
	if !isHex40(sha) {
		return "", false, fmt.Errorf("resolve ref %s: non-hex sha %q", ref, sha)
	}
	return sha, true, nil
}

// isHex40 reports whether s is a 40-character lowercase hex string.
func isHex40(s string) bool {
	if len(s) != 40 {
		return false
	}
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}

// enumerateTreeRef lists the tree of a git ref (`git ls-tree -r -z`, full
// records so entry mode and type survive parsing), keeps only paths inside
// the sanctioned artifact roots, and reads each kept artifact's content
// byte-exactly via `git show <ref>:<path>`.
//
// Skip rules mirror the working-tree walk: dot-prefixed basenames are
// skipped silently; non-blob entries and symlinks (tree mode 120000) are
// skipped with warnings and their targets are never read; basenames carrying
// control characters, colons, or a leading hyphen are skipped with warnings.
// Out-of-scope paths (not under a sanctioned root) are ignored without
// warnings — the sweep is not interested in the rest of the tree.
func enumerateTreeRef(repoRoot, ref, source string, storeRels []walkRoot) ([]candidate, []string, error) {
	out, err := run(repoRoot, "ls-tree", "-r", "-z", ref)
	if err != nil {
		return nil, nil, fmt.Errorf("list tree of %s: %w", ref, err)
	}

	var candidates []candidate
	var warnings []string
	for _, record := range strings.Split(string(out), "\x00") {
		if record == "" {
			continue
		}
		tab := strings.IndexByte(record, '\t')
		if tab < 0 {
			return nil, nil, fmt.Errorf("parse ls-tree record of %s: malformed record %q", ref, record)
		}
		meta := strings.Fields(record[:tab])
		if len(meta) != 3 {
			return nil, nil, fmt.Errorf("parse ls-tree record of %s: malformed metadata %q", ref, record[:tab])
		}
		mode, typ := meta[0], meta[1]
		p := record[tab+1:]

		wr, ok := classifyTreePath(p, storeRels)
		if !ok {
			continue
		}
		base := path.Base(p)
		if strings.HasPrefix(base, ".") {
			continue
		}
		if typ != "blob" {
			warnings = append(warnings, fmt.Sprintf("skipped non-blob: %s (in %s)", p, ref))
			continue
		}
		if mode == treeModeSymlink {
			warnings = append(warnings, fmt.Sprintf("skipped symlink: %s (in %s)", p, ref))
			continue
		}
		if reason, unsafe := unsafeBaseName(base); unsafe {
			warnings = append(warnings, fmt.Sprintf("skipped unsafe basename: %s (%s)", p, reason))
			continue
		}

		content, err := run(repoRoot, "show", ref+":"+p)
		if err != nil {
			return nil, nil, fmt.Errorf("read %s from %s: %w", p, ref, err)
		}
		sum := sha256.Sum256(content)
		c := candidate{
			Path:    p,
			SHA256:  hex.EncodeToString(sum[:]),
			Source:  source,
			Content: content,
			Kind:    wr.kind,
		}
		applyIdentity(&c, base, p, wr)
		candidates = append(candidates, c)
	}
	return candidates, warnings, nil
}

// classifyTreePath classifies a repository-relative tree path against the
// sanctioned artifact roots: the default trio matched at ANY depth (a
// `.uf/dewey/{learnings,compiled,knowledge}/` component sequence anywhere in
// the path) or one of the configured store-path prefixes. The artifact must
// live strictly UNDER the root — a file sitting exactly at the root position
// does not match. The returned walkRoot carries the kind and the identity
// context; ok is false for out-of-scope paths.
func classifyTreePath(rel string, storeRels []walkRoot) (walkRoot, bool) {
	comps := strings.Split(strings.Trim(rel, "/"), "/")
	for i := 0; i+3 < len(comps); i++ {
		if comps[i] == dotUFRoot && comps[i+1] == deweyDir {
			if kind, ok := trioKinds[comps[i+2]]; ok {
				return walkRoot{rel: path.Join(comps[:i+3]...), kind: kind}, true
			}
		}
	}
	for _, sr := range storeRels {
		if strings.HasPrefix(rel, sr.rel+"/") {
			return walkRoot{
				rel:         sr.rel,
				kind:        KindKnowledge,
				customStore: true,
				storeTag:    path.Base(sr.rel),
			}, true
		}
	}
	return walkRoot{}, false
}
