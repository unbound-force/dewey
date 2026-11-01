package artifacts

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// gitBinary is the executable every git subprocess in this package runs,
// resolved from the caller's PATH. The sweep speaks to git exclusively
// through subprocesses (design D2: pure git-subprocess core — no libgit2
// bindings, no CGO), matching the exec.Command("git", ...) precedent in
// tools/learning.go.
const gitBinary = "git"

// The remote names the publish-remote policy recognizes (design D4):
// `upstream` is the publish remote when configured (the fork layout this
// repository itself uses), else `origin`.
const (
	// remoteUpstream is the fork-layout publish remote, preferred when
	// configured.
	remoteUpstream = "upstream"

	// remoteOrigin is the default publish remote, and the push remote in
	// fork layouts.
	remoteOrigin = "origin"
)

// Fail-closed sentinels for the repository-geometry preconditions of the
// sweep (design D3: fail closed when the start directory is not inside a
// git repository, or when neither an `upstream` nor an `origin` remote
// exists). They are unexported deliberately, like every mechanical symbol
// in this package outside the JSON contract (types.go): callers and tests
// classify them through the wrapped chain with errors.Is.
var (
	// errNotARepository marks repo-root resolution attempted from a
	// directory that is not inside a git repository.
	errNotARepository = errors.New("not a git repository")

	// errNoPublishRemote marks a repository that has neither an `upstream`
	// nor an `origin` remote, leaving the sweep no publish destination.
	errNoPublishRemote = errors.New("no publish remote configured (add one with `git remote add <name> <url>`)")
)

// run executes the git subprocess `git args...` with repo as its working
// directory and returns git's standard output. It is the single subprocess
// seam of the sweep (design D2): enumeration, remote resolution, and the
// sync choreography all go through it, so error wrapping, PATH resolution,
// and working-directory handling live in one place.
//
// repo is the working directory the subprocess runs in: after repo-root
// resolution it is always the repository root (design D3 — all commands run
// with the repo root resolved via `git rev-parse --show-toplevel`), while
// resolution itself runs from the vault root; the empty string means the
// process working directory.
//
// A failing subprocess returns a *gitError carrying the invoked arguments,
// the working directory, git's exit status, and git's standard error, and
// wrapping the underlying exec error so errors.Is and errors.As reach
// *exec.ExitError and exec.ErrNotFound through the chain. Commands whose
// non-zero exits are expected probe results — `git rev-parse --verify
// --quiet <ref>` (absent ref: exit 1) and `git check-ignore <path>` (not
// ignored: exit 1) — classify them via errors.As:
//
//	var ge *gitError
//	if errors.As(err, &ge) && ge.exitCode == 1 {
//		// expected absence, not a failure
//	}
//
// run never invokes a shell: arguments reach exec.Command verbatim, so
// paths and repository content are never interpreted as shell syntax.
func run(repo string, args ...string) ([]byte, error) {
	cmd := exec.Command(gitBinary, args...)
	cmd.Dir = repo
	cmd.Env = scrubGitEnv()
	out, err := cmd.Output()
	if err != nil {
		ge := &gitError{args: args, dir: repo, cause: err, exitCode: -1}
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			ge.exitCode = ee.ExitCode()
			ge.stderr = string(ee.Stderr)
		}
		return nil, ge
	}
	return out, nil
}

// gitError is the error type run returns for every failed git subprocess: a
// non-zero exit, or a failure to start the executable at all. run only
// constructs it on failure, so cause is always non-nil — Unwrap reaches the
// underlying *exec.ExitError (non-zero exit) or *exec.Error (executable not
// found, itself wrapping exec.ErrNotFound).
type gitError struct {
	// args are the git arguments the failed subprocess was invoked with.
	args []string

	// dir is the working directory the subprocess ran in; the empty string
	// means the process working directory.
	dir string

	// exitCode is git's exit status, or -1 when git did not run to
	// completion (start failure or death by signal).
	exitCode int

	// stderr is git's standard error, empty when git never ran.
	stderr string

	// cause is the underlying exec error; always non-nil by construction.
	cause error
}

// Error renders the failed invocation with its working directory, exit
// status, and git's standard error, so every error surfacing at the command
// layer is actionable without re-running the command.
func (e *gitError) Error() string {
	where := e.dir
	if where == "" {
		where = "the working directory"
	}
	msg := fmt.Sprintf("git %s (in %s): %s", strings.Join(e.args, " "), where, e.cause)
	if s := strings.TrimSpace(e.stderr); s != "" {
		msg += ": " + s
	}
	return msg
}

// Unwrap exposes the underlying exec error for errors.Is and errors.As.
func (e *gitError) Unwrap() error { return e.cause }

// isNotARepository reports whether the failure is git's canonical
// not-a-repository condition: exit status 128 with the fatal message git
// has printed for it across every modern version. git itself stays the
// authority on repository-hood — no filesystem heuristic could match
// worktrees, submodules, GIT_DIR, or .git files — so resolveRepoRoot
// classifies this condition as errNotARepository rather than surfacing a
// generic subprocess failure.
func (e *gitError) isNotARepository() bool {
	return e.exitCode == 128 && strings.Contains(e.stderr, "not a git repository")
}

// resolveRepoRoot resolves the absolute path of the git repository root the
// sweep operates on, via `git rev-parse --show-toplevel` (design D3) run
// from vaultPath: the --vault flag value as the command layer passes it
// through. The flag itself — and this codebase's OBSIDIAN_VAULT_PATH → cwd
// fallback chain (main.go's resolveVaultPathOrCwd, the single entry point
// for vault resolution) — lives in the CLI layer, not here; an empty
// vaultPath means the process working directory.
//
// Resolution runs from the vault root rather than the incidental process
// working directory because Dewey anchors its artifact trio at
// <vault>/.uf/dewey/ (tools/learning.go), which diverges from the
// repository root when the vault is a subdirectory: the repository
// containing the vault is the sweep's universe, and a vault in a repository
// subdirectory still resolves that repository's root.
//
// Fail-closed behavior: a relative vaultPath is made absolute first; a
// vaultPath that does not name an existing directory fails closed with an
// error naming it; a start directory outside any git repository fails
// closed wrapping errNotARepository. Whether the vault root remains inside
// the resolved repository once symlinks are resolved is a separate
// physical-containment concern, verified by the working-tree enumerator's
// walk-root checks (design D3, enumeration mechanics); this function
// resolves the root git itself reports.
func resolveRepoRoot(vaultPath string) (string, error) {
	start := vaultPath
	if start == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve repository root: determine working directory: %w", err)
		}
		start = cwd
	} else {
		abs, err := filepath.Abs(start)
		if err != nil {
			return "", fmt.Errorf("resolve repository root: resolve vault root %q: %w", start, err)
		}
		start = abs
	}
	if info, err := os.Stat(start); err != nil {
		return "", fmt.Errorf("resolve repository root: vault root %q: %w", start, err)
	} else if !info.IsDir() {
		return "", fmt.Errorf("resolve repository root: vault root %q is not a directory", start)
	}
	out, err := run(start, "rev-parse", "--show-toplevel")
	if err != nil {
		var ge *gitError
		if errors.As(err, &ge) && ge.isNotARepository() {
			return "", fmt.Errorf("resolve repository root from %s: %w", start, errNotARepository)
		}
		return "", fmt.Errorf("resolve repository root from %s: %w", start, err)
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return "", fmt.Errorf("resolve repository root from %s: git rev-parse --show-toplevel printed no root", start)
	}
	return root, nil
}

// ResolveRepoRoot is the exported seam over resolveRepoRoot for command
// layers that need the same fail-closed repository resolution the sweep uses
// (not-a-repository classification included) without re-implementing the
// raw subprocess call.
func ResolveRepoRoot(vaultPath string) (string, error) {
	return resolveRepoRoot(vaultPath)
}

// gitEnvOverrides are the environment variables that silently redirect git's
// core repository geometry — which object store, index, and work tree a
// command operates on. The sweep scrubs them so a caller nested inside
// another git operation (a hook, a worktree script) cannot accidentally
// redirect every sweep subprocess at a repository other than the one resolved
// from the vault path.
var gitEnvOverrides = []string{
	"GIT_DIR",
	"GIT_WORK_TREE",
	"GIT_INDEX_FILE",
	"GIT_OBJECT_DIRECTORY",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES",
	"GIT_COMMON_DIR",
	"GIT_NAMESPACE",
}

// scrubGitEnv returns the current environment minus the git geometry
// overrides.
func scrubGitEnv() []string {
	drop := make(map[string]bool, len(gitEnvOverrides))
	for _, name := range gitEnvOverrides {
		drop[name] = true
	}
	env := os.Environ()
	filtered := make([]string, 0, len(env))
	for _, kv := range env {
		if name, _, _ := strings.Cut(kv, "="); drop[name] {
			continue
		}
		filtered = append(filtered, kv)
	}
	return filtered
}

// remoteNames lists the names of the repository's configured git remotes,
// via one `git remote` subprocess. The publish-remote policy (design D4:
// `upstream` when configured, else `origin`) resolves over this list, so
// remote existence costs a single subprocess per sweep and the dedupe
// stage composes the full policy without re-querying git.
func remoteNames(repo string) ([]string, error) {
	out, err := run(repo, "remote")
	if err != nil {
		return nil, fmt.Errorf("list git remotes: %w", err)
	}
	var names []string
	for _, line := range strings.Split(string(out), "\n") {
		if name := strings.TrimSpace(line); name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

// requirePublishRemote enforces the publish-remote floor (design D3): the
// sweep fails closed when the repository has neither an `upstream` nor an
// `origin` remote, because no publish destination can be resolved. The
// error names both recognized remotes alongside the configured ones, and
// wraps errNoPublishRemote. The full publish/push policy — the fork-layout
// rule that an `origin` must accompany an `upstream` publish remote, and
// the push-remote derivation — layers on the remote list in the dedupe
// stage (design D4), not here.
func requirePublishRemote(names []string) error {
	for _, name := range names {
		if name == remoteUpstream || name == remoteOrigin {
			return nil
		}
	}
	found := "none"
	if len(names) > 0 {
		found = strings.Join(names, ", ")
	}
	return fmt.Errorf("resolve publish remote: neither %q nor %q is configured (configured remotes: %s): %w",
		remoteUpstream, remoteOrigin, found, errNoPublishRemote)
}
