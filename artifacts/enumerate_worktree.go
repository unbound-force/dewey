package artifacts

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// The three default artifact directories Dewey writes under `.uf/dewey/`.
const (
	dirLearnings = "learnings"
	dirCompiled  = "compiled"
	dirKnowledge = "knowledge"
	dotUFRoot    = ".uf"
	deweyDir     = "dewey"
)

// walkRoot is one sanctioned directory scanned for working-tree artifacts.
type walkRoot struct {
	// root is the lexical absolute path of the directory to walk.
	root string
	// rel is the repository-relative slash path of root (for example
	// ".uf/dewey/learnings" or a configured store path).
	rel string
	// kind is the artifact kind assigned to every file found under root.
	kind string
	// customStore marks knowledge roots configured directly (their items'
	// identity is relative to the store root and the tag is the store name).
	customStore bool
	// storeTag is the tag for items under a custom store root (its basename).
	storeTag string
}

// enumerateWorkingTree walks the sanctioned artifact directories in the
// working tree — the default trio resolved under BOTH the vault root and the
// repository root (deduplicated by repository-relative path) plus the
// configured knowledge-store paths — and returns one candidate per
// discovered artifact file, regardless of git tracked/ignored/staged state.
//
// The vault root must resolve inside the repository (outside fails closed
// naming both paths). Every component of each walk root below the
// repository root is Lstat-checked: a symbolic-link component fails closed
// naming the root and the offending component, because lexical containment
// is not physical containment. A store path that resolves outside the
// repository, or carries control characters, a colon, or a leading-hyphen
// component, fails closed with an actionable error.
//
// Dot-prefixed files are skipped. Only regular files are collected;
// symbolic links and other non-regular types are skipped with surfaced
// warnings and their targets are never read or hashed. Basenames carrying
// control characters, a colon, or a leading hyphen are skipped with
// warnings. Missing directories are empty. Content is hashed with sha256,
// and a read error during the walk or hashing (EACCES, EIO) fails closed
// with a wrapped error naming the path — never silently skipped.
func enumerateWorkingTree(repoRoot, vaultRoot string, storePaths []string) ([]candidate, []string, error) {
	repoRoot = filepath.Clean(repoRoot)
	vaultRoot = filepath.Clean(vaultRoot)

	resolvedRepo, err := filepath.EvalSymlinks(repoRoot)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve repository root %s: %w", repoRoot, err)
	}
	resolvedVault, err := filepath.EvalSymlinks(vaultRoot)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve vault root %s: %w", vaultRoot, err)
	}
	if !withinDir(resolvedVault, resolvedRepo) {
		return nil, nil, fmt.Errorf(
			"vault root %s (resolved %s) is outside the git repository %s (resolved %s)",
			vaultRoot, resolvedVault, repoRoot, resolvedRepo)
	}
	vaultRel := relWithin(resolvedVault, resolvedRepo)

	var warnings []string
	roots, err := workingTreeWalkRoots(repoRoot, resolvedRepo, vaultRoot, vaultRel, storePaths)
	if err != nil {
		return nil, warnings, err
	}

	byPath := make(map[string]struct{})
	var candidates []candidate
	for _, wr := range roots {
		if err := verifyWalkRootComponents(repoRoot, wr.rel); err != nil {
			return nil, warnings, fmt.Errorf("walk root %s: %w", wr.root, err)
		}
		info, err := os.Lstat(wr.root)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue // missing directories are empty
			}
			return nil, warnings, fmt.Errorf("inspect artifact root %s: %w", wr.root, err)
		}
		if !info.IsDir() {
			return nil, warnings, fmt.Errorf("artifact root %s is not a directory", wr.root)
		}
		resolvedRoot, err := filepath.EvalSymlinks(wr.root)
		if err != nil {
			return nil, warnings, fmt.Errorf("resolve artifact root %s: %w", wr.root, err)
		}
		if !withinDir(resolvedRoot, resolvedRepo) {
			return nil, warnings, fmt.Errorf(
				"artifact root %s (resolved %s) is outside the git repository %s (resolved %s)",
				wr.root, resolvedRoot, repoRoot, resolvedRepo)
		}

		err = filepath.WalkDir(wr.root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return fmt.Errorf("walk %s: %w", p, err)
			}
			if d.IsDir() {
				return nil
			}
			base := filepath.Base(p)
			relFile := path.Join(wr.rel, filepath.ToSlash(strings.TrimPrefix(p, wr.root)))
			if strings.HasPrefix(base, ".") {
				return nil // dot-prefixed files are expected noise
			}
			if d.Type()&fs.ModeSymlink != 0 {
				warnings = append(warnings, fmt.Sprintf("skipped symlink: %s", relFile))
				return nil
			}
			if !d.Type().IsRegular() {
				warnings = append(warnings, fmt.Sprintf("skipped non-regular file: %s", relFile))
				return nil
			}
			if reason, unsafe := unsafeBaseName(base); unsafe {
				warnings = append(warnings, fmt.Sprintf("skipped unsafe basename: %s (%s)", relFile, reason))
				return nil
			}
			content, err := os.ReadFile(p)
			if err != nil {
				return fmt.Errorf("read %s: %w", p, err)
			}
			sum := sha256.Sum256(content)
			c := candidate{
				Path:    relFile,
				SHA256:  hex.EncodeToString(sum[:]),
				Source:  SourceWorkingTree,
				Content: content,
				Kind:    wr.kind,
			}
			applyIdentity(&c, base, relFile, wr)
			if _, dup := byPath[relFile]; !dup {
				byPath[relFile] = struct{}{}
				candidates = append(candidates, c)
			}
			return nil
		})
		if err != nil {
			return nil, warnings, err
		}
	}

	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Path < candidates[j].Path })
	return candidates, warnings, nil
}

// workingTreeWalkRoots builds the deduplicated, deterministic list of walk
// roots: the vault-anchored default trio, the repository-anchored default
// trio, then the configured store paths sorted by repository-relative path.
// Roots colliding on repository-relative path are kept first-wins.
func workingTreeWalkRoots(repoRoot, resolvedRepo, vaultRoot, vaultRel string, storePaths []string) ([]walkRoot, error) {
	trioKinds := map[string]string{
		dirLearnings: KindLearning,
		dirCompiled:  KindCompiled,
		dirKnowledge: KindKnowledge,
	}
	trioNames := []string{dirLearnings, dirCompiled, dirKnowledge}

	var roots []walkRoot
	seen := make(map[string]struct{})
	add := func(root, rel, kind string, customStore bool, storeTag string) {
		if _, dup := seen[rel]; dup {
			return
		}
		seen[rel] = struct{}{}
		roots = append(roots, walkRoot{
			root:        root,
			rel:         rel,
			kind:        kind,
			customStore: customStore,
			storeTag:    storeTag,
		})
	}

	anchors := []struct {
		root string
		rel  string
	}{
		{vaultRoot, vaultRel},
		{repoRoot, ""},
	}
	for _, anchor := range anchors {
		for _, name := range trioNames {
			root := filepath.Join(anchor.root, dotUFRoot, deweyDir, name)
			rel := path.Join(anchor.rel, dotUFRoot, deweyDir, name)
			add(root, rel, trioKinds[name], false, "")
		}
	}

	type storeRoot struct {
		rel  string
		root walkRoot
	}
	var stores []storeRoot
	for _, sp := range storePaths {
		wr, err := validateStorePath(repoRoot, resolvedRepo, sp)
		if err != nil {
			return nil, err
		}
		stores = append(stores, storeRoot{rel: wr.rel, root: wr})
	}
	sort.Slice(stores, func(i, j int) bool { return stores[i].rel < stores[j].rel })
	for _, st := range stores {
		add(st.root.root, st.root.rel, st.root.kind, st.root.customStore, st.root.storeTag)
	}

	// Keep the walk order deterministic: vault trio, repo trio, then stores
	// in sorted order (the add loop above already appends in that order).
	return roots, nil
}

// validateStorePath normalizes a configured knowledge-store path and fails
// closed when it resolves outside the repository or carries control
// characters, a colon, or a leading-hyphen component.
func validateStorePath(repoRoot, resolvedRepo, storePath string) (walkRoot, error) {
	if storePath == "" {
		return walkRoot{}, fmt.Errorf("empty knowledge-store path")
	}
	if reason, unsafe := unsafePath(storePath); unsafe {
		return walkRoot{}, fmt.Errorf("knowledge-store path %q is invalid: %s", storePath, reason)
	}

	abs := filepath.Clean(storePath)
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(repoRoot, filepath.FromSlash(storePath))
	}

	resolved, err := filepath.EvalSymlinks(abs)
	if err == nil {
		if !withinDir(resolved, resolvedRepo) {
			return walkRoot{}, fmt.Errorf(
				"knowledge-store path %q resolves to %s, outside the git repository %s",
				storePath, resolved, resolvedRepo)
		}
		rel := relWithin(resolved, resolvedRepo)
		if rel == "" {
			return walkRoot{}, fmt.Errorf(
				"knowledge-store path %q resolves to the repository root itself", storePath)
		}
		return storeWalkRoot(abs, rel), nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		// A missing store is empty, but its lexical path must still sit
		// inside the repository — otherwise containment cannot be shown.
		if !withinDir(abs, repoRoot) {
			return walkRoot{}, fmt.Errorf(
				"knowledge-store path %q is outside the git repository %s", storePath, repoRoot)
		}
		rel := relWithin(abs, repoRoot)
		if rel == "" {
			return walkRoot{}, fmt.Errorf(
				"knowledge-store path %q resolves to the repository root itself", storePath)
		}
		return storeWalkRoot(abs, rel), nil
	}
	return walkRoot{}, fmt.Errorf("resolve knowledge-store path %q: %w", storePath, err)
}

// storeWalkRoot builds the walk root for a knowledge store whose items take
// their identity from the path below the store root and their tag from the
// store name.
func storeWalkRoot(abs, rel string) walkRoot {
	return walkRoot{
		root:        abs,
		rel:         rel,
		kind:        KindKnowledge,
		customStore: true,
		storeTag:    path.Base(rel),
	}
}

// verifyWalkRootComponents Lstats every component of the walk root below the
// repository root and fails closed when any existing component is a
// symbolic link — lexical containment is not physical containment. A missing
// component means the root itself is missing, which callers treat as empty.
func verifyWalkRootComponents(repoRoot, rel string) error {
	comps := strings.Split(strings.Trim(rel, "/"), "/")
	if len(comps) == 1 && comps[0] == "" {
		return nil
	}
	for i := 1; i <= len(comps); i++ {
		p := filepath.Join(repoRoot, filepath.FromSlash(strings.Join(comps[:i], "/")))
		info, err := os.Lstat(p)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil // root (or an ancestor) is missing: empty
			}
			return fmt.Errorf("inspect component %s: %w", p, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf(
				"symbolic-link component %q — lexical containment is not physical containment",
				comps[i-1])
		}
	}
	return nil
}

// applyIdentity fills the display and PR-grouping metadata on a candidate
// according to its kind: learnings use the filename stem with the tag as
// its first hyphen-separated segment; compiled articles use the stem as
// both identity and tag (the compiled filename is `{tag}.md`); knowledge
// content uses the store path as identity with the store name as tag.
func applyIdentity(c *candidate, base, relFile string, wr walkRoot) {
	stem := base
	if strings.HasSuffix(base, ".md") {
		stem = strings.TrimSuffix(base, ".md")
	}
	switch wr.kind {
	case KindLearning:
		c.Identity = stem
		c.Tag = strings.SplitN(stem, "-", 2)[0]
	case KindCompiled:
		c.Identity = stem
		c.Tag = stem
	case KindKnowledge:
		relUnder := strings.TrimPrefix(relFile, wr.rel+"/")
		if wr.customStore {
			c.Identity = relUnder
			c.Tag = wr.storeTag
		} else {
			c.Identity = relUnder
			c.Tag = strings.SplitN(relUnder, "/", 2)[0]
		}
	}
}

// unsafeBaseName reports whether a basename carries control characters, a
// colon, or a leading hyphen — forms that are ambiguous in git revision
// syntax or hostile on the command line.
func unsafeBaseName(base string) (reason string, unsafe bool) {
	if hasControlChars(base) {
		return "control character in basename", true
	}
	if strings.ContainsRune(base, ':') {
		return "colon in basename", true
	}
	if strings.HasPrefix(base, "-") {
		return "leading hyphen in basename", true
	}
	return "", false
}

// unsafePath applies the store-path charset rules to a whole path.
func unsafePath(p string) (reason string, unsafe bool) {
	if hasControlChars(p) {
		return "control character in path", true
	}
	if strings.ContainsRune(p, ':') {
		return "colon in path", true
	}
	for _, comp := range strings.FieldsFunc(p, func(r rune) bool { return r == '/' || r == '\\' }) {
		if strings.HasPrefix(comp, "-") {
			return "leading hyphen in path component", true
		}
	}
	return "", false
}

// hasControlChars reports whether s contains C0 control characters or DEL.
func hasControlChars(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

// withinDir reports whether path is dir itself or lives underneath it.
// Both must be cleaned (and ideally resolved) paths.
func withinDir(p, dir string) bool {
	if p == dir {
		return true
	}
	return strings.HasPrefix(p, dir+string(filepath.Separator))
}

// relWithin returns the slash-separated path of p relative to dir (both
// cleaned, p known to be inside dir); the empty string when p is dir itself.
func relWithin(p, dir string) string {
	if p == dir {
		return ""
	}
	return filepath.ToSlash(strings.TrimPrefix(p, dir+string(filepath.Separator)))
}
