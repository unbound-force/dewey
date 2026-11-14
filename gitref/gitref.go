// Package gitref resolves the default-branch comparison ref that backs the
// git-trust-source capability
// (openspec/changes/git-is-the-source-of-record-for-page-trust-tiers): the
// locally fetched <remote>/<default-branch> ref whose blob equality grants
// the validated trust tier (GTS-FR-001).
//
// Resolution is offline and deterministic per IP-FR-004 and design decisions
// D2/D7: the effective <remote>/<default-branch> value comes from exactly one
// precedence chain — per-vault .uf/dewey/config.yaml → global config.yaml →
// environment variable DEWEY_DEFAULT_BRANCH_REF → git config
// dewey.defaultBranchRef → default origin/main — and the first leg providing
// a non-blank value wins. Every leg's value is validated in-process against
// the git check-ref-format grammar (plus flag-injection guards) BEFORE it is
// used in any git invocation, so a malformed or hostile value such as
// "--upload-pack=evil" is rejected with an invalid-config diagnostic and
// never reaches git.
//
// All git access is local plumbing invoked through argv arrays (never a
// shell string), bounded by a 5-second per-invocation timeout and a
// 60-second cumulative budget per Resolve call, following the
// tools/lint_anchors.go precedent; context cancellation propagates to the git
// child process, so a hung git cannot stall indexing. There is no network
// fetch — only remote-tracking refs already present in local git state are
// consulted.
//
// Resolution is fail-safe by contract: Resolve never returns an error and
// never fails the caller's ingestion. Every git derivation failure — missing
// binary, permission denied, corrupt object database, malformed output, a
// non-zero exit that does not mean "ref not found", or a hung process past
// the timeout — is reported as a non-resolved Status with a Reason, which
// the caller maps to the GTS-FR-004 draft fallback. The precedence leg that
// supplied the effective ref (or the fallback cause) is recorded on the
// result and in the operational log, for surfacing via dewey doctor.
package gitref

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"gopkg.in/yaml.v3"
)

// DefaultRef is the hard-default <remote>/<default-branch> comparison ref
// used when no precedence leg provides a value (IP-FR-004).
const DefaultRef = "origin/main"

// EnvDefaultBranchRef is the environment-variable leg of the IP-FR-004
// precedence chain. Its value must satisfy the <remote>/<branch> grammar and
// is validated like any other leg before reaching git.
const EnvDefaultBranchRef = "DEWEY_DEFAULT_BRANCH_REF"

// GitConfigKey is the git-config leg of the IP-FR-004 precedence chain,
// read via `git config --get dewey.defaultBranchRef`.
const GitConfigKey = "dewey.defaultBranchRef"

// ConfigFileKey is the config.yaml key (per-vault and global legs) that
// carries the <remote>/<default-branch> value.
const ConfigFileKey = "default_branch_ref"

const (
	// invokeTimeout bounds each git subprocess run during resolution. A hung
	// git process is killed at the deadline (the context cancellation
	// propagates through exec.CommandContext), following the
	// tools/lint_anchors.go precedent.
	invokeTimeout = 5 * time.Second

	// resolutionBudget bounds one whole Resolve pass. Resolution issues at
	// most three git invocations, so this is a safety net against pathological
	// cases, mirroring lint's anchorCheckBudget structure.
	resolutionBudget = 60 * time.Second

	// refsRemotesPrefix is prepended to the validated <remote>/<branch>
	// value to form the fully qualified remote-tracking ref. Using the
	// fully qualified form removes DWIM ambiguity with local branches or
	// tags that happen to share the short name.
	refsRemotesPrefix = "refs/remotes/"

	// notARepoMarker is the stable git error phrase for running plumbing
	// outside a repository; it distinguishes the GTS-FR-004 non-git-vault
	// fallback from other git failures for diagnostics.
	notARepoMarker = "not a git repository"

	// maxDiagLen bounds stderr excerpts embedded in diagnostics so a verbose
	// git failure cannot flood the log line.
	maxDiagLen = 200
)

// gitrefLogger is the package-level structured logger for default-branch ref
// resolution (the only package-level variable, per repo convention).
var gitrefLogger = log.NewWithOptions(os.Stderr, log.Options{
	Prefix:          "dewey/gitref",
	ReportTimestamp: true,
	TimeFormat:      "2006-01-02T15:04:05.000Z07:00",
})

// Status classifies the outcome of a default-branch ref resolution. Every
// status other than StatusResolved triggers the GTS-FR-004 fallback in the
// caller: affected pages degrade to draft with the recorded diagnostic, and
// ingestion continues.
type Status string

const (
	// StatusResolved means the ref resolved locally to a commit SHA.
	StatusResolved Status = "resolved"
	// StatusMissingRef is the resolvable-missing signal: the configured ref
	// value is valid but not present in local git state (no fetch yet).
	StatusMissingRef Status = "missing-ref"
	// StatusNotARepo means the vault directory is not inside a git
	// repository.
	StatusNotARepo Status = "not-a-repo"
	// StatusInvalidConfig means the configured ref value failed grammar
	// validation and was rejected before any git invocation carried it.
	StatusInvalidConfig Status = "invalid-config"
	// StatusGitFailed covers every other git derivation failure: missing
	// binary, permission denied, corrupt object database, malformed output,
	// an unexpected non-zero exit, or a hung process past the bounded
	// timeout.
	StatusGitFailed Status = "git-failed"
)

// Leg names the precedence source that supplied the effective
// <remote>/<default-branch> value (IP-FR-004). It is recorded on every
// RefResolution — resolved or not — so any configuration difference is
// explainable rather than heuristic.
type Leg string

const (
	// LegVaultConfig is the per-vault .uf/dewey/config.yaml leg.
	LegVaultConfig Leg = "vault-config"
	// LegGlobalConfig is the global config.yaml leg.
	LegGlobalConfig Leg = "global-config"
	// LegEnv is the DEWEY_DEFAULT_BRANCH_REF environment-variable leg.
	LegEnv Leg = "env"
	// LegGitConfig is the git config dewey.defaultBranchRef leg.
	LegGitConfig Leg = "git-config"
	// LegDefault is the hard default origin/main.
	LegDefault Leg = "default"
)

// RefResolution is the outcome of resolving the default-branch comparison
// ref. Callers that derive trust tiers treat any non-resolved status as the
// GTS-FR-004 draft fallback and never fail ingestion on it.
type RefResolution struct {
	// Ref is the effective <remote>/<branch> value (for example
	// "origin/main"), exactly as the winning precedence leg provided it. It
	// is recorded even when resolution fails, so diagnostics can name what
	// was attempted.
	Ref string `json:"ref"`
	// FullRef is the fully qualified remote-tracking ref
	// ("refs/remotes/<Ref>"), empty when the value was rejected by grammar
	// validation. Consumers running further git plumbing against the ref
	// (ls-tree batch lookups) SHOULD use FullRef to keep the same
	// disambiguation.
	FullRef string `json:"full_ref,omitempty"`
	// SHA is the 40-hex commit SHA of the locally fetched ref. It is empty
	// unless Status is StatusResolved.
	SHA string `json:"sha,omitempty"`
	// Status classifies the outcome.
	Status Status `json:"status"`
	// Leg records which precedence source supplied Ref.
	Leg Leg `json:"leg"`
	// Reason records the diagnostic cause when Status is not
	// StatusResolved: the invalid configuration, the missing ref, or the git
	// failure. Observable in logs and via dewey doctor.
	Reason string `json:"reason,omitempty"`
}

// Resolved reports whether the resolution produced a usable ref and commit
// SHA. It is the gate for the GTS-FR-001 blob-equality grant: an unresolved
// ref withholds validated (callers pass an empty default-branch blob SHA)
// without failing ingestion.
func (r RefResolution) Resolved() bool {
	return r.Status == StatusResolved
}

// Runner executes one git invocation. dir is the working directory for the
// command and args are the git arguments without the leading "git" (for
// example []string{"rev-parse", "--git-dir"}). Implementations MUST honor
// ctx: a cancelled or expired context stops the invocation and surfaces the
// context error. err is non-nil only when git could not be run at all
// (binary missing, timeout, cancellation) — never for a non-zero exit, which
// is reported through code together with stderr. The seam is injectable so
// tests can exercise failure paths without a real git (DR: injectable git
// runner for tests).
type Runner interface {
	Run(ctx context.Context, dir string, args []string) (stdout, stderr string, code int, err error)
}

// ExecRunner is the production Runner. It runs the git binary through argv
// arrays — never a shell string, per IP-FR-004 — with exec.CommandContext,
// so the parent context's deadline both kills the child process (cancellation
// propagates; a hung git cannot outlive the timeout) and surfaces as the
// returned error.
type ExecRunner struct{}

// Run implements Runner by executing the git binary in dir.
func (ExecRunner) Run(ctx context.Context, dir string, args []string) (string, string, int, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && ctx.Err() == nil {
		// A non-zero exit is an answer from git, not a failure to run it.
		return stdout.String(), stderr.String(), exitErr.ExitCode(), nil
	}
	if err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return "", stderr.String(), -1, fmt.Errorf("git %s: %w", strings.Join(args, " "), cerr)
		}
		return "", stderr.String(), -1, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return stdout.String(), stderr.String(), 0, nil
}

// Options configures a Resolver. The zero value is not usable: VaultDir MUST
// name the vault root.
type Options struct {
	// VaultDir is the vault root. Git runs with it as the working directory
	// (git finds the enclosing repository by walking up, so a vault inside a
	// larger repository works), and the per-vault config leg reads
	// <VaultDir>/.uf/dewey/config.yaml.
	VaultDir string
	// Runner runs git; nil selects ExecRunner. Injectable for tests.
	Runner Runner
	// Env reads environment variables; nil selects os.Getenv. Injectable
	// for tests so precedence cases do not mutate process state.
	Env func(string) string
	// Timeout bounds each git invocation; zero selects the 5-second default.
	Timeout time.Duration
	// Budget bounds one whole Resolve pass; zero selects the 60-second
	// default.
	Budget time.Duration
}

// Resolver resolves the default-branch comparison ref for one vault. It is
// safe for concurrent use: Resolve reads only the configuration fixed at New
// and never caches results, so the outcome is deterministic for a given
// local ref state (GTS-FR-005).
type Resolver struct {
	vaultDir string
	runner   Runner
	env      func(string) string
	timeout  time.Duration
	budget   time.Duration
}

// New returns a Resolver for opts, applying the documented defaults for the
// injectable and timeout fields.
func New(opts Options) *Resolver {
	r := &Resolver{
		vaultDir: opts.VaultDir,
		runner:   opts.Runner,
		env:      opts.Env,
		timeout:  opts.Timeout,
		budget:   opts.Budget,
	}
	if r.runner == nil {
		r.runner = ExecRunner{}
	}
	if r.env == nil {
		r.env = os.Getenv
	}
	if r.timeout <= 0 {
		r.timeout = invokeTimeout
	}
	if r.budget <= 0 {
		r.budget = resolutionBudget
	}
	return r
}

// Resolve resolves the effective <remote>/<default-branch> ref and its
// commit SHA from local git state only — no network fetch (IP-FR-004).
//
// It never returns an error and never fails the caller's ingestion: any git
// derivation failure (missing binary, permission denied, corrupt object
// database, malformed output, a non-zero exit that does not mean "ref not
// found", or a hung process past the bounded timeout) is reported as a
// non-resolved Status whose Reason names the cause, for the caller's
// GTS-FR-004 draft fallback. The precedence leg that supplied the effective
// value (or the fallback cause) is recorded on the result and logged.
//
// The configured value is grammar-validated in-process before any git
// invocation carries it, so injection attempts such as "--upload-pack=evil"
// never reach git. The one git invocation that can precede validation — the
// git-config leg's `git config --get dewey.defaultBranchRef` — passes only
// the compile-time-constant config key, never the configured value.
func (r *Resolver) Resolve(ctx context.Context) RefResolution {
	ctx, cancel := context.WithTimeout(ctx, r.budget)
	defer cancel()

	if r.vaultDir == "" {
		res := RefResolution{
			Ref:    DefaultRef,
			Status: StatusGitFailed,
			Leg:    LegDefault,
			Reason: "no vault directory configured for default-branch ref resolution",
		}
		r.logResult(res)
		return res
	}

	value, leg := r.configValue(ctx)
	res := RefResolution{Ref: value, Leg: leg}
	if err := ValidateRefName(value); err != nil {
		// Rejected before any git invocation carries the value; the affected
		// source degrades to the GTS-FR-004 draft fallback.
		res.Status = StatusInvalidConfig
		res.Reason = fmt.Sprintf(
			"configured default-branch ref %q (from %s) is invalid: %v — rejected before any git invocation; pages degrade to draft per GTS-FR-004",
			value, leg, err)
		r.logResult(res)
		return res
	}
	res.FullRef = refsRemotesPrefix + value

	// Repository probe. A missing repository is the expected GTS-FR-004
	// non-git-vault case; any other probe failure is a git derivation
	// failure. Both degrade to draft with a diagnostic.
	if _, stderr, code, err := r.run(ctx, "rev-parse", "--git-dir"); err != nil {
		return r.fail(res, fmt.Sprintf("probe git repository: %v", err))
	} else if code != 0 {
		if strings.Contains(stderr, notARepoMarker) {
			res.Status = StatusNotARepo
			res.Reason = fmt.Sprintf(
				"vault %s is not a git repository — trust tiers degrade to draft per GTS-FR-004", r.vaultDir)
			r.logResult(res)
			return res
		}
		return r.fail(res, fmt.Sprintf(
			"probe git repository: git rev-parse --git-dir exited %d: %s", code, excerpt(stderr)))
	}

	// Ref resolution against the fully qualified remote-tracking ref.
	// --end-of-options is rev-parse's argument separator (the bare `--` is a
	// path separator and would be wrong here); combined with the in-process
	// grammar validation and argv-array invocation it guarantees the
	// validated value can never be parsed as an option. `--verify --quiet`
	// exits 1 exactly when the ref is not resolvable — the
	// resolvable-missing signal for the GTS-FR-004 fallback.
	out, stderr, code, err := r.run(ctx, "rev-parse", "--verify", "--quiet", "--end-of-options", res.FullRef)
	if err != nil {
		return r.fail(res, fmt.Sprintf("resolve %s: %v", res.FullRef, err))
	}
	switch code {
	case 0:
		sha := strings.TrimSpace(out)
		if !isHex40(sha) {
			return r.fail(res, fmt.Sprintf("resolve %s: malformed rev-parse output %q", res.FullRef, sha))
		}
		res.Status = StatusResolved
		res.SHA = sha
	case 1:
		res.Status = StatusMissingRef
		res.Reason = fmt.Sprintf(
			"default-branch ref %s (%s) is not present in local git state — run `git fetch %s`; pages degrade to draft per GTS-FR-004 until the ref is fetched",
			res.Ref, res.FullRef, remoteOf(res.Ref))
	default:
		return r.fail(res, fmt.Sprintf(
			"resolve %s: git rev-parse exited %d: %s", res.FullRef, code, excerpt(stderr)))
	}
	r.logResult(res)
	return res
}

// run runs one git invocation in the vault directory, bounded by the
// per-invocation timeout; the timeout's cancellation propagates to the git
// child process through ExecRunner.
func (r *Resolver) run(ctx context.Context, args ...string) (string, string, int, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	return r.runner.Run(ctx, r.vaultDir, args)
}

// fail completes a non-resolved resolution as a git derivation failure and
// records it in the log. Ingestion is never failed — the caller maps the
// result to the GTS-FR-004 draft fallback.
func (r *Resolver) fail(res RefResolution, reason string) RefResolution {
	res.Status = StatusGitFailed
	res.Reason = reason + " — pages degrade to draft per GTS-FR-004; ingestion continues"
	r.logResult(res)
	return res
}

// logResult records the resolution in the operational log: the precedence
// leg and SHA on success, and the fallback cause (status, ref, leg, reason)
// on every non-resolved outcome, observable for dewey doctor.
func (r *Resolver) logResult(res RefResolution) {
	if res.Status == StatusResolved {
		gitrefLogger.Debug("default-branch ref resolved",
			"ref", res.Ref, "sha", res.SHA, "leg", string(res.Leg), "dir", r.vaultDir)
		return
	}
	gitrefLogger.Warn("default-branch ref unresolved; affected pages degrade to draft",
		"status", string(res.Status), "ref", res.Ref, "leg", string(res.Leg),
		"reason", res.Reason, "dir", r.vaultDir)
}

// configValue walks the IP-FR-004 precedence chain and returns the effective
// <remote>/<default-branch> value with the leg that supplied it. The first
// leg providing a non-blank value wins; a blank value (unset, empty, or
// whitespace-only) contributes nothing and the chain continues. Values are
// used exactly as provided — never normalized — so padded or malformed
// values surface as invalid-config diagnostics rather than being silently
// repaired.
func (r *Resolver) configValue(ctx context.Context) (string, Leg) {
	if v := readConfigRef(filepath.Join(r.vaultDir, ".uf", "dewey", "config.yaml")); !isBlank(v) {
		return v, LegVaultConfig
	}
	if dir := r.globalConfigDir(); dir != "" {
		if v := readConfigRef(filepath.Join(dir, "config.yaml")); !isBlank(v) {
			return v, LegGlobalConfig
		}
	}
	if v := r.env(EnvDefaultBranchRef); !isBlank(v) {
		return v, LegEnv
	}
	if v, ok := r.gitConfigValue(ctx); ok {
		return v, LegGitConfig
	}
	return DefaultRef, LegDefault
}

// globalConfigDir returns the global dewey config directory, mirroring the
// established embed/llm pattern: $XDG_CONFIG_HOME/dewey when set, else
// $HOME/.config/dewey. Lookups go through the injectable env function so
// tests can redirect the global leg without mutating process state.
func (r *Resolver) globalConfigDir() string {
	if xdg := r.env("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "dewey")
	}
	home := r.env("HOME")
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "dewey")
}

// gitConfigValue reads the git-config precedence leg via one bounded git
// invocation. `git config --get` exits 1 when the key is unset — the
// expected "leg provides nothing" signal, not a failure — and any other
// failure (missing binary, unexpected exit) also contributes no value: the
// chain falls to the hard default, and repository-level git failures surface
// later through the repository probe with their own diagnostic. The
// invocation carries only the compile-time-constant config key; the
// configured ref value never reaches git before validation.
func (r *Resolver) gitConfigValue(ctx context.Context) (string, bool) {
	out, _, code, err := r.run(ctx, "config", "--get", GitConfigKey)
	if err != nil || code != 0 {
		return "", false
	}
	return strings.TrimSpace(out), true
}

// readConfigRef reads the default_branch_ref value from one config.yaml
// file. An absent or unreadable file, or one that fails to parse, contributes
// no value: the caller falls through to the next precedence leg (mirroring
// the established config-reading pattern in the embed and llm packages).
func readConfigRef(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var cfg struct {
		DefaultBranchRef string `yaml:"default_branch_ref"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		gitrefLogger.Warn("failed to parse config.yaml for "+ConfigFileKey+"; continuing to next precedence leg",
			"path", path, "err", err)
		return ""
	}
	return cfg.DefaultBranchRef
}

// isBlank reports whether a precedence-leg value provides nothing: unset,
// empty, or whitespace-only values skip to the next leg.
func isBlank(v string) bool {
	return strings.TrimSpace(v) == ""
}

// remoteOf returns the remote component of a <remote>/<branch> value, for
// fetch-hint diagnostics.
func remoteOf(value string) string {
	if i := strings.IndexByte(value, '/'); i >= 0 {
		return value[:i]
	}
	return value
}

// excerpt returns a bounded single-line excerpt of git stderr for
// diagnostics.
func excerpt(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if len(s) > maxDiagLen {
		s = s[:maxDiagLen]
	}
	return s
}

// isHex40 reports whether s is a 40-character lowercase hex string, the
// shape of a git object ID as rev-parse prints it. Anything else counts as
// malformed output.
func isHex40(s string) bool {
	if len(s) != 40 {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}
