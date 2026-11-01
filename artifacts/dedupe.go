package artifacts

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"sort"
	"strings"
)

// resolveRepoForPlan resolves the deterministic repository facts a plan needs:
// the publish and push remotes, the base ref and its sha, and the sorted
// sanctioned artifact roots. It is strictly offline — every ref it consults is
// a local remote-tracking ref; refreshing them (fetch, set-head) belongs to
// the CLI layer that runs before the plan core.
//
// Publish-remote resolution: `upstream` when that remote exists (base-tree
// source and PR target — the fork layout this repository itself uses), else
// `origin`. The push remote is `origin` when the publish remote is `upstream`
// (contributors typically lack upstream push rights; the PR carries the fork
// head), else the publish remote. When `upstream` is the resolved publish
// remote, an `origin` remote MUST also exist (it is the push remote in fork
// layouts); its absence fails closed with an actionable error naming both
// remotes.
//
// Base-ref resolution chain: `<publish-remote>/HEAD` (symbolic ref) →
// `<publish-remote>/main` → `<publish-remote>/master`, each verified with
// rev-parse; when all are absent it fails closed with a fetch-directed error
// naming the resolved remote.
//
// It also builds the base tree: the content sha256 of every sanctioned
// artifact path in the base ref's tree, via `git ls-tree -z` to list paths and
// `git show <base>:<path>` for content. Records whose type is not a blob or
// whose mode is 120000 (symbolic link), and dot-prefixed or unsafe basenames,
// are skipped silently — the base tree is a reference lookup for status
// comparison, not stranded content, so there is nothing to warn about. (A
// `git cat-file --batch` batched read could replace the per-path `git show`
// loop; adopt only if profiling ever justifies it.)
//
// storeRels are the already-validated knowledge-store walk roots. The
// returned Repo.StashRefs is left nil — the plan assembler fills it from the
// stash enumeration.
func resolveRepoForPlan(repoRoot string, storeRels []walkRoot) (Repo, map[string]string, error) {
	remotes, err := remoteNames(repoRoot)
	if err != nil {
		return Repo{}, nil, err
	}
	if err := requirePublishRemote(remotes); err != nil {
		return Repo{}, nil, err
	}

	publishRemote := remoteOrigin
	if containsString(remotes, remoteUpstream) {
		publishRemote = remoteUpstream
	}
	pushRemote := publishRemote
	if publishRemote == remoteUpstream {
		pushRemote = remoteOrigin
		if !containsString(remotes, remoteOrigin) {
			return Repo{}, nil, fmt.Errorf(
				"resolve publish remote: upstream is the publish remote but no origin push remote exists — fork layouts require both (configured remotes: %s)",
				strings.Join(remotes, ", "))
		}
	}

	baseRef, baseBranch, err := resolveBaseRef(repoRoot, publishRemote)
	if err != nil {
		return Repo{}, nil, err
	}
	baseSHA, present, err := probeRef(repoRoot, baseRef)
	if err != nil {
		return Repo{}, nil, err
	}
	if !present || !isHex40(baseSHA) {
		return Repo{}, nil, fmt.Errorf("resolve base ref %s: invalid sha %q", baseRef, baseSHA)
	}

	baseTree, err := scanBaseTree(repoRoot, baseRef, storeRels)
	if err != nil {
		return Repo{}, nil, err
	}

	return Repo{
		BaseBranch:    baseBranch,
		BaseSHA:       baseSHA,
		PublishRemote: publishRemote,
		PushRemote:    pushRemote,
		ArtifactRoots: sanctionedRoots(storeRels),
		StashRefs:     nil,
	}, baseTree, nil
}

// resolveBaseRef walks the deterministic base-ref chain for the publish
// remote: the remote HEAD symbolic ref, then main, then master. It returns
// the resolved ref (e.g. "origin/main") and the bare branch name ("main").
func resolveBaseRef(repoRoot, publishRemote string) (ref, branch string, err error) {
	out, symErr := run(repoRoot, "symbolic-ref", "--short", "refs/remotes/"+publishRemote+"/HEAD")
	if symErr == nil {
		short := strings.TrimSpace(string(out))
		if short != "" {
			// symbolic-ref --short prints the full short ref (e.g.
			// "origin/main"), remote prefix included — do not re-prefix.
			return short, strings.TrimPrefix(short, publishRemote+"/"), nil
		}
	}
	// A missing or unset remote HEAD is expected on minimal clones — fall
	// through to the branch-name chain.
	for _, branch := range []string{"main", "master"} {
		ref := publishRemote + "/" + branch
		if _, present, err := probeRef(repoRoot, ref); err == nil && present {
			return ref, branch, nil
		}
	}
	return "", "", fmt.Errorf(
		"resolve base ref for remote %q: no remote default branch, main, or master — run `git fetch %s`",
		publishRemote, publishRemote)
}

// scanBaseTree lists the sanctioned artifact paths in the base ref's tree and
// hashes each blob's content with sha256. Skip rules match the enumeration
// sources (non-blob, symlink, dot-prefixed, unsafe basename), but silently —
// the base tree is reference data.
func scanBaseTree(repoRoot, baseRef string, storeRels []walkRoot) (map[string]string, error) {
	out, err := run(repoRoot, "ls-tree", "-r", "-z", baseRef)
	if err != nil {
		return nil, fmt.Errorf("list base tree of %s: %w", baseRef, err)
	}
	baseTree := make(map[string]string)
	for _, record := range strings.Split(string(out), "\x00") {
		if record == "" {
			continue
		}
		tab := strings.IndexByte(record, '\t')
		if tab < 0 {
			return nil, fmt.Errorf("parse ls-tree record of %s: malformed record %q", baseRef, record)
		}
		meta := strings.Fields(record[:tab])
		if len(meta) != 3 {
			return nil, fmt.Errorf("parse ls-tree record of %s: malformed metadata %q", baseRef, record[:tab])
		}
		mode, typ := meta[0], meta[1]
		p := record[tab+1:]

		if _, ok := classifyTreePath(p, storeRels); !ok {
			continue
		}
		if typ != "blob" || mode == treeModeSymlink {
			continue
		}
		base := path.Base(p)
		if strings.HasPrefix(base, ".") {
			continue
		}
		if _, unsafe := unsafeBaseName(base); unsafe {
			continue
		}
		content, err := run(repoRoot, "show", baseRef+":"+p)
		if err != nil {
			return nil, fmt.Errorf("read %s from %s: %w", p, baseRef, err)
		}
		sum := sha256.Sum256(content)
		baseTree[p] = hex.EncodeToString(sum[:])
	}
	return baseTree, nil
}

// sanctionedRoots returns the sorted sanctioned artifact roots in
// trailing-slash form: the three defaults plus every configured store path.
func sanctionedRoots(storeRels []walkRoot) []string {
	roots := []string{
		".uf/dewey/" + dirCompiled + "/",
		".uf/dewey/" + dirKnowledge + "/",
		".uf/dewey/" + dirLearnings + "/",
	}
	for _, sr := range storeRels {
		roots = append(roots, strings.TrimSuffix(sr.rel, "/")+"/")
	}
	sort.Strings(roots)
	return roots
}

// pathGroup is every candidate collected for one repository-relative path —
// the dedup, collision, conflict, and resolution key.
type pathGroup struct {
	path  string
	cands []candidate
}

// groupCandidatesByPath groups candidates by repository-relative path and
// sorts groups by path and candidates within each group by source, so
// downstream stages see deterministic order. Duplicate (path, source) pairs
// cannot occur (each enumerator yields one candidate per path and source;
// the commit enumerator already deduplicates internally), but a first-wins
// guard keeps the invariant cheap.
func groupCandidatesByPath(candidates []candidate) []pathGroup {
	byPath := make(map[string][]candidate)
	var paths []string
	for _, c := range candidates {
		if _, seen := byPath[c.Path]; !seen {
			paths = append(paths, c.Path)
		}
		dup := false
		for _, existing := range byPath[c.Path] {
			if existing.Source == c.Source {
				dup = true
				break
			}
		}
		if !dup {
			byPath[c.Path] = append(byPath[c.Path], c)
		}
	}
	sort.Strings(paths)
	groups := make([]pathGroup, 0, len(paths))
	for _, p := range paths {
		cands := byPath[p]
		sort.Slice(cands, func(i, j int) bool { return cands[i].Source < cands[j].Source })
		groups = append(groups, pathGroup{path: p, cands: cands})
	}
	return groups
}

// buildArtifactEntry turns a single-content path group into its plan entry.
// Every source in the group carries the same sha256, so the entry holds all
// of them as provenance. Status is `unchanged` when the base tree already
// holds the same content sha256 at the same path (observability — the sync
// stage excludes it from the publish set) and `new` otherwise; a path whose
// content differs from the base tree is a plain new entry, never a collision.
// When the group carries more than one distinct sha256, the entry is a
// cross-source conflict: isConflict is true and the returned Artifact is
// zero — conflict entries are built by the collision stage.
func buildArtifactEntry(group pathGroup, baseTree map[string]string) (Artifact, bool) {
	if len(group.cands) == 0 {
		return Artifact{}, true
	}
	first := group.cands[0]
	for _, c := range group.cands[1:] {
		if c.SHA256 != first.SHA256 {
			return Artifact{}, true
		}
	}
	sources := make([]string, 0, len(group.cands))
	for _, c := range group.cands {
		sources = append(sources, c.Source)
	}
	status := StatusNew
	if baseSHA, ok := baseTree[group.path]; ok && baseSHA == first.SHA256 {
		status = StatusUnchanged
	}
	return Artifact{
		Path:     group.path,
		Identity: first.Identity,
		Tag:      first.Tag,
		Kind:     first.Kind,
		SHA256:   first.SHA256,
		Sources:  sources,
		Status:   status,
		Findings: []Finding{},
	}, false
}

// containsString reports whether s is present in list.
func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
