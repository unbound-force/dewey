package gitref

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// testSHA is a syntactically valid 40-hex object ID for scripted responses.
const testSHA = "a94a8fe5ccb19ba61c4c0873d391e987982fbbd3"

// fakeResponse is one scripted Runner answer.
type fakeResponse struct {
	stdout string
	stderr string
	code   int
	err    error
}

// fakeRunner is a scripted Runner: it records every invocation and replays
// responses in order. A call with no scripted response fails the test's
// expectation by returning an error, which the resolver classifies as a git
// failure.
type fakeRunner struct {
	calls     [][]string
	dirs      []string
	responses []fakeResponse
}

func (f *fakeRunner) Run(ctx context.Context, dir string, args []string) (string, string, int, error) {
	f.calls = append(f.calls, args)
	f.dirs = append(f.dirs, dir)
	if len(f.responses) == 0 {
		return "", "", -1, fmt.Errorf("fakeRunner: unexpected call %v", args)
	}
	resp := f.responses[0]
	f.responses = f.responses[1:]
	return resp.stdout, resp.stderr, resp.code, resp.err
}

// mapEnv builds an injectable environment lookup.
func mapEnv(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// hungRunner simulates a git process that never responds: it blocks until
// the context is done and then surfaces the context error, matching what
// ExecRunner returns when exec.CommandContext kills the child at the
// deadline.
type hungRunner struct{}

func (hungRunner) Run(ctx context.Context, dir string, args []string) (string, string, int, error) {
	<-ctx.Done()
	return "", "", -1, ctx.Err()
}

// writeVaultConfig writes a per-vault config.yaml with the given
// default_branch_ref value.
func writeVaultConfig(t *testing.T, vaultDir, value string) {
	t.Helper()
	dir := filepath.Join(vaultDir, ".uf", "dewey")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	cfg := ConfigFileKey + ": " + value + "\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// Expected argv sequences, asserted exactly so the invocation shape (argv
// arrays, separators, fully qualified refs) is pinned by tests.
var (
	callConfigGet = []string{"config", "--get", GitConfigKey}
	callProbe     = []string{"rev-parse", "--git-dir"}
)

func callVerifyRef(fullRef string) []string {
	return []string{"rev-parse", "--verify", "--quiet", "--end-of-options", fullRef}
}

// TestResolve_ConfigPrecedenceAndFailureClassification covers the IP-FR-004
// precedence chain (which leg supplies the effective value) and the
// GTS-FR-004 failure classification (invalid config rejected before git,
// not-a-repo, missing ref, git failure, malformed output).
func TestResolve_ConfigPrecedenceAndFailureClassification(t *testing.T) {
	cases := []struct {
		name string
		// setup builds the Options (writing any config fixtures) and the
		// scripted runner.
		setup func(t *testing.T) (Options, *fakeRunner)
		// wantCalls is the exact expected argv sequence; nil means no git
		// invocation may happen at all (rejected before any git).
		wantCalls [][]string
		want      RefResolution
		// wantReasonContains is asserted against Reason when non-empty.
		wantReasonContains string
	}{
		{
			name: "vault config leg wins and resolves",
			setup: func(t *testing.T) (Options, *fakeRunner) {
				vaultDir := t.TempDir()
				writeVaultConfig(t, vaultDir, "upstream/main")
				runner := &fakeRunner{responses: []fakeResponse{
					{stdout: ".git\n"},       // rev-parse --git-dir
					{stdout: testSHA + "\n"}, // rev-parse --verify upstream/main
				}}
				return Options{VaultDir: vaultDir, Runner: runner, Env: mapEnv(nil)}, runner
			},
			wantCalls: [][]string{callProbe, callVerifyRef("refs/remotes/upstream/main")},
			want: RefResolution{
				Ref: "upstream/main", FullRef: "refs/remotes/upstream/main",
				SHA: testSHA, Status: StatusResolved, Leg: LegVaultConfig,
			},
		},
		{
			name: "global config leg",
			setup: func(t *testing.T) (Options, *fakeRunner) {
				vaultDir := t.TempDir() // no .uf/dewey here
				global := t.TempDir()
				if err := os.MkdirAll(filepath.Join(global, "dewey"), 0o755); err != nil {
					t.Fatalf("MkdirAll: %v", err)
				}
				cfg := ConfigFileKey + ": upstream/main\n"
				if err := os.WriteFile(filepath.Join(global, "dewey", "config.yaml"), []byte(cfg), 0o644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
				runner := &fakeRunner{responses: []fakeResponse{
					{stdout: ".git\n"},
					{stdout: testSHA + "\n"},
				}}
				env := mapEnv(map[string]string{"XDG_CONFIG_HOME": global})
				return Options{VaultDir: vaultDir, Runner: runner, Env: env}, runner
			},
			wantCalls: [][]string{callProbe, callVerifyRef("refs/remotes/upstream/main")},
			want: RefResolution{
				Ref: "upstream/main", FullRef: "refs/remotes/upstream/main",
				SHA: testSHA, Status: StatusResolved, Leg: LegGlobalConfig,
			},
		},
		{
			name: "env leg",
			setup: func(t *testing.T) (Options, *fakeRunner) {
				runner := &fakeRunner{responses: []fakeResponse{
					{stdout: ".git\n"},
					{stdout: testSHA + "\n"},
				}}
				env := mapEnv(map[string]string{EnvDefaultBranchRef: "upstream/main"})
				return Options{VaultDir: t.TempDir(), Runner: runner, Env: env}, runner
			},
			wantCalls: [][]string{callProbe, callVerifyRef("refs/remotes/upstream/main")},
			want: RefResolution{
				Ref: "upstream/main", FullRef: "refs/remotes/upstream/main",
				SHA: testSHA, Status: StatusResolved, Leg: LegEnv,
			},
		},
		{
			name: "blank env value is unset, not provided",
			setup: func(t *testing.T) (Options, *fakeRunner) {
				runner := &fakeRunner{responses: []fakeResponse{
					{code: 1},                // git config --get: unset
					{stdout: ".git\n"},       // probe
					{stdout: testSHA + "\n"}, // verify origin/main
				}}
				env := mapEnv(map[string]string{EnvDefaultBranchRef: "   "})
				return Options{VaultDir: t.TempDir(), Runner: runner, Env: env}, runner
			},
			wantCalls: [][]string{callConfigGet, callProbe, callVerifyRef("refs/remotes/origin/main")},
			want: RefResolution{
				Ref: "origin/main", FullRef: "refs/remotes/origin/main",
				SHA: testSHA, Status: StatusResolved, Leg: LegDefault,
			},
		},
		{
			name: "git config leg",
			setup: func(t *testing.T) (Options, *fakeRunner) {
				runner := &fakeRunner{responses: []fakeResponse{
					{stdout: "upstream/main\n"}, // git config --get
					{stdout: ".git\n"},
					{stdout: testSHA + "\n"},
				}}
				return Options{VaultDir: t.TempDir(), Runner: runner, Env: mapEnv(nil)}, runner
			},
			wantCalls: [][]string{callConfigGet, callProbe, callVerifyRef("refs/remotes/upstream/main")},
			want: RefResolution{
				Ref: "upstream/main", FullRef: "refs/remotes/upstream/main",
				SHA: testSHA, Status: StatusResolved, Leg: LegGitConfig,
			},
		},
		{
			name: "default leg origin/main",
			setup: func(t *testing.T) (Options, *fakeRunner) {
				runner := &fakeRunner{responses: []fakeResponse{
					{code: 1},
					{stdout: ".git\n"},
					{stdout: testSHA + "\n"},
				}}
				return Options{VaultDir: t.TempDir(), Runner: runner, Env: mapEnv(nil)}, runner
			},
			wantCalls: [][]string{callConfigGet, callProbe, callVerifyRef("refs/remotes/origin/main")},
			want: RefResolution{
				Ref: "origin/main", FullRef: "refs/remotes/origin/main",
				SHA: testSHA, Status: StatusResolved, Leg: LegDefault,
			},
		},
		{
			name: "invalid vault config rejected before any git invocation",
			setup: func(t *testing.T) (Options, *fakeRunner) {
				vaultDir := t.TempDir()
				writeVaultConfig(t, vaultDir, "--upload-pack=evil")
				runner := &fakeRunner{}
				return Options{VaultDir: vaultDir, Runner: runner, Env: mapEnv(nil)}, runner
			},
			wantCalls: nil,
			want: RefResolution{
				Ref: "--upload-pack=evil", Status: StatusInvalidConfig, Leg: LegVaultConfig,
			},
			wantReasonContains: "--upload-pack=evil",
		},
		{
			name: "invalid env value rejected before any git invocation",
			setup: func(t *testing.T) (Options, *fakeRunner) {
				runner := &fakeRunner{}
				env := mapEnv(map[string]string{EnvDefaultBranchRef: "origin main"})
				return Options{VaultDir: t.TempDir(), Runner: runner, Env: env}, runner
			},
			wantCalls: nil,
			want: RefResolution{
				Ref: "origin main", Status: StatusInvalidConfig, Leg: LegEnv,
			},
			wantReasonContains: "origin main",
		},
		{
			name: "non-git vault",
			setup: func(t *testing.T) (Options, *fakeRunner) {
				runner := &fakeRunner{responses: []fakeResponse{
					{code: 1},
					{code: 128, stderr: "fatal: not a git repository (or any of the parent directories): .git\n"},
				}}
				return Options{VaultDir: t.TempDir(), Runner: runner, Env: mapEnv(nil)}, runner
			},
			wantCalls: [][]string{callConfigGet, callProbe},
			want: RefResolution{
				Ref: "origin/main", FullRef: "refs/remotes/origin/main",
				Status: StatusNotARepo, Leg: LegDefault,
			},
			wantReasonContains: "not a git repository",
		},
		{
			name: "missing ref returns resolvable-missing signal",
			setup: func(t *testing.T) (Options, *fakeRunner) {
				runner := &fakeRunner{responses: []fakeResponse{
					{code: 1},
					{stdout: ".git\n"},
					{code: 1}, // rev-parse --verify --quiet: ref not found
				}}
				return Options{VaultDir: t.TempDir(), Runner: runner, Env: mapEnv(nil)}, runner
			},
			wantCalls: [][]string{callConfigGet, callProbe, callVerifyRef("refs/remotes/origin/main")},
			want: RefResolution{
				Ref: "origin/main", FullRef: "refs/remotes/origin/main",
				Status: StatusMissingRef, Leg: LegDefault,
			},
			wantReasonContains: "git fetch origin",
		},
		{
			name: "unexpected probe exit is a git failure",
			setup: func(t *testing.T) (Options, *fakeRunner) {
				runner := &fakeRunner{responses: []fakeResponse{
					{code: 1},
					{code: 128, stderr: "fatal: unable to access '/x/.git/objects/': Permission denied\n"},
				}}
				return Options{VaultDir: t.TempDir(), Runner: runner, Env: mapEnv(nil)}, runner
			},
			wantCalls: [][]string{callConfigGet, callProbe},
			want: RefResolution{
				Ref: "origin/main", FullRef: "refs/remotes/origin/main",
				Status: StatusGitFailed, Leg: LegDefault,
			},
			wantReasonContains: "Permission denied",
		},
		{
			name: "missing git binary is a git failure",
			setup: func(t *testing.T) (Options, *fakeRunner) {
				runner := &fakeRunner{responses: []fakeResponse{
					{code: 1},
					{err: errors.New(`exec: "git": executable file not found in $PATH`)},
				}}
				return Options{VaultDir: t.TempDir(), Runner: runner, Env: mapEnv(nil)}, runner
			},
			wantCalls: [][]string{callConfigGet, callProbe},
			want: RefResolution{
				Ref: "origin/main", FullRef: "refs/remotes/origin/main",
				Status: StatusGitFailed, Leg: LegDefault,
			},
			wantReasonContains: "executable file not found",
		},
		{
			name: "malformed rev-parse output is a git failure",
			setup: func(t *testing.T) (Options, *fakeRunner) {
				runner := &fakeRunner{responses: []fakeResponse{
					{code: 1},
					{stdout: ".git\n"},
					{stdout: "not-a-sha\n"}, // exit 0 but not a 40-hex object ID
				}}
				return Options{VaultDir: t.TempDir(), Runner: runner, Env: mapEnv(nil)}, runner
			},
			wantCalls: [][]string{callConfigGet, callProbe, callVerifyRef("refs/remotes/origin/main")},
			want: RefResolution{
				Ref: "origin/main", FullRef: "refs/remotes/origin/main",
				Status: StatusGitFailed, Leg: LegDefault,
			},
			wantReasonContains: "malformed",
		},
		{
			name: "empty vault dir is a git failure",
			setup: func(t *testing.T) (Options, *fakeRunner) {
				runner := &fakeRunner{}
				return Options{VaultDir: "", Runner: runner, Env: mapEnv(nil)}, runner
			},
			wantCalls: nil,
			want: RefResolution{
				Ref: "origin/main", Status: StatusGitFailed, Leg: LegDefault,
			},
			wantReasonContains: "no vault directory",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts, runner := tc.setup(t)
			res := New(opts).Resolve(context.Background())

			if res.Status != tc.want.Status {
				t.Errorf("Status = %q, want %q", res.Status, tc.want.Status)
			}
			if res.Leg != tc.want.Leg {
				t.Errorf("Leg = %q, want %q", res.Leg, tc.want.Leg)
			}
			if res.Ref != tc.want.Ref {
				t.Errorf("Ref = %q, want %q", res.Ref, tc.want.Ref)
			}
			if res.FullRef != tc.want.FullRef {
				t.Errorf("FullRef = %q, want %q", res.FullRef, tc.want.FullRef)
			}
			if res.SHA != tc.want.SHA {
				t.Errorf("SHA = %q, want %q", res.SHA, tc.want.SHA)
			}
			if res.Resolved() != (tc.want.Status == StatusResolved) {
				t.Errorf("Resolved() = %v, want %v", res.Resolved(), tc.want.Status == StatusResolved)
			}
			if tc.wantReasonContains != "" && !strings.Contains(res.Reason, tc.wantReasonContains) {
				t.Errorf("Reason = %q, want it to contain %q", res.Reason, tc.wantReasonContains)
			}
			if tc.wantCalls == nil {
				if len(runner.calls) != 0 {
					t.Errorf("git was invoked %d time(s) despite expectation of zero calls: %v", len(runner.calls), runner.calls)
				}
				return
			}
			if !reflect.DeepEqual(runner.calls, tc.wantCalls) {
				t.Errorf("git calls = %v, want %v", runner.calls, tc.wantCalls)
			}
			if len(runner.dirs) > 0 && runner.dirs[0] != opts.VaultDir {
				t.Errorf("git ran in %q, want %q", runner.dirs[0], opts.VaultDir)
			}
		})
	}
}

// TestResolve_HungGitIsBounded verifies the per-invocation timeout: a git
// process that never responds is abandoned at the deadline with a
// context-deadline diagnostic, classified as a git failure (GTS-FR-004), and
// never stalls the resolution.
func TestResolve_HungGitIsBounded(t *testing.T) {
	start := time.Now()
	res := New(Options{
		VaultDir: t.TempDir(),
		Runner:   hungRunner{},
		// The env leg short-circuits the git-config leg so the first — and
		// only — git invocation is the repository probe, which hangs.
		Env:     mapEnv(map[string]string{EnvDefaultBranchRef: "origin/main"}),
		Timeout: 25 * time.Millisecond,
	}).Resolve(context.Background())
	elapsed := time.Since(start)

	if res.Status != StatusGitFailed {
		t.Fatalf("Status = %q, want %q", res.Status, StatusGitFailed)
	}
	if !strings.Contains(res.Reason, "context deadline exceeded") {
		t.Errorf("Reason = %q, want it to contain the context deadline", res.Reason)
	}
	if elapsed > 5*time.Second {
		t.Errorf("Resolve took %v, want it bounded by the per-invocation timeout", elapsed)
	}
}

// gitrefRepo creates a git repository with one commit touching
// compiled/auth.md and, when fetchRef is true, a refs/remotes/origin/main
// remote-tracking ref pointing at it — the local state a fetch leaves
// behind, created without any network access.
func gitrefRepo(t *testing.T, fetchRef bool) (string, string) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping git subprocess test in short mode")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		full := append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)
		if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q", "-b", "main")
	if err := os.MkdirAll(filepath.Join(dir, "compiled"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	page := filepath.Join(dir, "compiled", "auth.md")
	if err := os.WriteFile(page, []byte("---\ntier: validated\n---\n# Auth\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	git("add", ".")
	git("commit", "-q", "-m", "first")
	head := gitOut(t, dir, "rev-parse", "HEAD")
	if fetchRef {
		git("update-ref", "refs/remotes/origin/main", head)
	}
	return dir, head
}

// gitOut runs git in dir and returns its trimmed stdout.
func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-C", dir}, args...)
	out, err := exec.Command("git", full...).Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

// TestResolve_RealGit exercises the production ExecRunner against real
// fixture repositories: offline resolution through a fetched remote-tracking
// ref (IP-FR-004), the missing-ref and non-git fallbacks (GTS-FR-004), and
// the git-config precedence leg.
func TestResolve_RealGit(t *testing.T) {
	t.Run("fetched ref resolves offline", func(t *testing.T) {
		dir, head := gitrefRepo(t, true)
		res := New(Options{VaultDir: dir, Env: mapEnv(nil)}).Resolve(context.Background())
		if !res.Resolved() {
			t.Fatalf("resolution = %+v, want resolved", res)
		}
		if res.Ref != "origin/main" || res.FullRef != "refs/remotes/origin/main" {
			t.Errorf("ref = %q (%q), want origin/main (refs/remotes/origin/main)", res.Ref, res.FullRef)
		}
		if res.SHA != head {
			t.Errorf("SHA = %q, want HEAD %q", res.SHA, head)
		}
		if res.Leg != LegDefault {
			t.Errorf("Leg = %q, want %q", res.Leg, LegDefault)
		}
	})

	t.Run("missing remote-tracking ref falls back with signal", func(t *testing.T) {
		dir, _ := gitrefRepo(t, false)
		res := New(Options{VaultDir: dir, Env: mapEnv(nil)}).Resolve(context.Background())
		if res.Status != StatusMissingRef {
			t.Fatalf("Status = %q, want %q (reason: %q)", res.Status, StatusMissingRef, res.Reason)
		}
		if res.SHA != "" {
			t.Errorf("SHA = %q, want empty on missing ref", res.SHA)
		}
	})

	t.Run("non-git vault falls back", func(t *testing.T) {
		res := New(Options{VaultDir: t.TempDir(), Env: mapEnv(nil)}).Resolve(context.Background())
		if res.Status != StatusNotARepo {
			t.Fatalf("Status = %q, want %q (reason: %q)", res.Status, StatusNotARepo, res.Reason)
		}
	})

	t.Run("git config leg supplies the ref", func(t *testing.T) {
		dir, head := gitrefRepo(t, true)
		git := func(args ...string) {
			t.Helper()
			full := append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)
			if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
		}
		git("config", GitConfigKey, "origin/main")
		res := New(Options{VaultDir: dir, Env: mapEnv(nil)}).Resolve(context.Background())
		if !res.Resolved() {
			t.Fatalf("resolution = %+v, want resolved", res)
		}
		if res.Leg != LegGitConfig {
			t.Errorf("Leg = %q, want %q", res.Leg, LegGitConfig)
		}
		if res.SHA != head {
			t.Errorf("SHA = %q, want HEAD %q", res.SHA, head)
		}
	})

	t.Run("vault config leg wins over git config", func(t *testing.T) {
		dir, head := gitrefRepo(t, true)
		git := func(args ...string) {
			t.Helper()
			full := append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)
			if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
		}
		// The git-config leg points at a ref that does not exist; the vault
		// config must win before it is ever consulted.
		git("config", GitConfigKey, "other/missing")
		writeVaultConfig(t, dir, "origin/main")
		res := New(Options{VaultDir: dir, Env: mapEnv(nil)}).Resolve(context.Background())
		if !res.Resolved() {
			t.Fatalf("resolution = %+v, want resolved", res)
		}
		if res.Leg != LegVaultConfig {
			t.Errorf("Leg = %q, want %q", res.Leg, LegVaultConfig)
		}
		if res.SHA != head {
			t.Errorf("SHA = %q, want HEAD %q", res.SHA, head)
		}
	})
}
