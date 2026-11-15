package tools

// End-to-end cycle verification for the weekly unattended distill
// (specs/018-weekly-distill-automation T042; spec.md US6 / SC-007): one
// fixture repository drives the real MCP tool pipeline in dispatch order —
// scope → group_learners → rolling_pr — then the two human-side steps the
// cycle allows (a reviewer's tier: validated suggestion applied by gh-hud's
// feedback task, and the final merge), and finally verifies the #203 trust
// derivation through the exact seam the indexing routes use: a blob
// resolver built by gitref.NewPageBlobResolver over the
// ".uf/dewey/compiled" workspace-layout pathspec feeding
// vault.DeriveEffectiveTier via its frontmatter seam
// (vault.DeriveTierFromProps — the same resolver-plus-derivation
// composition as the tools/curate.go IndexKnowledgeStore precedent).
//
// The derivation MUST yield validated for the article a code-owner
// approved and merged with tier: validated, and draft for the article
// merged still at tier: draft (US6 AS2: eligible for future re-promotion).
// Per the spec's known limitation (unbound-force/dewey#210), the
// dewey doctor / dewey status display of the derivation covers vault-root
// compiled/ and knowledge/ layouts today — this test deliberately does
// NOT assert that display for workspace-layout articles.
//
// The schedule → issue → dispatch trigger stage that precedes the Go
// pipeline is verified against the shipped workflow template in
// workflow_template_test.go (TestDistillWorkflowTemplate_DedupInvariant,
// FR-003/SC-002); the interactive /dewey-distill preservation (both
// HUMAN_GATE questions, full curator tool set, FR-018) is verified in
// cli_test.go (TestDeweyDistill_CuratorBindingAndHumanGates).

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/unbound-force/dewey/v3/gitref"
	"github.com/unbound-force/dewey/v3/vault"
)

// distillWorkspaceCompiled is the ls-tree pathspec covering the distill
// articles' workspace layout (.uf/dewey/compiled) — the pathspec the
// derivation seam composes for distill articles, mirroring how
// IndexKnowledgeStore passes its store-relative directory.
const distillWorkspaceCompiled = ".uf/dewey/compiled"

// deriveWorkspaceArticleTier runs the #203 derivation for one workspace-
// layout distill article exactly as the indexing routes would: parse the
// working-tree file's frontmatter, look up its blob pair through the
// page-blob resolver, and feed vault.DeriveEffectiveTier through its
// frontmatter seam. This is the SC-007 verification seam — it asserts the
// mechanism, not the (pending #210) doctor/status display.
func deriveWorkspaceArticleTier(resolver *gitref.BlobResolver, vaultDir, theme string) (tier, fileSHA, refSHA string) {
	path := distillCompiledPrefix + theme + ".md"
	content, err := os.ReadFile(filepath.Join(vaultDir, filepath.FromSlash(path)))
	if err != nil {
		return "", "", ""
	}
	props, _ := vault.ParseDocument("distill-e2e-"+theme, string(content))
	fileSHA, refSHA = resolver.PageBlobs(path)
	return vault.DeriveTierFromProps(props, fileSHA, refSHA), fileSHA, refSHA
}

// TestSC007_UnattendedCycleYieldsValidatedViaGitVouch walks the full
// unattended cycle on one fixture repo per SC-007: scope computed from
// git alone, themes grouped, rolling per-theme PRs opened at tier:
// draft, a reviewer vouch applied on the PR branch, the human merge —
// and the #203 blob-equality derivation yielding validated for the
// merged, vouched article without any manual intervention beyond the
// reviewer suggestion and the final merge.
func TestSC007_UnattendedCycleYieldsValidatedViaGitVouch(t *testing.T) {
	const alice = "auth-20260101T000001-alice"
	const govAlice = "governance-20260101T000001-alice"
	texts := map[string]string{
		alice:    rollingFixtureLearning("auth", "Alice's auth learning."),
		govAlice: rollingFixtureLearning("governance", "Alice's governance learning."),
	}
	vaultDir := initRollingPRFixture(t, texts)

	// Pin the resolver's highest-precedence leg (vault config) so the
	// fixture resolves origin/main regardless of ambient environment —
	// the same hermetic pattern as initDistillFixtureRepo.
	writeFixtureFile(t, vaultDir, ".uf/dewey/config.yaml", "default_branch_ref: origin/main\n")
	rollingGit(t, vaultDir, "add", ".")
	rollingGit(t, vaultDir, "commit", "-q", "-m", "chore: pin default branch ref for fixture")
	rollingGit(t, vaultDir, "push", "-q", "origin", "main")

	ctx := context.Background()
	d := &Distill{vaultPath: vaultDir, ghRunner: newFakeGH(t, 11)}

	// --- Stage: scope computed from git alone (no question, no-op
	// guard armed by the empty-scope path this test never takes). ---
	scopeRes, _, err := d.Scope(ctx, nil, DistillScopeInput{})
	if err != nil {
		t.Fatalf("scope tool: %v", err)
	}
	var scopePayload distillScopeResult
	decodeToolJSON(t, "scope", scopeRes, &scopePayload)
	wantScope := []string{alice, govAlice}
	if !reflect.DeepEqual(scopePayload.Scope.InScope, wantScope) {
		t.Fatalf("scope in_scope = %v, want %v", scopePayload.Scope.InScope, wantScope)
	}
	if scopePayload.Scope.Empty || scopePayload.NoOp {
		t.Fatal("scope reports empty/no-op; want a live two-learning scope")
	}
	if prov := scopePayload.Scope.Provenance; !strings.HasPrefix(prov, "git:origin/main:") {
		t.Errorf("scope provenance = %q, want git:origin/main:<sha> (the deterministic git leg)", prov)
	}

	// --- Stage: themes grouped (nil embedder — the degraded tag mode
	// is the deterministic fixture path; embeddings unavailable is the
	// documented edge case, not a failure). ---
	groupRes, _, err := d.GroupLearners(ctx, nil, DistillGroupInput{InScope: scopePayload.Scope.InScope})
	if err != nil {
		t.Fatalf("group_learners tool: %v", err)
	}
	var groupPayload distillGroupResult
	decodeToolJSON(t, "group_learners", groupRes, &groupPayload)
	themesByName := make(map[string]Theme, len(groupPayload.Themes))
	for _, theme := range groupPayload.Themes {
		themesByName[theme.Name] = theme
	}
	if len(groupPayload.Themes) != 2 {
		t.Fatalf("themes = %d, want 2 (auth, governance): %+v", len(groupPayload.Themes), groupPayload.Themes)
	}
	authTheme, ok := themesByName["auth"]
	if !ok {
		t.Fatalf("no auth theme in %+v", groupPayload.Themes)
	}
	govTheme, ok := themesByName["governance"]
	if !ok {
		t.Fatalf("no governance theme in %+v", groupPayload.Themes)
	}
	if !reflect.DeepEqual(authTheme.Members, []string{alice}) || !reflect.DeepEqual(govTheme.Members, []string{govAlice}) {
		t.Errorf("theme members = auth:%v governance:%v, want one member each", authTheme.Members, govTheme.Members)
	}

	// --- Stage: rolling per-theme PRs opened at tier: draft. ---
	prRes, _, err := d.RollingPR(ctx, nil, DistillRollingPRInput{Themes: groupPayload.Themes})
	if err != nil {
		t.Fatalf("rolling_pr tool: %v", err)
	}
	var prPayload distillRollingPRResult
	decodeToolJSON(t, "rolling_pr", prRes, &prPayload)
	if len(prPayload.Results) != 2 {
		t.Fatalf("rolling_pr results = %d, want 2", len(prPayload.Results))
	}
	branches := AssignThemeBranches([]string{"auth", "governance"})
	for _, result := range prPayload.Results {
		if result.State != RollingPRCreated {
			t.Errorf("theme %s state = %q, want %q", result.Theme, result.State, RollingPRCreated)
		}
		if result.TierWritten != DistillTierDraft {
			t.Errorf("theme %s tier_written = %q, want %q (the run never writes validated)", result.Theme, result.TierWritten, DistillTierDraft)
		}
		if !containsLabel(result.Labels, "human-gate") {
			t.Errorf("theme %s labels = %v, want human-gate", result.Theme, result.Labels)
		}
	}
	authBranch, govBranch := branches["auth"], branches["governance"]
	for _, article := range []string{"auth", "governance"} {
		content := remoteFile(t, vaultDir, branches[article], distillCompiledPrefix+article+".md")
		if !strings.Contains(content, "tier: draft") {
			t.Errorf("%s article on its PR branch lacks tier: draft:\n%s", article, content)
		}
	}

	// --- Stage: the reviewer's vouch. gh-hud's feedback task applies
	// the tier: validated suggestion on the auth PR branch (T041's
	// contract); the governance article keeps its draft tier. ---
	rollingGit(t, vaultDir, "checkout", "-q", authBranch)
	authDraft := remoteFile(t, vaultDir, authBranch, distillCompiledPrefix+"auth.md")
	writeFixtureFile(t, vaultDir, distillCompiledPrefix+"auth.md", strings.Replace(authDraft, "tier: draft", "tier: validated", 1))
	rollingGit(t, vaultDir, "add", distillCompiledPrefix+"auth.md")
	rollingGit(t, vaultDir, "-c", "user.name=Reviewer", "-c", "user.email=reviewer@example.com",
		"commit", "-q", "-m", "reviewer: accept article at validated")
	rollingGit(t, vaultDir, "push", "-q", "origin", authBranch)
	rollingGit(t, vaultDir, "checkout", "-q", "main")

	// --- Stage: the human merge — the only manual step the cycle
	// allows. Both theme PRs merge: auth carrying the reviewer's
	// validated vouch, governance still at draft (US6 AS2). ---
	rollingGit(t, vaultDir, "merge", "-q", "--no-ff", authBranch, "-m", "merge PR #11: code-owner approved vouch")
	rollingGit(t, vaultDir, "merge", "-q", "--no-ff", govBranch, "-m", "merge PR #12: still evaluating, draft")
	rollingGit(t, vaultDir, "push", "-q", "origin", "main")
	rollingGit(t, vaultDir, "fetch", "-q", "origin")

	// --- Stage: the #203 derivation, verified directly through the
	// seam the indexing routes use (SC-007): one page-blob resolver
	// over the workspace-layout pathspec feeding
	// vault.DeriveEffectiveTier. The dewey doctor/status display for
	// workspace-layout articles is pending unbound-force/dewey#210 and
	// is deliberately not asserted here. ---
	resolver := gitref.NewPageBlobResolver(ctx,
		gitref.Options{VaultDir: vaultDir}, []string{distillWorkspaceCompiled})

	authTier, authFileSHA, authRefSHA := deriveWorkspaceArticleTier(resolver, vaultDir, "auth")
	if authFileSHA == "" || authRefSHA == "" {
		t.Fatalf("auth blob pair = (%q, %q), want non-empty (merged article must be on both sides)", authFileSHA, authRefSHA)
	}
	if authFileSHA != authRefSHA {
		t.Errorf("auth blob equality fails: file %q != default-branch %q", authFileSHA, authRefSHA)
	}
	if authTier != "validated" {
		t.Errorf("auth derived tier = %q, want validated (merged code-owner vouch per #203)", authTier)
	}

	govTier, govFileSHA, govRefSHA := deriveWorkspaceArticleTier(resolver, vaultDir, "governance")
	if govFileSHA == "" || govRefSHA == "" || govFileSHA != govRefSHA {
		t.Errorf("governance blob pair = (%q, %q), want equal non-empty SHAs", govFileSHA, govRefSHA)
	}
	if govTier != "draft" {
		t.Errorf("governance derived tier = %q, want draft (merged at draft stays draft, eligible for future re-promotion — US6 AS2)", govTier)
	}

	// A working-tree edit after the merge withholds the grant
	// (GTS-FR-002): the vouch is git state, never a self-promoting
	// local edit. Restoring the file is not needed — the fixture dies
	// with the test.
	page := filepath.Join(vaultDir, filepath.FromSlash(distillCompiledPrefix+"auth.md"))
	if err := os.WriteFile(page, []byte(strings.Replace(authDraft, "tier: draft", "tier: validated", 1)+"\nlocal edit\n"), 0o644); err != nil {
		t.Fatalf("write local edit: %v", err)
	}
	if editedTier, _, _ := deriveWorkspaceArticleTier(resolver, vaultDir, "auth"); editedTier != "draft" {
		t.Errorf("post-edit derived tier = %q, want draft (a working-tree edit cannot self-promote)", editedTier)
	}
}
