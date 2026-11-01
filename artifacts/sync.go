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
	path     string
	sha256   string
	sources  []string
	kind     string
	tag      string
	identity string
}

// plannedWrites derives the write set from the plan: every artifact plus every
// conflict resolved through the path-keyed resolutions map. A conflict without
// a resolution, or a resolution naming a sha that is not one of the conflict's
// variants, fails closed.
func plannedWrites(plan *Plan, resolutions map[string]string) ([]planWrite, error) {
	var writes []planWrite
	for _, art := range plan.Artifacts {
		writes = append(writes, planWrite{path: art.Path, sha256: art.SHA256, sources: art.Sources, kind: art.Kind, tag: art.Tag, identity: art.Identity})
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
		kind, tag := deriveKindTag(conf.Path, conf.Identity, plan.Repo.ArtifactRoots)
		writes = append(writes, planWrite{path: conf.Path, sha256: sha, sources: sources, kind: kind, tag: tag, identity: conf.Identity})
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

// stagePlannedFiles stages the planned artifact files in the sync worktree
// with `git add -f` — the force flag is load-bearing because repositories
// ignoring .uf/ wholesale are the most common stranded state, and tracked
// files legitimately override gitignore (design D7 step 6). The repaired
// .gitignore rides along when a fix was applied. After staging, the cached
// index is verified against the planned set exactly; any mismatch fails
// closed rather than publishing an unplanned or incomplete change.
func stagePlannedFiles(worktree string, filesWritten []string, gitignoreFixApplied bool) error {
	args := make([]string, 0, len(filesWritten)+5)
	args = append(args, "add", "-f", "--")
	args = append(args, filesWritten...)
	if gitignoreFixApplied {
		args = append(args, ".gitignore")
	}
	if _, err := run(worktree, args...); err != nil {
		return fmt.Errorf("git add: %w", err)
	}

	out, err := run(worktree, "diff", "--cached", "--name-only")
	if err != nil {
		return fmt.Errorf("git diff --cached: %w", err)
	}
	staged := []string{}
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			staged = append(staged, line)
		}
	}
	sort.Strings(staged)

	planned := append([]string{}, filesWritten...)
	if gitignoreFixApplied {
		planned = append(planned, ".gitignore")
	}
	sort.Strings(planned)

	if len(staged) != len(planned) {
		return fmt.Errorf("staged set mismatch: staged %v, planned %v", staged, planned)
	}
	for i := range staged {
		if staged[i] != planned[i] {
			return fmt.Errorf("staged set mismatch: staged %v, planned %v", staged, planned)
		}
	}
	return nil
}

// deriveKindTag recovers the kind and tag for a resolved conflict path. The
// plan JSON's conflict entries carry path and identity but not the enumeration
// time kind/tag, so sync re-derives them from the plan's recorded artifact
// roots using the same rules the enumerators applied: learnings roots give the
// first hyphen segment of the identity as tag, compiled roots use the identity
// stem itself, the standard knowledge root uses the first path segment, and
// custom store roots use the store directory basename.
func deriveKindTag(path, identity string, roots []string) (kind, tag string) {
	for _, root := range roots {
		if !strings.HasPrefix(path, root) {
			continue
		}
		switch {
		case strings.HasSuffix(root, "/learnings/"):
			return KindLearning, strings.SplitN(identity, "-", 2)[0]
		case strings.HasSuffix(root, "/compiled/"):
			return KindCompiled, identity
		case strings.HasSuffix(root, "/knowledge/"):
			rel := strings.TrimPrefix(path, root)
			if seg := strings.SplitN(rel, "/", 2)[0]; seg != "" {
				return KindKnowledge, seg
			}
			return KindKnowledge, identity
		default:
			return KindKnowledge, filepath.Base(strings.TrimSuffix(root, "/"))
		}
	}
	return KindKnowledge, identity
}

// renderCommitMessage renders the deterministic Conventional Commit message
// for the chore commit (design D7 step 7): subject line with the artifact
// count, per-kind counts, the sorted tag list, the non-destructive note, and
// the Approved-by and Plan-sha256 trailers binding the commit to the consumed
// plan content. The approver is control-character-stripped so a newline cannot
// forge additional trailers.
func renderCommitMessage(writes []planWrite, approvedBy, planSHA256 string) string {
	kindCounts := map[string]int{}
	tagSet := map[string]bool{}
	for _, w := range writes {
		kindCounts[w.kind]++
		if w.tag != "" {
			tagSet[w.tag] = true
		}
	}
	kinds := make([]string, 0, len(kindCounts))
	for k := range kindCounts {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	tags := make([]string, 0, len(tagSet))
	for t := range tagSet {
		tags = append(tags, t)
	}
	sort.Strings(tags)

	var b strings.Builder
	fmt.Fprintf(&b, "chore(dewey): publish %d knowledge artifacts\n\n", len(writes))
	for _, k := range kinds {
		noun := "artifacts"
		if kindCounts[k] == 1 {
			noun = "artifact"
		}
		fmt.Fprintf(&b, "- %d %s %s\n", kindCounts[k], k, noun)
	}
	fmt.Fprintf(&b, "\nTags: %s\n", strings.Join(tags, ", "))
	b.WriteString("\nNon-destructive: adds knowledge artifacts only; no source changes.\n\n")
	fmt.Fprintf(&b, "Approved-by: %s\n", stripControl(approvedBy))
	fmt.Fprintf(&b, "Plan-sha256: %s\n", planSHA256)
	return b.String()
}

// renderPRBody renders the tool-rendered PR-body fragment (design DE-003):
// tag-grouped itemized listing with identity, kind, and sha256 per artifact,
// per-kind counts, the non-destructive note, the optional gitignore-fix note,
// and the approver. Rendered strings are control-character-stripped and
// markdown-escaped; the command layer relays the fragment verbatim.
func renderPRBody(writes []planWrite, gitignoreFixApplied bool, approvedBy string) string {
	kindCounts := map[string]int{}
	tagWrites := map[string][]planWrite{}
	tagSet := map[string]bool{}
	for _, w := range writes {
		kindCounts[w.kind]++
		tag := w.tag
		if tag == "" {
			tag = "(none)"
		}
		tagSet[tag] = true
		tagWrites[tag] = append(tagWrites[tag], w)
	}
	kinds := make([]string, 0, len(kindCounts))
	for k := range kindCounts {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	counts := make([]string, 0, len(kinds))
	for _, k := range kinds {
		counts = append(counts, fmt.Sprintf("%d %s", kindCounts[k], k))
	}
	tags := make([]string, 0, len(tagSet))
	for t := range tagSet {
		tags = append(tags, t)
	}
	sort.Strings(tags)

	var b strings.Builder
	b.WriteString("## Published knowledge artifacts\n\n")
	fmt.Fprintf(&b, "%d artifacts across %d tags (%s).\n\n", len(writes), len(tags), strings.Join(counts, ", "))
	for _, tag := range tags {
		fmt.Fprintf(&b, "### tag: %s\n", renderString(tag))
		for _, w := range tagWrites[tag] {
			fmt.Fprintf(&b, "- %s (identity: %s, %s, sha256: %s)\n", renderString(w.path), renderString(w.identity), w.kind, w.sha256)
		}
		b.WriteString("\n")
	}
	if gitignoreFixApplied {
		b.WriteString("## Gitignore fix\n\n")
		b.WriteString("The bare `dewey` pattern was replaced with the anchored ignore block so runtime state stays ignored after merge.\n\n")
	}
	b.WriteString("## Notes\n\n")
	b.WriteString("- Non-destructive: adds knowledge artifacts only; no source changes.\n")
	fmt.Fprintf(&b, "- Approved by %s.\n", renderString(approvedBy))
	return b.String()
}

// commitPlanned commits the staged set in the sync worktree with the rendered
// message and returns the new commit's sha256.
func commitPlanned(worktree, message string) (string, error) {
	if _, err := run(worktree, "commit", "-m", message); err != nil {
		return "", fmt.Errorf("git commit: %w", err)
	}
	out, err := run(worktree, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("resolve commit: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// RunSync executes the publish sync for a consumed plan file: it validates the
// plan structurally (fail closed before any effect), materializes the planned
// files in an OS-tempdir git worktree on a fresh chore branch, stages and
// verifies them, and commits with the deterministic chore message carrying the
// Approved-by and Plan-sha256 trailers (design D7).
//
// The sanctioned artifact directories are re-derived at execution time from
// the repository, vault root, and store paths — the plan's recorded
// repo.artifact_roots is only a cross-check, never the confinement authority.
// Collisions must carry path-keyed resolutions naming a clean variant.
// approve must be true (the human gate lives in the command layer); approvedBy
// must be printable-only. An empty publish set returns NothingToPublish without
// creating anything.
func RunSync(repoRoot, vaultRoot string, storePaths []string, planPath, resolutionsPath string, approve bool, approvedBy string) (*SyncResult, error) {
	if !approve {
		return nil, fmt.Errorf("sync requires --approve — present the plan's rendered summary to the human gate before approving")
	}
	if err := validateApprovedBy(approvedBy); err != nil {
		return nil, err
	}
	approvedBy, err := resolveApprover(repoRoot, approvedBy)
	if err != nil {
		return nil, err
	}

	repoRoot = filepath.Clean(repoRoot)
	vaultRoot = filepath.Clean(vaultRoot)
	resolvedRepo, err := filepath.EvalSymlinks(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve repository root %s: %w", repoRoot, err)
	}
	resolvedVault, err := filepath.EvalSymlinks(vaultRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve vault root %s: %w", vaultRoot, err)
	}
	if !withinDir(resolvedVault, resolvedRepo) {
		return nil, fmt.Errorf("vault root %s (resolved %s) is outside the git repository %s (resolved %s)", vaultRoot, resolvedVault, repoRoot, resolvedRepo)
	}
	roots, err := workingTreeWalkRoots(repoRoot, resolvedRepo, vaultRoot, relWithin(resolvedVault, resolvedRepo), storePaths)
	if err != nil {
		return nil, fmt.Errorf("derive sanctioned roots: %w", err)
	}
	sanctioned := sanctionedRoots(roots)

	plan, planSHA256, err := loadPlan(planPath)
	if err != nil {
		return nil, err
	}
	if err := validatePlan(plan, sanctioned); err != nil {
		return nil, err
	}

	resolutions := map[string]string{}
	if resolutionsPath != "" {
		resolutions, err = loadResolutions(resolutionsPath)
		if err != nil {
			return nil, err
		}
	}
	if err := validateResolutions(plan, resolutions); err != nil {
		return nil, err
	}

	writes, err := plannedWrites(plan, resolutions)
	if err != nil {
		return nil, err
	}
	if len(writes) == 0 {
		return &SyncResult{
			FilesWritten:     []string{},
			NothingToPublish: true,
			ApprovedBy:       approvedBy,
			PlanSHA256:       planSHA256,
		}, nil
	}

	worktree, branch, err := createSyncWorktree(repoRoot, plan.Repo.BaseSHA, time.Now())
	if err != nil {
		return nil, err
	}
	filesWritten, gitignoreFixApplied, err := materializePlan(repoRoot, worktree, plan, resolutions)
	if err != nil {
		return nil, fmt.Errorf("%w (worktree dir left at %s; remove it and run `git worktree prune`)", err, worktree)
	}
	if err := stagePlannedFiles(worktree, filesWritten, gitignoreFixApplied); err != nil {
		return nil, fmt.Errorf("%w (worktree dir left at %s; remove it and run `git worktree prune`)", err, worktree)
	}
	commitSHA, err := commitPlanned(worktree, renderCommitMessage(writes, approvedBy, planSHA256))
	if err != nil {
		return nil, fmt.Errorf("%w (worktree dir left at %s; remove it and run `git worktree prune`)", err, worktree)
	}
	return &SyncResult{
		Branch:              branch,
		Worktree:            worktree,
		Commit:              commitSHA,
		FilesWritten:        filesWritten,
		GitignoreFixApplied: gitignoreFixApplied,
		NothingToPublish:    false,
		PRBody:              renderPRBody(writes, gitignoreFixApplied, approvedBy),
		ApprovedBy:          approvedBy,
		PlanSHA256:          planSHA256,
	}, nil
}

// resolveApprover returns the approval identity: an explicit --approved-by
// value, falling back to the repository's git config user.name. An empty
// resolved identity fails closed — the audit trail requires a named approver.
func resolveApprover(repoRoot, approvedBy string) (string, error) {
	if approvedBy != "" {
		return approvedBy, nil
	}
	out, err := run(repoRoot, "config", "user.name")
	if err != nil {
		return "", fmt.Errorf("no approver identity — pass --approved-by or set git config user.name")
	}
	if name := strings.TrimSpace(string(out)); name != "" {
		return name, nil
	}
	return "", fmt.Errorf("no approver identity — pass --approved-by or set git config user.name")
}

// validateApprovedBy rejects approver identities carrying control characters:
// a newline in --approved-by could forge additional commit trailers such as a
// second Plan-sha256 (design D7 step 1). An empty identity is allowed here;
// the fallback resolution happens later in the flow.
func validateApprovedBy(s string) error {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("approved-by contains control characters")
		}
	}
	return nil
}

// validatePlan performs the structural, fail-closed validation of a consumed
// plan against the execution-time sanctioned roots: schema version, hex
// fields, source-label vocabulary, path hygiene and confinement, and the
// bounded gitignore-fix shape (design D7 step 1). Any violation is an error
// before any worktree, branch, or file exists.
func validatePlan(plan *Plan, sanctioned []string) error {
	if plan.SchemaVersion != PlanSchemaVersion {
		return fmt.Errorf("unsupported plan schema version %d", plan.SchemaVersion)
	}
	if !isHex40(plan.Repo.BaseSHA) {
		return fmt.Errorf("plan repo.base_sha %q is not 40-hex", plan.Repo.BaseSHA)
	}
	labels := make([]string, 0, len(plan.Repo.StashRefs))
	for label := range plan.Repo.StashRefs {
		labels = append(labels, label)
	}
	sort.Strings(labels)
	for _, label := range labels {
		if !isHex40(plan.Repo.StashRefs[label]) {
			return fmt.Errorf("plan repo.stash_refs[%s] is not 40-hex", label)
		}
	}
	if err := matchRoots(plan.Repo.ArtifactRoots, sanctioned); err != nil {
		return err
	}
	for _, art := range plan.Artifacts {
		if err := validPlanPath(art.Path, sanctioned); err != nil {
			return err
		}
		if !isHex64(art.SHA256) {
			return fmt.Errorf("plan artifact %s sha256 is not 64-hex", art.Path)
		}
		for _, src := range art.Sources {
			if !validSourceLabel(src) {
				return fmt.Errorf("plan artifact %s carries vocabulary-violating source label %q", art.Path, src)
			}
		}
	}
	for _, conf := range plan.Conflicts {
		if err := validPlanPath(conf.Path, sanctioned); err != nil {
			return err
		}
		for _, v := range conf.Variants {
			if !isHex64(v.SHA256) {
				return fmt.Errorf("plan conflict %s variant sha256 is not 64-hex", conf.Path)
			}
			for _, src := range v.Sources {
				if !validSourceLabel(src) {
					return fmt.Errorf("plan conflict %s carries vocabulary-violating source label %q", conf.Path, src)
				}
			}
		}
	}
	for _, b := range plan.Blocked {
		if err := validPlanPath(b.Path, sanctioned); err != nil {
			return err
		}
	}
	if fix := plan.Gitignore.Fix; fix != nil {
		if fix.File != ".gitignore" {
			return fmt.Errorf("gitignore fix names unsupported file %q", fix.File)
		}
		if fix.Anchor.From != "dewey" || fix.Anchor.To != "/dewey" {
			return fmt.Errorf("gitignore fix anchor is not the bounded dewey-to-/dewey shape")
		}
		if len(fix.Reignore) == 0 {
			return fmt.Errorf("gitignore fix carries no runtime-state re-ignores")
		}
	}
	return nil
}

// validPlanPath enforces plan-path hygiene: repository-relative, clean,
// traversal-free, free of control characters, colons, and leading-hyphen
// components, and confined to the sanctioned artifact directories.
func validPlanPath(p string, sanctioned []string) error {
	if filepath.IsAbs(p) {
		return fmt.Errorf("plan path %q is absolute", p)
	}
	if p != filepath.Clean(p) {
		return fmt.Errorf("plan path %q is not clean", p)
	}
	if hasControlChars(p) {
		return fmt.Errorf("plan path %q carries control characters", p)
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return fmt.Errorf("plan path %q traverses out of the repository", p)
		}
		if strings.HasPrefix(seg, "-") {
			return fmt.Errorf("plan path %q has a leading-hyphen component", p)
		}
		if strings.Contains(seg, ":") {
			return fmt.Errorf("plan path %q carries a colon", p)
		}
	}
	for _, root := range sanctioned {
		if strings.HasPrefix(p, root) {
			return nil
		}
	}
	return fmt.Errorf("plan path %q is outside the sanctioned artifact directories", p)
}

// validSourceLabel reports whether a plan source label is in the plan
// vocabulary: working-tree, stash:stash@{N}, stash-untracked:stash@{N}, or
// commit:<40-hex>.
func validSourceLabel(s string) bool {
	if s == SourceWorkingTree {
		return true
	}
	if rest, ok := strings.CutPrefix(s, SourceStashUntrackedPrefix); ok {
		return isStashLabel(rest)
	}
	if rest, ok := strings.CutPrefix(s, SourceStashPrefix); ok {
		return isStashLabel(rest)
	}
	if rest, ok := strings.CutPrefix(s, SourceCommitPrefix); ok {
		return isHex40(rest)
	}
	return false
}

// isStashLabel reports whether s is a raw stash label of the form
// stash@{N} with a non-empty numeric N.
func isStashLabel(s string) bool {
	rest, ok := strings.CutPrefix(s, "stash@{")
	if !ok {
		return false
	}
	digits, ok := strings.CutSuffix(rest, "}")
	if !ok || digits == "" {
		return false
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// isHex64 reports whether s is 64 lowercase hexadecimal characters.
func isHex64(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

// loadResolutions reads the path-keyed resolutions file (path → chosen
// variant sha256) produced by the human gate.
func loadResolutions(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read resolutions %s: %w", path, err)
	}
	var resolutions map[string]string
	if err := json.Unmarshal(raw, &resolutions); err != nil {
		return nil, fmt.Errorf("parse resolutions: %w", err)
	}
	return resolutions, nil
}

// validateResolutions checks every resolution entry against the plan's
// conflicts: the path must name a real conflict, the sha256 must be one of
// its variants, and a variant carrying blocking findings is never a valid
// resolution (design D7 step 1).
func validateResolutions(plan *Plan, resolutions map[string]string) error {
	keys := make([]string, 0, len(resolutions))
	for k := range resolutions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, path := range keys {
		sha := resolutions[path]
		if !isHex64(sha) {
			return fmt.Errorf("resolution for %s names a sha256 that is not 64-hex", path)
		}
		var conf *Conflict
		for i := range plan.Conflicts {
			if plan.Conflicts[i].Path == path {
				conf = &plan.Conflicts[i]
				break
			}
		}
		if conf == nil {
			return fmt.Errorf("resolution names unknown conflict path %s", path)
		}
		for _, v := range conf.Variants {
			if v.SHA256 != sha {
				continue
			}
			if shaBlocked(v.Findings) {
				return fmt.Errorf("resolution for %s names blocked variant %s — choose a clean variant", path, sha)
			}
			return nil
		}
		return fmt.Errorf("resolution for %s names sha256 %s, which is not a variant of the conflict", path, sha)
	}
	return nil
}

// matchRoots cross-checks the plan's recorded artifact roots against the
// execution-time sanctioned set; a mismatch means the plan is stale relative
// to the repository's current configuration and must be re-run.
func matchRoots(planRoots, sanctioned []string) error {
	a := append([]string(nil), planRoots...)
	b := append([]string(nil), sanctioned...)
	sort.Strings(a)
	sort.Strings(b)
	if len(a) != len(b) {
		return fmt.Errorf("plan artifact roots do not match the sanctioned set — re-run plan")
	}
	for i := range a {
		if a[i] != b[i] {
			return fmt.Errorf("plan artifact roots do not match the sanctioned set — re-run plan")
		}
	}
	return nil
}
