package artifacts

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// physicalPath resolves symlinks so comparisons against git output match:
// t.TempDir() lives under /var/folders (a symlink of /private/var/...) on
// macOS, and git prints physical paths.
func physicalPath(t *testing.T, p string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatalf("resolve symlinks for %q: %v", p, err)
	}
	return resolved
}

func TestRun_SuccessReturnsOutput(t *testing.T) {
	f := newFixtureRepo(t)
	out, err := run(f.root, "rev-parse", "--show-toplevel")
	if err != nil {
		t.Fatalf("run(rev-parse --show-toplevel): %v", err)
	}
	if got := strings.TrimSpace(string(out)); got != physicalPath(t, f.root) {
		t.Errorf("run output = %q, want repo root %q", got, f.root)
	}
}

func TestRun_FailureReturnsGitError(t *testing.T) {
	f := newFixtureRepo(t)
	_, err := run(f.root, "rev-parse", "--verify", "--quiet", "refs/heads/does-not-exist")
	if err == nil {
		t.Fatalf("run with absent ref: expected error, got nil")
	}
	var ge *gitError
	if !errors.As(err, &ge) {
		t.Fatalf("errors.As(*gitError): got %T, want *gitError", err)
	}
	if ge.exitCode != 1 {
		t.Errorf("exitCode = %d, want 1", ge.exitCode)
	}
	if ge.dir != f.root {
		t.Errorf("dir = %q, want %q", ge.dir, f.root)
	}
	if len(ge.args) == 0 || ge.args[0] != "rev-parse" {
		t.Errorf("args = %v, want leading rev-parse", ge.args)
	}
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Errorf("errors.As(*exec.ExitError) through Unwrap chain: got %T", err)
	}
}

func TestRun_ExpectedAbsenceProbeClassification(t *testing.T) {
	f := newFixtureRepo(t)
	_, err := run(f.root, "rev-parse", "--verify", "--quiet", "refs/heads/does-not-exist")
	if err == nil {
		t.Fatalf("expected error for absent ref")
	}
	var ge *gitError
	if !errors.As(err, &ge) {
		t.Fatalf("errors.As(*gitError): got %T", err)
	}
	// Documented classification pattern: exit code 1 from rev-parse
	// --verify --quiet means the ref is absent, not that the command
	// failed operationally.
	if ge.exitCode != 1 {
		t.Fatalf("exitCode = %d, want 1 (expected absence)", ge.exitCode)
	}
	if ge.isNotARepository() {
		t.Errorf("exit-1 probe misclassified as not-a-repository")
	}
}

func TestGitError_IsNotARepositoryClassifier(t *testing.T) {
	nonRepo := t.TempDir()
	_, err := run(nonRepo, "rev-parse", "--show-toplevel")
	if err == nil {
		t.Fatalf("rev-parse in non-repo: expected error, got nil")
	}
	var ge *gitError
	if !errors.As(err, &ge) {
		t.Fatalf("errors.As(*gitError): got %T", err)
	}
	if ge.exitCode != 128 {
		t.Errorf("exitCode = %d, want 128", ge.exitCode)
	}
	if !ge.isNotARepository() {
		t.Errorf("exit-128 not-a-repository failure not classified: %v", err)
	}

	f := newFixtureRepo(t)
	_, err = run(f.root, "rev-parse", "--verify", "--quiet", "refs/heads/absent")
	if err == nil {
		t.Fatalf("expected error for absent ref")
	}
	var probe *gitError
	if !errors.As(err, &probe) {
		t.Fatalf("errors.As(*gitError): got %T", err)
	}
	if probe.isNotARepository() {
		t.Errorf("exit-1 absence probe misclassified as not-a-repository")
	}
}

func TestGitError_ErrorRendering(t *testing.T) {
	cause := errors.New("exit status 128")
	ge := &gitError{
		args:     []string{"status"},
		dir:      "",
		exitCode: 128,
		stderr:   "fatal: not a git repository\n",
		cause:    cause,
	}
	msg := ge.Error()
	for _, want := range []string{"git status", "the working directory", "fatal: not a git repository"} {
		if !strings.Contains(msg, want) {
			t.Errorf("Error() = %q, missing %q", msg, want)
		}
	}
	if ge.Unwrap() != cause {
		t.Errorf("Unwrap() = %v, want recorded cause", ge.Unwrap())
	}

	geDir := &gitError{
		args:     []string{"status"},
		dir:      "/some/dir",
		exitCode: 1,
		cause:    errors.New("exit status 1"),
	}
	if msg := geDir.Error(); !strings.Contains(msg, "(in /some/dir)") {
		t.Errorf("Error() = %q, missing dir rendering", msg)
	}
}

func TestResolveRepoRoot_SuccessFromRootAndSubdirectory(t *testing.T) {
	f := newFixtureRepo(t)
	want := physicalPath(t, f.root)

	got, err := resolveRepoRoot(f.root)
	if err != nil {
		t.Fatalf("resolveRepoRoot(repo root): %v", err)
	}
	if got != want {
		t.Errorf("resolveRepoRoot(repo root) = %q, want %q", got, want)
	}

	sub := f.path("notes/deep")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}
	got, err = resolveRepoRoot(sub)
	if err != nil {
		t.Fatalf("resolveRepoRoot(subdir): %v", err)
	}
	if got != want {
		t.Errorf("resolveRepoRoot(subdir) = %q, want %q", got, want)
	}
}

func TestResolveRepoRoot_EmptyAndRelativeMeanProcessCWD(t *testing.T) {
	f := newFixtureRepo(t)
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd(): %v", err)
	}
	if err := os.Chdir(f.root); err != nil {
		t.Fatalf("chdir to fixture root: %v", err)
	}
	defer func() {
		if err := os.Chdir(orig); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	}()

	want := physicalPath(t, f.root)
	got, err := resolveRepoRoot("")
	if err != nil {
		t.Fatalf("resolveRepoRoot(\"\"): %v", err)
	}
	if got != want {
		t.Errorf("resolveRepoRoot(\"\") = %q, want %q", got, want)
	}
	got, err = resolveRepoRoot(".")
	if err != nil {
		t.Fatalf("resolveRepoRoot(\".\"): %v", err)
	}
	if got != want {
		t.Errorf("resolveRepoRoot(\".\") = %q, want %q", got, want)
	}
}

func TestResolveRepoRoot_FailClosedNonExistent(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	_, err := resolveRepoRoot(missing)
	if err == nil {
		t.Fatalf("resolveRepoRoot(non-existent): expected error, got nil")
	}
	if errors.Is(err, errNotARepository) {
		t.Errorf("Stat failure misclassified as not-a-repository: %v", err)
	}
	if !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("error does not name the offending path: %v", err)
	}
}

func TestResolveRepoRoot_FailClosedNotADirectory(t *testing.T) {
	file := filepath.Join(t.TempDir(), "plain.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("write plain file: %v", err)
	}
	_, err := resolveRepoRoot(file)
	if err == nil {
		t.Fatalf("resolveRepoRoot(file): expected error, got nil")
	}
	if !strings.Contains(err.Error(), "is not a directory") {
		t.Errorf("error does not state not-a-directory: %v", err)
	}
}

func TestResolveRepoRoot_FailClosedNotARepository(t *testing.T) {
	nonRepo := t.TempDir()
	_, err := resolveRepoRoot(nonRepo)
	if err == nil {
		t.Fatalf("resolveRepoRoot(non-repo): expected error, got nil")
	}
	if !errors.Is(err, errNotARepository) {
		t.Fatalf("errors.Is(errNotARepository): got %v", err)
	}
	if !strings.Contains(err.Error(), nonRepo) {
		t.Errorf("error does not name the start directory: %v", err)
	}
}

func TestRemoteNames_ListsConfiguredRemotes(t *testing.T) {
	f := newFixtureRepo(t)

	names, err := remoteNames(f.root)
	if err != nil {
		t.Fatalf("remoteNames: %v", err)
	}
	if len(names) != 1 || names[0] != remoteOrigin {
		t.Fatalf("initial remotes = %v, want [%s]", names, remoteOrigin)
	}

	f.addUpstreamRemote()
	names, err = remoteNames(f.root)
	if err != nil {
		t.Fatalf("remoteNames after upstream: %v", err)
	}
	has := func(want string) bool {
		for _, n := range names {
			if n == want {
				return true
			}
		}
		return false
	}
	if !has(remoteOrigin) || !has(remoteUpstream) {
		t.Errorf("remotes after upstream = %v, want both %s and %s", names, remoteOrigin, remoteUpstream)
	}
}

func TestRequirePublishRemote_Floor(t *testing.T) {
	for _, ok := range [][]string{
		{remoteOrigin},
		{remoteUpstream},
		{remoteUpstream, remoteOrigin},
		{"other", remoteOrigin},
	} {
		if err := requirePublishRemote(ok); err != nil {
			t.Errorf("requirePublishRemote(%v) = %v, want nil", ok, err)
		}
	}

	err := requirePublishRemote([]string{"other"})
	if !errors.Is(err, errNoPublishRemote) {
		t.Fatalf("requirePublishRemote([other]) = %v, want errNoPublishRemote", err)
	}
	if !strings.Contains(err.Error(), "other") {
		t.Errorf("error does not list configured remotes: %v", err)
	}

	err = requirePublishRemote(nil)
	if !errors.Is(err, errNoPublishRemote) {
		t.Fatalf("requirePublishRemote(nil) = %v, want errNoPublishRemote", err)
	}
	if !strings.Contains(err.Error(), "none") {
		t.Errorf("error does not render empty remote list as none: %v", err)
	}
}
