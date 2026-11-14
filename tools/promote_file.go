package tools

// Frontmatter file-edit mechanics for the promote tool
// (openspec/changes/git-is-the-source-of-record-for-page-trust-tiers,
// PW-FR-001, PW-FR-002, PW-FR-004): promotion is a `tier: validated`
// frontmatter mutation on the current branch's file, not a `pages.tier`
// database write. The database tier is a cache recomputed on the next
// index by the git-trust derivation (design D4), so this file owns the
// path-resolution and atomic-write machinery the promotion act needs.

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/unbound-force/dewey/v3/store"
)

// tierValidated is the canonical validated tier value promote writes into
// page frontmatter (CS-012: the domain vocabulary as a constant).
const tierValidated = "validated"

// promotePrefixes are the vault-relative path prefixes whose files promote
// may edit (PW-FR-004). They cover both on-disk homes of the git-trust page
// families: the knowledge-repo layout at the vault root (`compiled/`,
// `knowledge/` — the paths git-trust blob comparison keys on) and the dewey
// workspace layout (`.uf/dewey/compiled/`, `.uf/dewey/knowledge/` — where
// store_compiled and the curation pipeline write their files). Learning
// pages are absent from the list in both layouts: learnings stay draft
// forever (IP-FR-002) and are never promotable.
var promotePrefixes = []string{
	"compiled/",
	"knowledge/",
	deweyWorkspaceDir + "/compiled/",
	deweyWorkspaceDir + "/knowledge/",
}

// promoteEdit is one prepared, not-yet-applied frontmatter mutation. All
// edits in a batch are prepared (paths validated, contents read and
// rewritten in memory) before any is applied, which keeps the batch
// all-or-nothing: a failure during preparation touches nothing, and a
// failure during application restores the already-applied edits from their
// in-memory originals.
type promoteEdit struct {
	page        string      // resolved page name for reporting
	relPath     string      // vault-relative slash path of the edited file
	absPath     string      // fully resolved absolute path (all symlinks resolved)
	origContent []byte      // pre-edit content, kept for rollback
	newContent  []byte      // post-edit content (frontmatter tier rewritten)
	mode        os.FileMode // original file mode, preserved by the rename
}

// resolveVaultRoot returns the absolute, symlink-resolved vault root that
// promote paths are validated against (PW-FR-004: resolution starts from
// the vault root). An empty or unresolvable vault path is a configuration
// error the handler reports before touching any page.
func resolveVaultRoot(vaultPath string) (string, error) {
	if strings.TrimSpace(vaultPath) == "" {
		return "", fmt.Errorf("vault path is not configured")
	}
	abs, err := filepath.Abs(vaultPath)
	if err != nil {
		return "", fmt.Errorf("resolve vault path %q: %w", vaultPath, err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolve vault root %q: %w", abs, err)
	}
	return resolved, nil
}

// checkPromoteArgument rejects raw promote arguments that can never name a
// promotable target (PW-FR-004): absolute paths and paths containing `..`
// segments, before any store lookup or filesystem access. The check runs on
// the argument as given — before canonicalization — so `compiled/evil/../../s`
// fails here rather than being silently cleaned into an escape.
func checkPromoteArgument(name string) error {
	trimmed := strings.TrimSpace(name)
	if filepath.IsAbs(trimmed) || strings.HasPrefix(filepath.ToSlash(trimmed), "/") {
		return fmt.Errorf("invalid page path %q: absolute paths are not allowed", name)
	}
	for _, seg := range strings.Split(filepath.ToSlash(trimmed), "/") {
		if seg == ".." {
			return fmt.Errorf("invalid page path %q: '..' segments are not allowed", name)
		}
	}
	return nil
}

// checkPromoteRelPath performs the lexical half of PW-FR-004 path
// validation on a vault-relative path: it rejects empty, absolute, and
// `..`-bearing paths, cleans the remainder, and verifies the cleaned path
// sits under a promotable prefix. Cleaning matters because a path like
// `compiled/evil/../../secrets` canonicalizes outside `compiled/` — the
// cleaned form must pass the prefix check too ("..` segments before or
// after canonicalization" are both rejected). It returns the cleaned
// slash-separated path.
func checkPromoteRelPath(rel string) (string, error) {
	cleaned := path.Clean(strings.TrimSpace(filepath.ToSlash(rel)))
	if cleaned == "" || cleaned == "." {
		return "", fmt.Errorf("invalid page path %q: empty", rel)
	}
	if strings.HasPrefix(cleaned, "/") {
		return "", fmt.Errorf("invalid page path %q: absolute paths are not allowed", rel)
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("invalid page path %q: '..' segments are not allowed", rel)
	}
	if !isAllowedPromotePrefix(cleaned) {
		return "", fmt.Errorf("invalid page path %q: only files under compiled/ or knowledge/ can be promoted", rel)
	}
	return cleaned, nil
}

// isAllowedPromotePrefix reports whether the cleaned vault-relative path
// falls under one of the promotable prefixes. The trailing slash is
// load-bearing: a top-level file named `compiled.md` or a sibling directory
// like `compiled-notes/` is not promotable.
func isAllowedPromotePrefix(cleanedRel string) bool {
	for _, prefix := range promotePrefixes {
		if strings.HasPrefix(cleanedRel, prefix) {
			return true
		}
	}
	return false
}

// resolvePromotePath performs the filesystem half of PW-FR-004 validation:
// it joins the cleaned relative path under the resolved vault root, resolves
// symlinks in every component to a fully resolved absolute path, and verifies
// that the resolved path stays inside the vault root and under a promotable
// prefix. A symlink anywhere in the path that escapes the vault or lands
// outside the allowed prefixes is a validation failure, never a follow. A
// missing final path surfaces as fs.ErrNotExist so callers can distinguish
// "no such file" (fall through to the not-found error) from a validation
// failure (report as a path error). It returns the resolved vault-relative
// slash path and the resolved absolute path.
func resolvePromotePath(resolvedRoot, cleanedRel string) (string, string, error) {
	joined := filepath.Join(resolvedRoot, filepath.FromSlash(cleanedRel))
	resolved, err := filepath.EvalSymlinks(joined)
	if err != nil {
		return "", "", err
	}
	if !pathWithinRoot(resolved, resolvedRoot) {
		return "", "", fmt.Errorf("resolved path %q escapes the vault root", resolved)
	}
	resolvedRel, err := filepath.Rel(resolvedRoot, resolved)
	if err != nil {
		return "", "", fmt.Errorf("relativize %q against %q: %w", resolved, resolvedRoot, err)
	}
	resolvedRel = filepath.ToSlash(resolvedRel)
	if !isAllowedPromotePrefix(resolvedRel) {
		return "", "", fmt.Errorf("resolved path %q is outside the promotable prefixes (compiled/, knowledge/)", resolvedRel)
	}
	return resolvedRel, resolved, nil
}

// pathWithinRoot reports whether abs is the root itself or lies underneath
// it. Both arguments must already be fully resolved (symlinks evaluated) so
// the prefix comparison cannot be fooled by an alias of the root.
func pathWithinRoot(abs, root string) bool {
	if abs == root {
		return true
	}
	return strings.HasPrefix(abs, root+string(os.PathSeparator))
}

// promotableFileCandidates lists the vault-relative file paths a resolved
// store page could be backed by, in preference order. Both homes of the
// git-trust page families are probed: the vault-root layout (`compiled/x.md`,
// the knowledge-repo shape the git-trust blob comparison keys on) and the
// dewey workspace layout (`.uf/dewey/compiled/x.md`, where store_compiled
// and knowledge stores write). store_compiled pages are probed
// workspace-first because that is where their files live. The source doc ID
// is included because disk-source pages carry the vault-relative file path
// there while their page names carry a source-ID prefix. Candidates that
// fail validation (for example a source-prefixed name) are simply skipped
// by the caller.
func promotableFileCandidates(page *store.Page) []string {
	var out []string
	add := func(rel string) {
		if rel == "" {
			return
		}
		if !strings.HasSuffix(rel, ".md") {
			rel += ".md"
		}
		out = append(out, path.Clean(filepath.ToSlash(rel)))
	}
	if page.SourceID == store.SourceCompiled {
		add(deweyWorkspaceDir + "/" + page.Name)
	}
	add(page.Name)
	add(deweyWorkspaceDir + "/" + page.Name)
	add(page.SourceDocID)
	return dedupeStrings(out)
}

// resolvePromotableFile maps a resolved store page to the vault-relative
// and absolute paths of the file promotion must edit. Learning pages are
// refused outright with the IP-FR-002 rationale (learnings stay draft
// forever; no path escalates a learning). Otherwise the first candidate
// that passes PW-FR-004 validation and exists as a regular file wins; when
// none does, the error names every path considered so the caller can see
// why the page has no promotable file.
func resolvePromotableFile(resolvedRoot string, page *store.Page) (string, string, error) {
	if isLearningPageRef(page) {
		return "", "", fmt.Errorf(
			"learning pages are never promotable: learnings stay draft (only compiled/ and knowledge/ pages can be promoted)")
	}
	candidates := promotableFileCandidates(page)
	for _, cand := range candidates {
		cleaned, err := checkPromoteRelPath(cand)
		if err != nil {
			continue // candidate from another layout; not a user-input error
		}
		relSlash, abs, err := resolvePromotePath(resolvedRoot, cleaned)
		if err != nil {
			continue // missing file or unresolvable symlink chain in this layout
		}
		if info, statErr := os.Stat(abs); statErr != nil || !info.Mode().IsRegular() {
			continue // exists but is not a regular page file (e.g. a directory)
		}
		return relSlash, abs, nil
	}
	return "", "", fmt.Errorf(
		"no promotable file found for page %q under compiled/ or knowledge/ (looked at: %s)",
		page.Name, strings.Join(candidates, ", "))
}

// isLearningPageRef reports whether the page belongs to the learning family
// (IP-FR-002): a page name or source doc ID under `learning/` or
// `learnings/`, in either the vault-root or workspace layout. This mirrors
// vault's unexported isLearningPage predicate, which is deliberately kept
// local to the ingestion package; promote keeps its own copy at the tools
// seam.
func isLearningPageRef(page *store.Page) bool {
	for _, ref := range []string{page.Name, page.SourceDocID} {
		name := strings.ToLower(strings.TrimSuffix(filepath.ToSlash(ref), ".md"))
		for _, prefix := range learningRefPrefixes {
			if strings.HasPrefix(name, prefix) {
				return true
			}
		}
	}
	return false
}

// learningRefPrefixes are the page-name prefixes marking the learning
// family: the store's `learning/` namespace (learning.LearningPrefix) and
// the `learnings/` directory name the markdown backups live in.
var learningRefPrefixes = []string{store.LearningPrefix, "learnings/"}

// buildPromoteEdit reads the validated target file and computes its
// post-promotion content. The read is the no-follow half of PW-FR-004's
// TOCTOU defense: opening with O_NOFOLLOW after validation means a symlink
// swapped onto the final component between validation and read is treated
// as a validation failure (ELOOP) rather than followed.
func buildPromoteEdit(page, rel string, abs string) (promoteEdit, error) {
	content, mode, err := readPromoteTarget(abs)
	if err != nil {
		return promoteEdit{}, err
	}
	return promoteEdit{
		page:        page,
		relPath:     rel,
		absPath:     abs,
		origContent: content,
		newContent:  setFrontmatterTier(content, tierValidated),
		mode:        mode,
	}, nil
}

// readPromoteTarget opens the fully resolved target with O_NOFOLLOW and
// returns its content and mode. O_NOFOLLOW makes the open refuse a symlink
// final component (the TOCTOU window between validation and mutation), and
// the mode is taken from the open file's own Stat rather than a path Stat,
// so it describes the file actually read.
func readPromoteTarget(abs string) ([]byte, os.FileMode, error) {
	f, err := os.OpenFile(abs, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, 0, fmt.Errorf("open page file %s: %w", abs, err)
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return nil, 0, fmt.Errorf("stat page file %s: %w", abs, err)
	}
	if !info.Mode().IsRegular() {
		return nil, 0, fmt.Errorf("page file %s is not a regular file", abs)
	}
	content, err := io.ReadAll(f)
	if err != nil {
		return nil, 0, fmt.Errorf("read page file %s: %w", abs, err)
	}
	mode := info.Mode().Perm()
	if mode == 0 {
		mode = 0o644
	}
	return content, mode, nil
}

// applyPromoteEdits applies every prepared edit atomically, preserving the
// batch all-or-nothing contract: when an edit fails mid-batch, the edits
// already applied are restored from their in-memory originals with the same
// atomic write, so a failed batch leaves no promotable file changed.
func applyPromoteEdits(edits []promoteEdit) error {
	for i := range edits {
		if err := writeFileAtomic(edits[i].absPath, edits[i].newContent, edits[i].mode); err != nil {
			restored := restoreAppliedEdits(edits[:i])
			if restored > 0 {
				return fmt.Errorf("promote %s: %v (rolled back %d earlier edit(s))", edits[i].relPath, err, restored)
			}
			return fmt.Errorf("promote %s: %v (no files were changed)", edits[i].relPath, err)
		}
	}
	return nil
}

// restoreAppliedEdits writes the original contents back over already-applied
// edits, most recent first, and reports how many restores succeeded. Restore
// failures are not escalatable — the caller already failed — so they are
// counted rather than returned and the error message reports the shortfall.
func restoreAppliedEdits(applied []promoteEdit) int {
	restored := 0
	for i := len(applied) - 1; i >= 0; i-- {
		if err := writeFileAtomic(applied[i].absPath, applied[i].origContent, applied[i].mode); err == nil {
			restored++
		}
	}
	return restored
}

// writeFileAtomic replaces the file at path with content via
// write-temp-in-same-directory + fsync + rename, preserving mode (PW-FR-001).
// On any failure the original file is unchanged: the temp file is removed
// and an error returned. rename(2) replaces the directory entry itself and
// never writes through a symlink, so even a symlink swapped onto the final
// component after the O_NOFOLLOW read is replaced (not followed) — no file
// outside the allowed prefixes is ever modified (PW-FR-004 TOCTOU).
func writeFileAtomic(path string, content []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".promote-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	if err := writeTempFile(tmp, content, mode); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename over %s: %w", path, err)
	}
	return nil
}

// writeTempFile writes, syncs, closes, and chmods the temp file ahead of its
// rename. The mode is applied to the temp file (CreateTemp creates 0600) so
// the rename preserves the original file's mode exactly (PW-FR-001).
func writeTempFile(tmp *os.File, content []byte, mode os.FileMode) error {
	if _, err := tmp.Write(content); err != nil {
		return fmt.Errorf("write temp file %s: %w", tmp.Name(), err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync temp file %s: %w", tmp.Name(), err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file %s: %w", tmp.Name(), err)
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return fmt.Errorf("chmod temp file %s: %w", tmp.Name(), err)
	}
	return nil
}

// setFrontmatterTier returns content with `tier: <tier>` written into the
// YAML frontmatter, creating the frontmatter block when absent (PW-FR-001).
// The edit is line-oriented surgery, not a YAML round-trip: every
// non-matching line — all other frontmatter keys, their ordering, and the
// entire body — is preserved byte-for-byte. A top-level `tier:` key has its
// value replaced in place; an existing block without one gets
// `tier: <tier>` inserted directly after the opening delimiter; a file with
// no frontmatter (including a file whose opening `---` has no closing
// delimiter, mirroring vault.parseFrontmatter's no-frontmatter fallback)
// gets a fresh block prepended.
func setFrontmatterTier(content []byte, tier string) []byte {
	text := string(content)
	if text == "" {
		return []byte(fmt.Sprintf("---\ntier: %s\n---\n", tier))
	}
	lines := strings.Split(text, "\n")
	// strings.Split/Join are exact inverses: rejoining untouched lines
	// reproduces the original bytes, including any \r line endings and the
	// trailing empty element of a newline-terminated file.
	if strings.TrimRight(lines[0], "\r") != "---" {
		return []byte(fmt.Sprintf("---\ntier: %s\n---\n%s", tier, text))
	}
	closeIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r") == "---" {
			closeIdx = i
			break
		}
	}
	if closeIdx < 0 {
		return []byte(fmt.Sprintf("---\ntier: %s\n---\n%s", tier, text))
	}
	tierLine := "tier: " + tier
	for i := 1; i < closeIdx; i++ {
		if isTopLevelYAMLKey(lines[i], "tier") {
			if strings.HasSuffix(lines[i], "\r") {
				tierLine += "\r"
			}
			lines[i] = tierLine
			return []byte(strings.Join(lines, "\n"))
		}
	}
	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[0], tierLine)
	out = append(out, lines[1:]...)
	return []byte(strings.Join(out, "\n"))
}

// isTopLevelYAMLKey reports whether line is a top-level definition of the
// given key (`key:` at column zero). Indented lines (nested mappings, block
// scalars) and longer keys sharing the prefix (`tiers:`) do not match, so
// only the page's own tier declaration is rewritten.
func isTopLevelYAMLKey(line, key string) bool {
	return strings.HasPrefix(strings.TrimRight(line, "\r"), key+":")
}

// dedupeStrings removes duplicate entries while preserving order.
func dedupeStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
