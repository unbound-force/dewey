package artifacts

// fixtures_test.go is the shared fixture harness for the artifacts package's
// test suite: it builds throwaway git repositories with a local bare origin
// and provides helpers to construct every stranded state the sweep
// enumerates (design D3's four git states) plus the zero-mutation capture
// the plan and sync tests assert against. It also carries the harness's own
// self-tests, which pin the git behaviors the enumerators rely on:
//
//   - `git stash push -u -- <path>` removes the emptied directory along
//     with the stashed file, so the write helper re-creates directories.
//   - `git remote set-head origin -a` after `git push -u` is what makes
//     refs/remotes/origin/HEAD resolvable; it stays unset otherwise, which
//     is exactly the gap the base-ref chain's later links cover.
//   - A staged-then-stashed new file is recoverable from the stash commit
//     itself (stash@{N}:<path>), while an untracked-then-stashed file lives
//     only in the untracked third parent (stash@{N}^3:<path>).

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureRepo is a disposable git repository for one test: a working repo in
// a TempDir with a local bare repository wired as `origin`, a pushed base
// branch, and hermetic repo-local git configuration (identity, no signing,
// no hooks, no CRLF translation) so fixture commits never depend on the
// host machine's global git configuration.
type fixtureRepo struct {
	t *testing.T

	// root is the working repository's absolute path.
	root string

	// origin is the absolute path of the bare repository wired as the
	// `origin` remote.
	origin string

	// base is the base branch name, always "main".
	base string
}

// newFixtureRepo builds the canonical fixture: `git init -b main`, hermetic
// repo-local config, a bare origin repository, an initial commit pushed
// with -u, and origin/HEAD set via `git remote set-head origin -a` so the
// base-ref resolution chain's first link resolves.
func newFixtureRepo(t *testing.T) *fixtureRepo {
	t.Helper()
	dir := t.TempDir()
	f := &fixtureRepo{
		t:      t,
		root:   filepath.Join(dir, "work"),
		origin: filepath.Join(dir, "origin.git"),
		base:   "main",
	}
	hooks := filepath.Join(dir, "empty-hooks")
	for _, d := range []string{f.root, hooks} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("fixture: create directory %s: %v", d, err)
		}
	}
	f.git("init", "-b", "main")
	f.git("config", "user.email", "fixture@example.invalid")
	f.git("config", "user.name", "Fixture Tester")
	f.git("config", "commit.gpgsign", "false")
	f.git("config", "core.autocrlf", "false")
	f.git("config", "core.hooksPath", hooks)
	f.git("init", "--bare", "-b", "main", f.origin)
	f.git("remote", "add", "origin", f.origin)
	f.write("README.md", "fixture base\n")
	f.git("add", "--", "README.md")
	f.git("commit", "-m", "fixture: initial base commit")
	f.git("push", "-u", "origin", "main")
	f.git("remote", "set-head", "origin", "-a")
	return f
}

// git runs `git args...` in the working repository, failing the test on any
// error, and returns git's standard output.
func (f *fixtureRepo) git(args ...string) string {
	f.t.Helper()
	out, ok := f.tryGit(args...)
	if !ok {
		f.t.Fatalf("fixture git %v: failed", args)
	}
	return out
}

// tryGit runs `git args...` in the working repository and reports whether it
// succeeded, for probes whose failure is the expected result (an absent ref,
// content absent from a stash part).
func (f *fixtureRepo) tryGit(args ...string) (string, bool) {
	f.t.Helper()
	out, err := run(f.root, args...)
	return string(out), err == nil
}

// gitIn runs `git args...` in an arbitrary directory (a sync worktree, the
// bare origin), failing the test on any error.
func (f *fixtureRepo) gitIn(dir string, args ...string) string {
	f.t.Helper()
	out, err := run(dir, args...)
	if err != nil {
		f.t.Fatalf("fixture git (in %s) %v: %v", dir, args, err)
	}
	return string(out)
}

// write writes content at the repository-relative path rel, creating parent
// directories as needed. Directories removed by `git stash push -u` are
// re-created here, which is why every stranded-state helper routes through
// this method.
func (f *fixtureRepo) write(rel, content string) {
	f.t.Helper()
	abs := filepath.Join(f.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		f.t.Fatalf("fixture: create %s: %v", filepath.Dir(abs), err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		f.t.Fatalf("fixture: write %s: %v", abs, err)
	}
}

// path returns the absolute path for the repository-relative path rel.
func (f *fixtureRepo) path(rel string) string {
	return filepath.Join(f.root, filepath.FromSlash(rel))
}

// stageArtifact writes and stages an artifact file: the working-tree staged
// state.
func (f *fixtureRepo) stageArtifact(rel, content string) {
	f.t.Helper()
	f.write(rel, content)
	f.git("add", "--", rel)
}

// stashTrackedArtifact strands content in a stash's tracked part: the file
// is written and staged, then stashed with `git stash push -- <rel>`, which
// removes it from the index and the working tree. The content is recoverable
// from the stash commit itself (stash@{N}:<rel>).
func (f *fixtureRepo) stashTrackedArtifact(rel, content string) {
	f.t.Helper()
	f.stageArtifact(rel, content)
	f.git("stash", "push", "--", rel)
}

// stashUntrackedArtifact strands content in a stash's untracked part: the
// file is written untracked, then stashed with `git stash push -u -- <rel>`.
// The content lives only in the stash's third parent (stash@{N}^3:<rel>),
// and the emptied directory is removed from the working tree.
func (f *fixtureRepo) stashUntrackedArtifact(rel, content string) {
	f.t.Helper()
	f.write(rel, content)
	f.git("stash", "push", "-u", "--", rel)
}

// commitArtifactOnBranch strands content in an unpushed commit: a new branch
// is created from the base branch, the artifact is committed to it, and the
// working repository returns to the base branch. The commit stays local:
// `git rev-list --branches --not --remotes` reports it.
func (f *fixtureRepo) commitArtifactOnBranch(branch, rel, content, message string) {
	f.t.Helper()
	f.git("checkout", "-b", branch)
	f.write(rel, content)
	f.git("add", "--", rel)
	f.git("commit", "-m", message)
	f.git("checkout", f.base)
}

// enableBareDeweyIgnore commits the bare `dewey` gitignore pattern to the
// base branch and pushes it, making .uf/dewey/** ignored: the working-tree
// ignored state, and the gitignore-bug fixture of designs D8 and D10.
func (f *fixtureRepo) enableBareDeweyIgnore() {
	f.t.Helper()
	f.write(".gitignore", "dewey\n")
	f.git("add", "--", ".gitignore")
	f.git("commit", "-m", "fixture: bare dewey ignore pattern")
	f.git("push", "origin", f.base)
}

// addUpstreamRemote wires a second remote named `upstream` to the bare
// origin repository and fetches it: the fork layout of design D4, where
// `upstream` is the publish remote and `origin` the push remote.
// upstream/HEAD is deliberately left unset so the base-ref chain resolves
// through its upstream/main link.
func (f *fixtureRepo) addUpstreamRemote() {
	f.t.Helper()
	f.git("remote", "add", "upstream", f.origin)
	f.git("fetch", "upstream")
}

// fixtureState is a zero-mutation snapshot of the working repository: the
// `git status --porcelain` and `git stash list` outputs.
type fixtureState struct {
	status  string
	stashes string
}

// capture snapshots the working repository's status and stash stack.
func (f *fixtureRepo) capture() fixtureState {
	f.t.Helper()
	return fixtureState{
		status:  f.git("status", "--porcelain"),
		stashes: f.git("stash", "list"),
	}
}

// assertUnchanged fails the test unless the repository's current state
// equals the captured before state: the zero-mutation property every plan
// test asserts after enumeration.
func (f *fixtureRepo) assertUnchanged(before fixtureState) {
	f.t.Helper()
	after := f.capture()
	if after != before {
		f.t.Fatalf("repository state changed:\nbefore:\n%s\n%s\nafter:\n%s\n%s",
			before.status, before.stashes, after.status, after.stashes)
	}
}

// fixtureSHA256 returns the lowercase hex sha256 of b: the digest plan
// tests expect in artifact and variant entries.
func fixtureSHA256(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// --- Harness self-tests ---

// TestFixtureRepo_BaseGeometry verifies the canonical fixture's geometry:
// origin/HEAD resolves, the base branch is pushed, and the tree is clean.
func TestFixtureRepo_BaseGeometry(t *testing.T) {
	f := newFixtureRepo(t)
	if got, want := strings.TrimSpace(f.git("symbolic-ref", "--short", "refs/remotes/origin/HEAD")), "origin/main"; got != want {
		t.Errorf("origin/HEAD = %q, want %q", got, want)
	}
	head := strings.TrimSpace(f.git("rev-parse", "HEAD"))
	originMain := strings.TrimSpace(f.git("rev-parse", "origin/main"))
	if head != originMain {
		t.Errorf("origin/main (%s) != HEAD (%s): base branch not pushed", originMain, head)
	}
	if status := f.git("status", "--porcelain"); status != "" {
		t.Errorf("base fixture not clean:\n%s", status)
	}
}

// TestFixtureRepo_WorkingTreeStates verifies the untracked and staged
// stranded states show up in `git status --porcelain` as the enumerators
// expect.
func TestFixtureRepo_WorkingTreeStates(t *testing.T) {
	f := newFixtureRepo(t)
	f.write(".uf/dewey/learnings/auth-20260101T000000Z-tester.md", "untracked\n")
	f.stageArtifact(".uf/dewey/learnings/git-20260101T000001Z-tester.md", "staged\n")
	status := f.git("status", "--porcelain")
	for _, want := range []string{
		"?? .uf/dewey/learnings/auth-20260101T000000Z-tester.md\n",
		"A  .uf/dewey/learnings/git-20260101T000001Z-tester.md\n",
	} {
		if !strings.Contains(status, want) {
			t.Errorf("status --porcelain missing %q:\n%s", want, status)
		}
	}
}

// TestFixtureRepo_IgnoredState verifies the bare `dewey` pattern makes
// artifact paths ignored: absent from status --porcelain, classified by
// check-ignore as the bare pattern from the in-repo .gitignore.
func TestFixtureRepo_IgnoredState(t *testing.T) {
	f := newFixtureRepo(t)
	f.enableBareDeweyIgnore()
	rel := ".uf/dewey/learnings/auth-20260101T000000Z-tester.md"
	f.write(rel, "ignored\n")
	if status := f.git("status", "--porcelain"); strings.Contains(status, rel) {
		t.Errorf("ignored artifact appears in status --porcelain:\n%s", status)
	}
	out := f.git("check-ignore", "-v", rel)
	if !strings.Contains(out, "dewey") || !strings.Contains(out, ".gitignore") {
		t.Errorf("check-ignore -v = %q, want the bare dewey pattern from .gitignore", out)
	}
}

// TestFixtureRepo_StashTrackedPart verifies the stash tracked-part state:
// the file leaves the working tree, no ^3 untracked commit exists, and the
// content is recoverable from the stash commit itself.
func TestFixtureRepo_StashTrackedPart(t *testing.T) {
	f := newFixtureRepo(t)
	rel := ".uf/dewey/learnings/auth-20260101T000000Z-tester.md"
	f.stashTrackedArtifact(rel, "stashed-tracked\n")
	if got := strings.TrimSpace(f.git("stash", "list")); !strings.HasPrefix(got, "stash@{0}") {
		t.Fatalf("stash list = %q, want one stash entry", got)
	}
	if _, err := os.Stat(f.path(rel)); !os.IsNotExist(err) {
		t.Errorf("stashed tracked file still in working tree: %v", err)
	}
	if _, ok := f.tryGit("rev-parse", "--verify", "--quiet", "stash@{0}^3"); ok {
		t.Errorf("tracked-part stash unexpectedly has a ^3 untracked commit")
	}
	if got, want := f.git("show", "stash@{0}:"+rel), "stashed-tracked\n"; got != want {
		t.Errorf("stash@{0}:%s = %q, want %q", rel, got, want)
	}
}

// TestFixtureRepo_StashUntrackedPart verifies the stash untracked-part
// state: the file and its emptied directory leave the working tree, the ^3
// commit holds the content, the tracked part does not, and the write helper
// re-creates removed directories.
func TestFixtureRepo_StashUntrackedPart(t *testing.T) {
	f := newFixtureRepo(t)
	rel := ".uf/dewey/learnings/auth-20260101T000000Z-tester.md"
	f.stashUntrackedArtifact(rel, "stashed-untracked\n")
	if _, err := os.Stat(f.path(rel)); !os.IsNotExist(err) {
		t.Errorf("stashed untracked file still in working tree: %v", err)
	}
	if _, err := os.Stat(f.path(".uf/dewey/learnings")); !os.IsNotExist(err) {
		t.Errorf("emptied learnings directory still present after stash -u")
	}
	if _, ok := f.tryGit("rev-parse", "--verify", "--quiet", "stash@{0}^3"); !ok {
		t.Fatalf("untracked-part stash has no ^3 commit")
	}
	if got, want := f.git("show", "stash@{0}^3:"+rel), "stashed-untracked\n"; got != want {
		t.Errorf("stash@{0}^3:%s = %q, want %q", rel, got, want)
	}
	if _, ok := f.tryGit("show", "stash@{0}:"+rel); ok {
		t.Errorf("untracked content unexpectedly present in the stash's tracked part")
	}
	next := ".uf/dewey/learnings/git-20260101T000001Z-tester.md"
	f.write(next, "after\n")
	if _, err := os.Stat(f.path(next)); err != nil {
		t.Errorf("write did not re-create the removed directory: %v", err)
	}
}

// TestFixtureRepo_UnpushedCommit verifies the unpushed-commit state: exactly
// one local-only commit, carrying the artifact in its tree, with the working
// tree clean after returning to the base branch.
func TestFixtureRepo_UnpushedCommit(t *testing.T) {
	f := newFixtureRepo(t)
	rel := ".uf/dewey/learnings/auth-20260101T000000Z-tester.md"
	f.commitArtifactOnBranch("feature", rel, "unpushed\n", "fixture: unpushed artifact")
	out := f.git("rev-list", "--branches", "--not", "--remotes")
	lines := strings.Fields(strings.TrimSpace(out))
	if len(lines) != 1 {
		t.Fatalf("rev-list --branches --not --remotes = %v, want exactly one unpushed commit", lines)
	}
	tree := f.git("ls-tree", "-r", "--name-only", lines[0])
	if !strings.Contains(tree, rel) {
		t.Errorf("unpushed commit tree missing %s:\n%s", rel, tree)
	}
	if status := f.git("status", "--porcelain"); status != "" {
		t.Errorf("working tree not clean after branch return:\n%s", status)
	}
}

// TestFixtureRepo_ZeroMutationCapture verifies the capture helpers observe
// repository state faithfully: no change across a read-only command, and a
// detected difference after an actual mutation.
func TestFixtureRepo_ZeroMutationCapture(t *testing.T) {
	f := newFixtureRepo(t)
	before := f.capture()
	f.git("rev-parse", "HEAD")
	f.assertUnchanged(before)
	f.write(".uf/dewey/learnings/auth-20260101T000000Z-tester.md", "untracked\n")
	mutated := f.capture()
	if mutated == before {
		t.Fatalf("capture failed to observe an untracked file")
	}
	f.assertUnchanged(mutated)
}
