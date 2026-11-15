// Performance-goal validation for the unattended weekly distill
// (specs/018-weekly-distill-automation, Phase 9: T045; plan.md
// Performance Goals). This file pins timing bounds only — the
// correctness suites (scope_test.go, group_test.go, rolling_pr_test.go)
// own behavior. Each test measures one plan goal at its stated scale
// point:
//
//   - scope computation < 2s for ~2k learnings
//   - grouping < 5s for the 500/week steady state
//   - grouping < 30s for the one-time 1,729 backlog bootstrap
//   - PR review-queue body render < 1s per theme
//
// These are validation tests: production code is never modified to
// make a bound pass, and a bound that cannot be met is a gate conflict
// to report, not an assertion to weaken.
package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// perfFixtureDay is the calendar date embedded in perf-fixture
// identities. Any valid YYYYMMDD works — the scope and grouping legs
// parse identity shape, never wall-clock time — and a fixed date keeps
// every fixture deterministic.
const perfFixtureDay = "20261025"

// perfTagName renders fixture tag tagIdx, zero-padded so identity
// parsing and lexicographic ordering stay unambiguous.
func perfTagName(tagIdx int) string {
	return fmt.Sprintf("tag%02d", tagIdx)
}

// perfIdentity renders one fixture learning identity,
// {tag}-{YYYYMMDDTHHMMSS}-{author}, with a one-based sequence in the
// timestamp's seconds field so identities within a tag never collide.
func perfIdentity(tag string, seq int) string {
	return fmt.Sprintf("%s-%sT%06d-fixture", tag, perfFixtureDay, seq+1)
}

// Perf scope fixture shape (initPerfScopeRepo): 40 tags × 50 learnings
// of one file each, plus 10 compiled articles (tags 0-9) citing 5
// distinct learnings apiece — 2,000 learnings, 50 cited, 1,950 in
// scope.
const (
	perfScopeTags            = 40
	perfScopePerTag          = 50
	perfScopeArticleTags     = 10
	perfScopeCitesPerArticle = 5
)

// initPerfScopeRepo builds the scope goal's ~2k scale point as one
// real git repository, reusing initDistillFixtureRepo's hermetic
// pattern: .uf/dewey/config.yaml pins default_branch_ref: origin/main
// (the gitref precedence chain's highest-priority leg), one commit
// carries every file, and refs/remotes/origin/main points at it.
// Fixture build time is excluded from every measured window. Returns
// the vault dir, the total learning count, and the cited count.
func initPerfScopeRepo(t *testing.T) (dir string, learnings, cited int) {
	t.Helper()
	dir = t.TempDir()
	git := func(args ...string) {
		t.Helper()
		full := append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)
		if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	learningsDir := filepath.Join(dir, distillLearningsPrefix)
	compiledDir := filepath.Join(dir, distillCompiledPrefix)
	for _, d := range []string{learningsDir, compiledDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	// Pin the resolver's vault-config leg so ambient environment or
	// global config cannot redirect the fixture.
	if err := os.WriteFile(filepath.Join(dir, ".uf", "dewey", "config.yaml"), []byte("default_branch_ref: origin/main\n"), 0o644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}
	// Learning bodies are constant: the scope's git leg reads learning
	// identities from ls-tree paths and only compiled blobs by content.
	const learningBody = "---\ntier: draft\n---\n# perf fixture learning\n"
	for tag := 0; tag < perfScopeTags; tag++ {
		for seq := 0; seq < perfScopePerTag; seq++ {
			p := filepath.Join(learningsDir, perfIdentity(perfTagName(tag), seq)+".md")
			if err := os.WriteFile(p, []byte(learningBody), 0o644); err != nil {
				t.Fatalf("write learning %s: %v", p, err)
			}
		}
	}
	for tag := 0; tag < perfScopeArticleTags; tag++ {
		var sb strings.Builder
		sb.WriteString("---\ntier: draft\nsources:\n")
		for seq := 0; seq < perfScopeCitesPerArticle; seq++ {
			fmt.Fprintf(&sb, "  - %s\n", perfIdentity(perfTagName(tag), seq))
		}
		fmt.Fprintf(&sb, "---\n\n# %s\n", perfTagName(tag))
		p := filepath.Join(compiledDir, perfTagName(tag)+".md")
		if err := os.WriteFile(p, []byte(sb.String()), 0o644); err != nil {
			t.Fatalf("write compiled %s: %v", p, err)
		}
	}
	git("add", ".")
	git("commit", "-q", "-m", "perf scope fixture")
	git("update-ref", "refs/remotes/origin/main", "HEAD")
	return dir, perfScopeTags * perfScopePerTag, perfScopeArticleTags * perfScopeCitesPerArticle
}

// TestPerfGoal_ScopeTwoThousandUnderTwoSeconds pins plan.md's scope
// goal: ComputeScope over ~2k learnings finishes in under 2 seconds.
// Only the ComputeScope call sits inside the measured window — the
// git fixture build is setup. The scale-point counts are asserted
// exactly (TC-009) so a fast but wrong scope cannot pass.
func TestPerfGoal_ScopeTwoThousandUnderTwoSeconds(t *testing.T) {
	if testing.Short() {
		t.Skip("perf fixture requires subprocesses")
	}
	dir, learningCount, citedCount := initPerfScopeRepo(t)

	start := time.Now()
	scope, err := ComputeScope(context.Background(), dir, nil, DistillPolicy{})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("ComputeScope: %v", err)
	}
	t.Logf("ComputeScope over %d learnings: %s (goal < 2s)", learningCount, elapsed)
	if elapsed >= 2*time.Second {
		t.Errorf("ComputeScope took %s over %d learnings, want < 2s (plan.md Performance Goals)", elapsed, learningCount)
	}

	if scope.Empty {
		t.Error("Empty = true, want false")
	}
	if got := len(scope.AllLearnings); got != learningCount {
		t.Errorf("AllLearnings = %d, want %d", got, learningCount)
	}
	if got := len(scope.Cited); got != citedCount {
		t.Errorf("Cited = %d, want %d", got, citedCount)
	}
	if want := learningCount - citedCount; len(scope.InScope) != want {
		t.Errorf("InScope = %d entries, want %d (learnings minus cited)", len(scope.InScope), want)
	}
	// Spot checks: the first cited identity stays out of scope, an
	// uncited identity from an article-less tag stays in, and the
	// provenance names the pinned git ref.
	inScope := make(map[string]struct{}, len(scope.InScope))
	for _, id := range scope.InScope {
		inScope[id] = struct{}{}
	}
	citedSample := perfIdentity(perfTagName(0), 0)
	if _, ok := inScope[citedSample]; ok {
		t.Errorf("cited identity %s appears in InScope", citedSample)
	}
	uncitedSample := perfIdentity(perfTagName(perfScopeTags-1), perfScopePerTag-1)
	if _, ok := inScope[uncitedSample]; !ok {
		t.Errorf("uncited identity %s missing from InScope", uncitedSample)
	}
	if !strings.HasPrefix(scope.Provenance, "git:origin/main:") {
		t.Errorf("Provenance = %q, want git:origin/main:<sha>", scope.Provenance)
	}
}

// buildPerfGroupInput constructs the grouping goals' scale point:
// count in-scope identities spread round-robin over tagCount tags, a
// Texts entry for every member, and one existing article per tag so
// the deterministic tag-fallback mode (nil embedder) assigns every
// component onto its article's theme. GroupLearners is pure over
// GroupInput — no git, no filesystem — so construction cost stays
// outside the measured window.
func buildPerfGroupInput(count, tagCount int) GroupInput {
	identities := make([]string, count)
	for j := 0; j < count; j++ {
		identities[j] = perfIdentity(perfTagName(j%tagCount), j/tagCount)
	}
	sort.Strings(identities)
	texts := make(map[string]string, count)
	for _, id := range identities {
		texts[id] = "perf learning text for " + id
	}
	articles := make([]GroupArticle, tagCount)
	for tag := 0; tag < tagCount; tag++ {
		articles[tag] = GroupArticle{
			Name: perfTagName(tag),
			Path: distillCompiledPrefix + perfTagName(tag) + ".md",
			Text: "existing compiled article for " + perfTagName(tag),
		}
	}
	return GroupInput{InScope: identities, Texts: texts, Articles: articles}
}

// assertPerfPartition verifies the grouping output partitions the
// identities: every identity appears in exactly one theme's member
// list. The failure output stays bounded — aggregate counts plus the
// first offender — so a broken partition fails loudly without
// flooding one line per identity.
func assertPerfPartition(t *testing.T, themes []Theme, identities []string) {
	t.Helper()
	seen := make(map[string]int, len(identities))
	members := 0
	for _, theme := range themes {
		members += len(theme.Members)
		for _, m := range theme.Members {
			seen[m]++
		}
	}
	if members != len(identities) {
		t.Errorf("themes hold %d members total, want %d", members, len(identities))
	}
	bad, first := 0, ""
	for _, id := range identities {
		if seen[id] != 1 {
			bad++
			if first == "" {
				first = id
			}
		}
	}
	if bad > 0 {
		t.Errorf("%d of %d identities not assigned exactly once (first: %s)", bad, len(identities), first)
	}
}

// TestPerfGoal_GroupingFiveHundredUnderFiveSeconds pins plan.md's
// steady-state grouping goal: GroupLearners over the 500/week scale
// point — nil embedder, the deterministic tag-fallback mode — finishes
// in under 5 seconds and partitions all 500 identities.
func TestPerfGoal_GroupingFiveHundredUnderFiveSeconds(t *testing.T) {
	if testing.Short() {
		t.Skip("perf fixture requires subprocesses")
	}
	const (
		members  = 500
		tagCount = 30
	)
	input := buildPerfGroupInput(members, tagCount)

	start := time.Now()
	themes := GroupLearners(context.Background(), input, GroupOptions{Embedder: nil})
	elapsed := time.Since(start)
	t.Logf("GroupLearners over %d identities: %s (goal < 5s)", members, elapsed)
	if elapsed >= 5*time.Second {
		t.Errorf("GroupLearners took %s over %d identities, want < 5s (plan.md Performance Goals)", elapsed, members)
	}
	assertPerfPartition(t, themes, input.InScope)
}

// TestPerfGoal_GroupingBacklog1729UnderThirtySeconds pins plan.md's
// backlog-bootstrap grouping goal: GroupLearners over the one-time
// 1,729-learning scale point finishes in under 30 seconds (a watched
// pane is acceptable) and partitions all 1,729 identities.
func TestPerfGoal_GroupingBacklog1729UnderThirtySeconds(t *testing.T) {
	if testing.Short() {
		t.Skip("perf fixture requires subprocesses")
	}
	const (
		members  = 1729
		tagCount = 60
	)
	input := buildPerfGroupInput(members, tagCount)

	start := time.Now()
	themes := GroupLearners(context.Background(), input, GroupOptions{Embedder: nil})
	elapsed := time.Since(start)
	t.Logf("GroupLearners over %d identities: %s (goal < 30s)", members, elapsed)
	if elapsed >= 30*time.Second {
		t.Errorf("GroupLearners took %s over %d identities, want < 30s (plan.md Performance Goals)", elapsed, members)
	}
	assertPerfPartition(t, themes, input.InScope)
}

// TestPerfGoal_ReviewQueueBodyUnderOneSecond pins plan.md's PR body
// goal: rendering one realistic large theme's review-queue section —
// 100 newly cited members, 100 assignment decisions, supersession
// pairs, and lint lines — finishes in under 1 second and produces the
// expected body structure.
func TestPerfGoal_ReviewQueueBodyUnderOneSecond(t *testing.T) {
	if testing.Short() {
		t.Skip("perf fixture requires subprocesses")
	}
	const members = 100
	newlyCited := make([]string, members)
	decisions := make([]AssignmentDecision, members)
	for i := 0; i < members; i++ {
		id := perfIdentity("auth", i)
		newlyCited[i] = id
		decisions[i] = AssignmentDecision{
			Identity:    id,
			TargetTheme: "auth",
			Source:      DecisionTagFallback,
			Confidence:  0.8,
		}
	}
	supersessions := make([]string, 5)
	for i := range supersessions {
		supersessions[i] = perfIdentity("auth", 2*i) + " supersedes " + perfIdentity("auth", 2*i+1)
	}
	article := ReviewQueueArticle{
		Title:         "auth",
		Path:          distillCompiledPrefix + "auth.md",
		WhatChanged:   "100 learnings added (100 cited total)",
		NewlyCited:    newlyCited,
		Supersessions: supersessions,
		Lint:          []string{"anchor_drift", "embedding_gap", "citation_coverage"},
		ConfidenceAvg: 0.8,
		Decisions:     decisions,
	}

	start := time.Now()
	body := RenderReviewQueueBody([]ReviewQueueArticle{article})
	elapsed := time.Since(start)
	t.Logf("RenderReviewQueueBody for a %d-member theme: %s (goal < 1s per theme)", members, elapsed)
	if elapsed >= time.Second {
		t.Errorf("RenderReviewQueueBody took %s for a %d-member theme, want < 1s per theme (plan.md Performance Goals)", elapsed, members)
	}
	if body == "" {
		t.Fatal("body is empty, want the rendered review queue")
	}
	for _, want := range []string{
		"human-gate",
		"## auth (`" + distillCompiledPrefix + "auth.md`)",
		"**What changed**: 100 learnings added \\(100 cited total\\)",
		"**Newly cited**",
		"(100)",
		"**Supersessions**",
		"**Lint**",
		"**Confidence**: 0.80 avg",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
	// Exactly 100 items sit at the truncation bound: the last identity
	// renders and no truncation note fires.
	if !strings.Contains(body, newlyCited[members-1]) {
		t.Errorf("body missing the %dth identity %s; list truncated early", members, newlyCited[members-1])
	}
	if strings.Contains(body, "list truncated") {
		t.Errorf("body reports list truncation at exactly %d items, want none", maxReviewQueueListItems)
	}
}
