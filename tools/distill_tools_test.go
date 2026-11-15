package tools

// Handler-level tests for the distill automation MCP tools
// (018-weekly-distill-automation, T039): the scope, group_learners, and
// rolling_pr handlers as registered in server.go — guard ordering,
// result shapes, and the FR-021 no-op artifact recording. The pipeline
// functions underneath (ComputeScope, GroupLearners, ManageRollingPRs)
// carry their own fixture suites (scope_test.go, group_test.go,
// rolling_pr_test.go); these tests pin the MCP surface that wraps them:
// the empty-vault-path guard fires before any pipeline work, empty
// inputs are rejected as caller errors, results decode as JSON, and
// the no-op and run artifacts land server-side.

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// decodeToolJSON asserts a successful text result and unmarshals its
// JSON payload into target.
func decodeToolJSON(t *testing.T, label string, res *mcp.CallToolResult, target any) {
	t.Helper()
	if res == nil {
		t.Fatalf("%s: result is nil", label)
	}
	if res.IsError {
		t.Fatalf("%s: result is an error result: %+v", label, res.Content)
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("%s: content[0] is %T, want *mcp.TextContent", label, res.Content[0])
	}
	if err := json.Unmarshal([]byte(tc.Text), target); err != nil {
		t.Fatalf("%s: decode JSON payload: %v\n%s", label, err, tc.Text)
	}
}

// toolErrorText asserts an error result and returns its text.
func toolErrorText(t *testing.T, label string, res *mcp.CallToolResult) string {
	t.Helper()
	if res == nil {
		t.Fatalf("%s: result is nil", label)
	}
	if !res.IsError {
		t.Fatalf("%s: result is not an error result: %+v", label, res.Content)
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("%s: content[0] is %T, want *mcp.TextContent", label, res.Content[0])
	}
	return tc.Text
}

// TestDistillScope_VaultPathRequired pins the construction guard: a
// Distill handler without a vault path rejects before any pipeline
// work rather than dereferencing an empty path.
func TestDistillScope_VaultPathRequired(t *testing.T) {
	d := NewDistill(nil, nil, "")
	res, _, err := d.Scope(context.Background(), nil, DistillScopeInput{})
	if err != nil {
		t.Fatalf("Scope returned err %v, want error result", err)
	}
	if got := toolErrorText(t, "scope", res); !strings.Contains(got, "vault path") {
		t.Errorf("scope error = %q, want vault-path mention", got)
	}
}

// TestDistillScope_Fixture pins the happy path against the git fixture:
// the result decodes as JSON carrying the deterministic scope and no
// no-op signal.
func TestDistillScope_Fixture(t *testing.T) {
	dir, _ := initDistillFixtureRepo(t)
	d := NewDistill(nil, nil, dir)

	res, _, err := d.Scope(context.Background(), nil, DistillScopeInput{})
	if err != nil {
		t.Fatalf("Scope returned err %v, want result", err)
	}
	var payload distillScopeResult
	decodeToolJSON(t, "scope", res, &payload)
	if payload.NoOp {
		t.Error("no_op = true, want false (fixture has in-scope learnings)")
	}
	if want := expectedFixtureInScope(); !reflect.DeepEqual(payload.Scope.InScope, want) {
		t.Errorf("in_scope = %v, want %v", payload.Scope.InScope, want)
	}
	if payload.Scope.Empty {
		t.Error("scope.empty = true, want false")
	}
}

// TestDistillScope_EmptyScopeNoOpArtifact pins the FR-021 handler
// behavior: an empty scope records the no-op artifact server-side (the
// agent cannot write files) and the result carries the artifact path.
func TestDistillScope_EmptyScopeNoOpArtifact(t *testing.T) {
	dir, _ := initDistillFixtureRepo(t)
	// Cite every learning: one compiled article listing all ten sources.
	citing := "---\ntier: draft\nsources:\n"
	for _, id := range distillFixtureIdentities {
		citing += "  - " + id + "\n"
	}
	citing += "---\n# Everything\n"
	if err := os.WriteFile(filepath.Join(dir, ".uf/dewey/compiled/everything.md"), []byte(citing), 0o644); err != nil {
		t.Fatalf("write everything.md: %v", err)
	}
	git := func(args ...string) {
		t.Helper()
		full := append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)
		if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("add", ".")
	git("commit", "-q", "-m", "cite everything")
	// Scope enumerates at the ref: re-point origin/main at the citing commit.
	git("update-ref", "refs/remotes/origin/main", "HEAD")

	d := NewDistill(nil, nil, dir)
	res, _, err := d.Scope(context.Background(), nil, DistillScopeInput{})
	if err != nil {
		t.Fatalf("Scope returned err %v, want result", err)
	}
	var payload distillScopeResult
	decodeToolJSON(t, "scope", res, &payload)
	if !payload.NoOp {
		t.Error("no_op = false, want true (every learning cited)")
	}
	if payload.NoOpReason != NoOpReasonEmptyScope {
		t.Errorf("no_op_reason = %q, want %q", payload.NoOpReason, NoOpReasonEmptyScope)
	}
	if payload.Artifact == "" {
		t.Fatal("artifact is empty, want the recorded no-op artifact path")
	}
	if _, err := os.Stat(payload.Artifact); err != nil {
		t.Errorf("artifact %s not recorded: %v", payload.Artifact, err)
	}
}

// TestDistillScope_BadVaultDir pins the error path: a vault directory
// that does not exist is an error result, never a masquerading empty
// no-op scope.
func TestDistillScope_BadVaultDir(t *testing.T) {
	d := NewDistill(nil, nil, filepath.Join(t.TempDir(), "does-not-exist"))
	res, _, err := d.Scope(context.Background(), nil, DistillScopeInput{})
	if err != nil {
		t.Fatalf("Scope returned err %v, want error result", err)
	}
	if got := toolErrorText(t, "scope", res); !strings.Contains(got, "scope") {
		t.Errorf("scope error = %q, want a scope-prefixed error", got)
	}
}

// TestDistillGroupLearners_VaultPathRequired pins the construction
// guard for group_learners.
func TestDistillGroupLearners_VaultPathRequired(t *testing.T) {
	d := NewDistill(nil, nil, "")
	res, _, err := d.GroupLearners(context.Background(), nil, DistillGroupInput{InScope: []string{"x"}})
	if err != nil {
		t.Fatalf("GroupLearners returned err %v, want error result", err)
	}
	if got := toolErrorText(t, "group_learners", res); !strings.Contains(got, "vault path") {
		t.Errorf("group_learners error = %q, want vault-path mention", got)
	}
}

// TestDistillGroupLearners_EmptyInScopeRejected pins the pipeline-order
// guard: the scope tool exits no-op before grouping, so an empty
// in_scope here is a caller error.
func TestDistillGroupLearners_EmptyInScopeRejected(t *testing.T) {
	d := NewDistill(nil, nil, t.TempDir())
	res, _, err := d.GroupLearners(context.Background(), nil, DistillGroupInput{})
	if err != nil {
		t.Fatalf("GroupLearners returned err %v, want error result", err)
	}
	if got := toolErrorText(t, "group_learners", res); !strings.Contains(got, "in_scope is empty") {
		t.Errorf("group_learners error = %q, want empty-in_scope mention", got)
	}
}

// TestDistillGroupLearners_TagFallback pins the happy path with a nil
// embedder: the result decodes as JSON, reports the documented degraded
// mode, and groups the fixture by tag centroids.
func TestDistillGroupLearners_TagFallback(t *testing.T) {
	dir, _ := initDistillFixtureRepo(t)
	d := NewDistill(nil, nil, dir) // nil embedder selects tag fallback

	res, _, err := d.GroupLearners(context.Background(), nil, DistillGroupInput{InScope: expectedFixtureInScope()})
	if err != nil {
		t.Fatalf("GroupLearners returned err %v, want result", err)
	}
	var payload distillGroupResult
	decodeToolJSON(t, "group_learners", res, &payload)
	if payload.EmbeddingsAvailable {
		t.Error("embeddings_available = true, want false (nil embedder)")
	}
	if len(payload.Themes) == 0 {
		t.Fatal("themes is empty, want tag-fallback themes from the fixture")
	}
	// The union of member identities must equal the in-scope input —
	// grouping partitions the scope.
	grouped := make(map[string]bool)
	for _, theme := range payload.Themes {
		for _, id := range theme.Members {
			if grouped[id] {
				t.Errorf("identity %s appears in more than one theme", id)
			}
			grouped[id] = true
		}
	}
	want := make(map[string]bool)
	for _, id := range expectedFixtureInScope() {
		want[id] = true
	}
	if !reflect.DeepEqual(grouped, want) {
		t.Errorf("grouped members = %v, want exactly the in-scope set %v", grouped, want)
	}
}

// TestDistillRollingPR_VaultPathRequired pins the construction guard
// for rolling_pr.
func TestDistillRollingPR_VaultPathRequired(t *testing.T) {
	d := NewDistill(nil, nil, "")
	res, _, err := d.RollingPR(context.Background(), nil, DistillRollingPRInput{Themes: []Theme{{Name: "auth"}}})
	if err != nil {
		t.Fatalf("RollingPR returned err %v, want error result", err)
	}
	if got := toolErrorText(t, "rolling_pr", res); !strings.Contains(got, "vault path") {
		t.Errorf("rolling_pr error = %q, want vault-path mention", got)
	}
}

// TestDistillRollingPR_EmptyThemesRejected pins the pipeline-order
// guard: the group_learners tool produces the themes this stage
// consumes, so zero themes is a caller error.
func TestDistillRollingPR_EmptyThemesRejected(t *testing.T) {
	d := NewDistill(nil, nil, t.TempDir())
	res, _, err := d.RollingPR(context.Background(), nil, DistillRollingPRInput{})
	if err != nil {
		t.Fatalf("RollingPR returned err %v, want error result", err)
	}
	if got := toolErrorText(t, "rolling_pr", res); !strings.Contains(got, "no themes") {
		t.Errorf("rolling_pr error = %q, want no-themes mention", got)
	}
}

// TestDistillScope_BacklogFromBoundsWindow pins the FR-020 backlog
// bootstrap bound at the tool boundary: a cutoff on or after every
// fixture timestamp keeps the scope, an earlier cutoff empties it (the
// FR-021 no-op path fires), and legacy identities without a timestamp
// always pass.
func TestDistillScope_BacklogFromBoundsWindow(t *testing.T) {
	dir, _ := initDistillFixtureRepo(t)
	d := NewDistill(nil, nil, dir)

	// The fixture's identities all carry 20260101T0000xx timestamps, so
	// a 2026-01-01 cutoff keeps the full in-scope set.
	res, _, err := d.Scope(context.Background(), nil, DistillScopeInput{BacklogFrom: "2026-01-01"})
	if err != nil {
		t.Fatalf("Scope returned err %v, want result", err)
	}
	var payload distillScopeResult
	decodeToolJSON(t, "scope", res, &payload)
	if payload.NoOp {
		t.Error("no_op = true, want false (cutoff covers every fixture learning)")
	}
	if want := expectedFixtureInScope(); !reflect.DeepEqual(payload.Scope.InScope, want) {
		t.Errorf("in_scope = %v, want %v", payload.Scope.InScope, want)
	}

	// A cutoff before every timestamp leaves nothing in scope: the
	// no-op exit fires and the artifact records why.
	res, _, err = d.Scope(context.Background(), nil, DistillScopeInput{BacklogFrom: "2025-12-31"})
	if err != nil {
		t.Fatalf("Scope returned err %v, want result", err)
	}
	decodeToolJSON(t, "scope", res, &payload)
	if !payload.NoOp {
		t.Fatal("no_op = false, want true (cutoff predates every fixture learning)")
	}
	if payload.NoOpReason != NoOpReasonEmptyScope {
		t.Errorf("no_op_reason = %q, want %q", payload.NoOpReason, NoOpReasonEmptyScope)
	}
	if payload.Artifact == "" {
		t.Error("artifact empty on the bounded no-op, want the recorded path")
	}
}

// TestDistillScope_BacklogFromInvalidDateFailsClosed pins the fail-closed
// bound: an unparseable backlog_from is an error result, never a silently
// unbounded or silently empty scope.
func TestDistillScope_BacklogFromInvalidDateFailsClosed(t *testing.T) {
	dir, _ := initDistillFixtureRepo(t)
	d := NewDistill(nil, nil, dir)

	res, _, err := d.Scope(context.Background(), nil, DistillScopeInput{BacklogFrom: "2026/01/01"})
	if err != nil {
		t.Fatalf("Scope returned err %v, want error result", err)
	}
	if got := toolErrorText(t, "scope", res); !strings.Contains(got, "backlog_from") {
		t.Errorf("scope error = %q, want backlog_from mention", got)
	}
}

// TestDistillRollingPR_DryRunPreviewNoArtifact pins the dry-run preview at
// the tool boundary (FR-020): each theme reports its would-be state, no
// mutation happens, and no run artifact is recorded — the preview result
// is the run's entire output.
func TestDistillRollingPR_DryRunPreviewNoArtifact(t *testing.T) {
	const alice = "auth-20260101T000001-alice"
	texts := map[string]string{alice: rollingFixtureLearning("auth", "Alice's auth learning.")}
	vault := initRollingPRFixture(t, texts)
	gh := newFakeGH(t, 11)
	d := &Distill{vaultPath: vault, ghRunner: gh}

	res, _, err := d.RollingPR(context.Background(), nil, DistillRollingPRInput{
		Themes: []Theme{distillTestTheme("auth", "", alice)},
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("RollingPR returned err %v, want result", err)
	}
	var payload distillRollingPRResult
	decodeToolJSON(t, "rolling_pr", res, &payload)
	if len(payload.Results) != 1 {
		t.Fatalf("results = %d, want 1", len(payload.Results))
	}
	if payload.Results[0].State != RollingPRWouldCreate {
		t.Errorf("state = %q, want %q", payload.Results[0].State, RollingPRWouldCreate)
	}
	if payload.Artifact != "" {
		t.Errorf("artifact = %q, want empty — a dry run records no artifact", payload.Artifact)
	}
	if got, want := gh.calls, []string{"list"}; !reflect.DeepEqual(got, want) {
		t.Errorf("gh calls = %v, want %v", got, want)
	}
	branch := AssignThemeBranches([]string{"auth"})["auth"]
	if out := rollingGit(t, vault, "ls-remote", "origin", "refs/heads/"+branch); strings.TrimSpace(out) != "" {
		t.Error("dry run created a remote branch; the preview must not push")
	}
}

// TestDistillRollingPR_TwoThemesRunArtifact pins the happy path with an
// injected gh runner: one draft PR per theme, tier written draft, and
// the FR-021 run artifact recorded server-side with the themes and
// touched branches.
func TestDistillRollingPR_TwoThemesRunArtifact(t *testing.T) {
	const alice = "auth-20260101T000001-alice"
	const bob = "auth-20260101T000002-bob"
	const carol = "db-20260101T000003-carol"
	texts := map[string]string{
		alice: rollingFixtureLearning("auth", "Alice's auth learning."),
		bob:   rollingFixtureLearning("auth", "Bob's auth learning."),
		carol: rollingFixtureLearning("db", "Carol's db learning."),
	}
	vault := initRollingPRFixture(t, texts)
	d := &Distill{vaultPath: vault, ghRunner: newFakeGH(t, 11)}

	res, _, err := d.RollingPR(context.Background(), nil, DistillRollingPRInput{
		Themes: []Theme{
			distillTestTheme("auth", "", alice, bob),
			distillTestTheme("db", "", carol),
		},
	})
	if err != nil {
		t.Fatalf("RollingPR returned err %v, want result", err)
	}
	var payload distillRollingPRResult
	decodeToolJSON(t, "rolling_pr", res, &payload)
	if len(payload.Results) != 2 {
		t.Fatalf("results = %d, want 2", len(payload.Results))
	}
	touched := make(map[string]string)
	for _, r := range payload.Results {
		if r.State != RollingPRCreated {
			t.Errorf("theme %s state = %q, want %q", r.Theme, r.State, RollingPRCreated)
		}
		if r.TierWritten != DistillTierDraft {
			t.Errorf("theme %s tier_written = %q, want %q", r.Theme, r.TierWritten, DistillTierDraft)
		}
		touched[r.Branch] = r.Theme
	}
	if len(touched) != 2 {
		t.Errorf("touched branches = %v, want 2 distinct branches", touched)
	}

	// The run artifact is the FR-021 audit surface: it exists, names
	// both themes, and lists both touched branches.
	if payload.Artifact == "" {
		t.Fatal("artifact is empty, want the recorded run artifact path")
	}
	data, err := os.ReadFile(payload.Artifact)
	if err != nil {
		t.Fatalf("read run artifact %s: %v", payload.Artifact, err)
	}
	var record DistillRunRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("decode run artifact: %v\n%s", err, data)
	}
	if record.ScopeSize != 3 {
		t.Errorf("artifact scope_size = %d, want 3", record.ScopeSize)
	}
	if record.Schema != distillRunRecordSchema {
		t.Errorf("artifact schema = %d, want %d", record.Schema, distillRunRecordSchema)
	}
	if len(record.Themes) != 2 || len(record.PRsTouched) != 2 {
		t.Errorf("artifact themes = %v, prs_touched = %v, want 2 and 2", record.Themes, record.PRsTouched)
	}
}
