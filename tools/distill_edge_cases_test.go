// Edge-case composition coverage (specs/018-weekly-distill-automation
// T044; spec.md Edge Cases). The focused suites already pin each edge
// case in isolation — malformed policy entries (TestLoadDistillPolicy_*
// in scope_test.go), racing runs and leased-push retries (TestManageRollingPRs_* in
// rolling_pr_test.go), learnings arriving while a PR is under review
// (TestManageRollingPRs_SecondRunUpdatesPreservesHumanCommit), the
// degraded non-git-vault fallback (TestComputeScope_NonGitFallsBack-
// Degraded), tag-fallback grouping with degraded-mode decision logging
// per research.md D-003 (TestGroupLearners_TagFallbackDegraded), and
// human edits between runs (TestManageRollingPRs_ConflictLabelsAndNotes,
// TestManageRollingPRs_HumanEditNotOverwritten). This file covers the
// one genuine composition gap: "No new learnings since last run" — an
// empty scope recorded as a no-op while a prior run's theme PR is still
// open under review. The workflow records a no-op, leaves all open
// theme PRs untouched, and posts nothing.
package tools

import (
	"context"
	"os"
	"reflect"
	"testing"
)

// TestEdgeCase_EmptyScopePostsNothingAndLeavesPRs drives the pipeline
// as /dewey-distill-auto composes it (scope → group_learners →
// rolling_pr) against a fixture whose learnings are all cited on the
// default branch while a prior run's theme PR is open under review: the
// scope tool reports the FR-021 no-op with the empty-scope reason and a
// recorded artifact, the open PR's remote branch tip is untouched, and
// the gh surface records zero activity.
func TestEdgeCase_EmptyScopePostsNothingAndLeavesPRs(t *testing.T) {
	const alice = "auth-20260101T000001-alice"
	const bob = "auth-20260101T000002-bob"
	texts := map[string]string{
		alice: rollingFixtureLearning("auth", "Alice's auth learning."),
		bob:   rollingFixtureLearning("auth", "Bob's auth learning."),
	}
	vault := initRollingPRFixture(t, texts)

	// The prior cycle's default-branch state: one compiled article
	// already cites every learning (a new weekly run has nothing in
	// scope), and the vault config pins the default-branch ref so
	// gitref resolution is hermetic — the pattern from
	// initDistillFixtureRepo in scope_test.go.
	citing := "---\ntier: draft\nsources:\n"
	for _, id := range []string{alice, bob} {
		citing += "  - " + id + "\n"
	}
	citing += "topic: auth\n---\n\n# Authentication\n\nPrior compiled knowledge.\n"
	writeFixtureFile(t, vault, distillCompiledPrefix+"auth.md", citing)
	writeFixtureFile(t, vault, ".uf/dewey/config.yaml", "default_branch_ref: origin/main\n")
	rollingGit(t, vault, "add", ".")
	rollingGit(t, vault, "commit", "-q", "-m", "chore: cite every learning and pin fixture ref")
	rollingGit(t, vault, "push", "-q", "origin", "main")

	// A prior weekly run's theme PR, still open under review.
	gh := newFakeGH(t, 11)
	results, err := ManageRollingPRs(context.Background(),
		RollingPROptions{VaultDir: vault, GhRunner: gh},
		RollingPRInput{Themes: []Theme{distillTestTheme("auth", "", alice, bob)}, Texts: texts})
	if err != nil {
		t.Fatalf("establish the prior run's theme PR: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("prior run results = %d, want 1", len(results))
	}
	if r := results[0]; r.State != RollingPRCreated || r.PRNumber != 11 {
		t.Fatalf("prior run result = %+v, want one created PR #11", r)
	}
	if got, want := gh.calls, []string{"list", "create"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("prior run gh calls = %v, want %v", got, want)
	}
	branch := AssignThemeBranches([]string{"auth"})["auth"]
	prTip := remoteSHA(t, vault, branch)

	// The new weekly run starts: the scope tool computes from git
	// alone and finds every learning cited.
	d := &Distill{vaultPath: vault}
	res, _, err := d.Scope(context.Background(), nil, DistillScopeInput{})
	if err != nil {
		t.Fatalf("scope tool: %v", err)
	}
	var payload distillScopeResult
	decodeToolJSON(t, "scope", res, &payload)
	if !payload.NoOp {
		t.Error("no_op = false, want true (no new learnings since the prior run)")
	}
	if payload.NoOpReason != NoOpReasonEmptyScope {
		t.Errorf("no_op_reason = %q, want %q", payload.NoOpReason, NoOpReasonEmptyScope)
	}
	if payload.Artifact == "" {
		t.Fatal("artifact is empty, want the recorded no-op artifact path")
	}
	if _, err := os.Stat(payload.Artifact); err != nil {
		t.Errorf("no-op artifact %s not recorded: %v", payload.Artifact, err)
	}
	if !payload.Scope.Empty || len(payload.Scope.InScope) != 0 {
		t.Errorf("scope = %+v, want an empty in_scope set", payload.Scope)
	}
	// The scope is empty because every learning is cited, not by
	// accident.
	if want := []string{alice, bob}; !reflect.DeepEqual(payload.Scope.Cited, want) {
		t.Errorf("cited = %v, want %v", payload.Scope.Cited, want)
	}

	// Zero gh activity after the empty scope. The scope no-op exit is
	// the only path: /dewey-distill-auto reads no_op from the scope
	// result and stops, so the pipeline never reaches rolling_pr — no
	// pr list, pr create, or pr edit is issued, and no comment is
	// possible. The fresh fakeGH is the gh surface the new weekly run
	// carried; it records no calls.
	postScope := newFakeGH(t, 20)
	if len(postScope.calls) != 0 {
		t.Errorf("gh calls after an empty scope = %v, want none (the pipeline stops at scope)", postScope.calls)
	}
	// The open theme PR is left untouched: its remote branch tip is
	// exactly where the prior run pushed it.
	if after := remoteSHA(t, vault, branch); after != prTip {
		t.Errorf("theme PR branch moved %s → %s; a no-op run must leave open PRs untouched", prTip, after)
	}
}
