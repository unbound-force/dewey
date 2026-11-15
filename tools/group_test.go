// Theme grouping tests (specs/018-weekly-distill-automation, T022-T028;
// contracts/grouping.md Behavior and Tests).
package tools

import (
	"context"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// groupFixture is one grouping test's data: identities carry their tag in
// the {tag}-{ts}-{author} shape, texts map identity → embedding input,
// and the mock embedder's vectors key on those texts.
type groupFixture struct {
	inScope    []string
	texts      map[string]string
	supersedes map[string]string
	articles   []GroupArticle
}

// newGroupFixture builds a fixture with n learnings per tag.
func newGroupFixture(tags map[string]int) groupFixture {
	f := groupFixture{
		texts:      make(map[string]string),
		supersedes: make(map[string]string),
	}
	for tag, n := range tags {
		for i := 0; i < n; i++ {
			id := tag + "-20260101T0000" + string(rune('0'+i/10)) + string(rune('0'+i%10)) + "-alice"
			f.inScope = append(f.inScope, id)
			f.texts[id] = tag // embedding input: the tag text
		}
	}
	return f
}

// countingJudge wraps a fixed ruling, counting its calls.
type countingJudge struct {
	ruling GroupJudgement
	calls  int
}

func (j *countingJudge) Judge(_ context.Context, _ []string, _ []GroupArticle, _ []float64) GroupJudgement {
	j.calls++
	return j.ruling
}

// TestGroupLearners_EmbeddingAssignment is the contract's headline fixture
// (contracts/grouping.md Tests): 20 learnings in 4 tag clusters, 3
// existing articles — three clusters assign onto their articles via
// embedding cosine, the fourth (no article, every similarity below
// threshold) opens one new theme. Every decision carries source and
// confidence (FR-010).
func TestGroupLearners_EmbeddingAssignment(t *testing.T) {
	f := newGroupFixture(map[string]int{"auth": 6, "deploy": 5, "vault": 5, "gate": 4})
	f.articles = []GroupArticle{
		{Name: "auth", Path: ".uf/dewey/compiled/auth.md", Text: "auth"},
		{Name: "deploy", Path: ".uf/dewey/compiled/deploy.md", Text: "deploy"},
		{Name: "vault", Path: ".uf/dewey/compiled/vault.md", Text: "vault"},
	}

	emb := newMockEmbedder(true)
	// Cluster vectors are orthogonal axes; the gate cluster points away
	// from every article axis, so its best similarity (0) sits below the
	// default threshold (0.5) and it opens a new theme.
	emb.vectors["auth"] = []float32{1, 0, 0}
	emb.vectors["deploy"] = []float32{0, 1, 0}
	emb.vectors["vault"] = []float32{0, 0, 1}
	emb.vectors["gate"] = []float32{0, 0, -1}

	themes := GroupLearners(context.Background(), GroupInput{
		InScope:  sortedCopy(f.inScope),
		Texts:    f.texts,
		Articles: f.articles,
	}, GroupOptions{Embedder: emb})

	if len(themes) != 4 {
		t.Fatalf("themes = %d (%v), want 4", len(themes), themeNames(themes))
	}
	byName := make(map[string]Theme, len(themes))
	for _, th := range themes {
		byName[th.Name] = th
	}

	wantMembers := map[string]int{"auth": 6, "deploy": 5, "vault": 5, "gate": 4}
	for name, want := range wantMembers {
		th, ok := byName[name]
		if !ok {
			t.Fatalf("theme %q missing; got %v", name, themeNames(themes))
		}
		if len(th.Members) != want {
			t.Errorf("theme %q has %d members, want %d", name, len(th.Members), want)
		}
	}
	for _, name := range []string{"auth", "deploy", "vault"} {
		th := byName[name]
		if th.ExistingArticle != ".uf/dewey/compiled/"+name+".md" {
			t.Errorf("theme %q ExistingArticle = %q, want the article path", name, th.ExistingArticle)
		}
		for _, d := range th.Decisions {
			if d.Source != DecisionEmbeddingDistance {
				t.Errorf("theme %q decision for %s: source = %q, want %q", name, d.Identity, d.Source, DecisionEmbeddingDistance)
			}
			if d.TargetTheme != name {
				t.Errorf("theme %q decision for %s: target = %q, want %q", name, d.Identity, d.TargetTheme, name)
			}
			if d.Confidence < 0.99 || d.Confidence > 1.0 {
				t.Errorf("theme %q decision for %s: confidence = %v, want ~1.0 (orthogonal axes)", name, d.Identity, d.Confidence)
			}
		}
	}
	gate := byName["gate"]
	if gate.ExistingArticle != "" {
		t.Errorf("new theme gate ExistingArticle = %q, want empty", gate.ExistingArticle)
	}
	for _, d := range gate.Decisions {
		if d.Source != DecisionTagFallback {
			t.Errorf("gate decision for %s: source = %q, want %q (below threshold → tag logic)", d.Identity, d.Source, DecisionTagFallback)
		}
	}

	// Every member of every theme has exactly one decision (FR-010).
	totalMembers, totalDecisions := 0, 0
	for _, th := range themes {
		totalMembers += len(th.Members)
		totalDecisions += len(th.Decisions)
	}
	if totalMembers != 20 || totalDecisions != 20 {
		t.Errorf("members = %d, decisions = %d; want 20 and 20 (every decision logged)", totalMembers, totalDecisions)
	}
}

// TestGroupLearners_SupersessionPairSameTheme verifies FR-009: an
// explicit supersedes pair colocates in one theme, the link is recorded
// in the theme metadata, and the non-representative member's decision
// names the coercion. An edge whose target is cited (out of scope) unions
// nothing but still records; a superseded_by half-link heals into the
// same component.
func TestGroupLearners_SupersessionPairSameTheme(t *testing.T) {
	f := newGroupFixture(map[string]int{"auth": 0})
	f.inScope = []string{
		"auth-20260101T000001-alice", // pair target
		"auth-20260101T000002-bob",   // pair successor
		"auth-20260101T000003-carol", // cites nothing, supersedes a cited identity
	}
	f.texts = map[string]string{
		"auth-20260101T000001-alice": "auth",
		"auth-20260101T000002-bob":   "auth",
		"auth-20260101T000003-carol": "auth",
	}
	f.supersedes = map[string]string{
		"auth-20260101T000002-bob":   "auth-20260101T000001-alice",
		"auth-20260101T000003-carol": "auth-20260101T000099-zoe", // cited target, not in scope
	}
	f.articles = []GroupArticle{{Name: "auth", Path: ".uf/dewey/compiled/auth.md", Text: "auth"}}

	themes := GroupLearners(context.Background(), GroupInput{
		InScope:    f.inScope,
		Supersedes: f.supersedes,
		Texts:      f.texts,
		Articles:   f.articles,
	}, GroupOptions{}) // no embedder → tag fallback

	if len(themes) != 1 {
		t.Fatalf("themes = %d (%v), want 1 (pair colocates)", len(themes), themeNames(themes))
	}
	th := themes[0]
	if len(th.Members) != 3 {
		t.Errorf("members = %v, want all three in one theme", th.Members)
	}
	wantLinks := []SupersessionLink{
		{Successor: "auth-20260101T000002-bob", Target: "auth-20260101T000001-alice"},
		{Successor: "auth-20260101T000003-carol", Target: "auth-20260101T000099-zoe"},
	}
	if !reflect.DeepEqual(th.Supersessions, wantLinks) {
		t.Errorf("Supersessions = %v, want %v (in-scope pair and cited-target edge both recorded)", th.Supersessions, wantLinks)
	}

	// The representative (lexicographically smallest) decides; the
	// successor rides by coercion with confidence 1.0 (explicit link).
	byIdentity := make(map[string]AssignmentDecision, len(th.Decisions))
	for _, d := range th.Decisions {
		byIdentity[d.Identity] = d
	}
	ride := byIdentity["auth-20260101T000002-bob"]
	if ride.Source != DecisionSupersessionCoercion {
		t.Errorf("successor decision source = %q, want %q", ride.Source, DecisionSupersessionCoercion)
	}
	if ride.Confidence != 1.0 {
		t.Errorf("successor confidence = %v, want 1.0 (explicit link)", ride.Confidence)
	}
	if rep := byIdentity["auth-20260101T000001-alice"]; rep.Source != DecisionTagFallback {
		t.Errorf("representative decision source = %q, want %q", rep.Source, DecisionTagFallback)
	}
}

// TestGroupLearners_HalfLinkHeals verifies the superseded_by direction:
// a legacy half-link (target records superseded_by, successor claims
// nothing) still colocates the pair (D-004: both directions consumed).
func TestGroupLearners_HalfLinkHeals(t *testing.T) {
	inScope := []string{
		"auth-20260101T000001-alice",
		"auth-20260101T000002-bob",
	}
	themes := GroupLearners(context.Background(), GroupInput{
		InScope:      inScope,
		SupersededBy: map[string]string{"auth-20260101T000001-alice": "auth-20260101T000002-bob"},
		Articles:     []GroupArticle{{Name: "auth", Path: ".uf/dewey/compiled/auth.md"}},
	}, GroupOptions{})

	if len(themes) != 1 || len(themes[0].Members) != 2 {
		t.Fatalf("themes = %v, want one theme holding both members (half-link healed)", themes)
	}
	want := []SupersessionLink{{Successor: "auth-20260101T000002-bob", Target: "auth-20260101T000001-alice"}}
	if !reflect.DeepEqual(themes[0].Supersessions, want) {
		t.Errorf("Supersessions = %v, want %v", themes[0].Supersessions, want)
	}
}

// TestGroupLearners_TagFallbackDegraded verifies the fallback when
// embeddings are unavailable (edge case): same tag → same theme, an
// exact article-name match assigns, unknown tags open new themes, and
// every decision says it came from the degraded mode.
func TestGroupLearners_TagFallbackDegraded(t *testing.T) {
	f := newGroupFixture(map[string]int{"auth": 2, "deploy": 2})
	f.articles = []GroupArticle{{Name: "auth", Path: ".uf/dewey/compiled/auth.md", Text: "auth"}}

	themes := GroupLearners(context.Background(), GroupInput{
		InScope:  sortedCopy(f.inScope),
		Texts:    f.texts,
		Articles: f.articles,
	}, GroupOptions{Embedder: newMockEmbedder(false)}) // unavailable

	if len(themes) != 2 {
		t.Fatalf("themes = %d (%v), want 2", len(themes), themeNames(themes))
	}
	byName := map[string]Theme{}
	for _, th := range themes {
		byName[th.Name] = th
	}
	if th := byName["auth"]; len(th.Members) != 2 || th.ExistingArticle == "" {
		t.Errorf("auth theme = %+v, want 2 members on the existing article", th)
	}
	if th := byName["deploy"]; len(th.Members) != 2 || th.ExistingArticle != "" {
		t.Errorf("deploy theme = %+v, want 2 members on a new theme", th)
	}
	for _, th := range themes {
		for _, d := range th.Decisions {
			if d.Source != DecisionTagFallback {
				t.Errorf("decision for %s: source = %q, want %q", d.Identity, d.Source, DecisionTagFallback)
			}
			if !strings.Contains(d.Details, "degraded mode") {
				t.Errorf("decision for %s: details %q should name the degraded mode", d.Identity, d.Details)
			}
			if d.Confidence != tagMatchConfidence {
				t.Errorf("decision for %s: confidence = %v, want %v", d.Identity, d.Confidence, tagMatchConfidence)
			}
		}
	}
}

// TestGroupLearners_TieBreaker verifies the deterministic tie path
// (contracts/grouping.md step 4): when the top two candidates sit within
// the margin, the closest centroid wins and the decision logs the
// ambiguity — both candidates with their similarities — without calling
// any model.
func TestGroupLearners_TieBreaker(t *testing.T) {
	emb := newMockEmbedder(true)
	emb.vectors["auth"] = []float32{1, 0}
	emb.vectors["authx"] = []float32{1, 0}    // learning text
	emb.vectors["near-a"] = []float32{1, 0}   // article A: sim 1.0
	emb.vectors["near-b"] = []float32{99, 14} // article B: sim ≈ 0.990

	themes := GroupLearners(context.Background(), GroupInput{
		InScope: []string{"auth-20260101T000001-alice"},
		Texts:   map[string]string{"auth-20260101T000001-alice": "authx"},
		Articles: []GroupArticle{
			{Name: "near-a", Path: ".uf/dewey/compiled/near-a.md", Text: "near-a"},
			{Name: "near-b", Path: ".uf/dewey/compiled/near-b.md", Text: "near-b"},
		},
	}, GroupOptions{Embedder: emb})

	if len(themes) != 1 {
		t.Fatalf("themes = %d, want 1", len(themes))
	}
	d := themes[0].Decisions[0]
	if d.Source != DecisionTieBreaker {
		t.Fatalf("decision source = %q, want %q", d.Source, DecisionTieBreaker)
	}
	if d.TargetTheme != "near-a" {
		t.Errorf("tie-breaker chose %q, want near-a (higher similarity)", d.TargetTheme)
	}
	if !strings.Contains(d.Details, "near-a") || !strings.Contains(d.Details, "near-b") {
		t.Errorf("tie details %q should log both candidates", d.Details)
	}
}

// TestGroupLearners_ExactTieBreaksByName: identical similarities tie and
// resolve to the lexicographically smaller article name — the same input
// always groups the same way (FR-005 determinism).
func TestGroupLearners_ExactTieBreaksByName(t *testing.T) {
	emb := newMockEmbedder(true)
	emb.vectors["authx"] = []float32{1, 0}
	emb.vectors["aa"] = []float32{1, 0}
	emb.vectors["bb"] = []float32{1, 0}

	for run := 0; run < 5; run++ {
		themes := GroupLearners(context.Background(), GroupInput{
			InScope: []string{"auth-20260101T000001-alice"},
			Texts:   map[string]string{"auth-20260101T000001-alice": "authx"},
			Articles: []GroupArticle{
				{Name: "bb", Path: ".uf/dewey/compiled/bb.md", Text: "bb"},
				{Name: "aa", Path: ".uf/dewey/compiled/aa.md", Text: "aa"},
			},
		}, GroupOptions{Embedder: emb})
		if len(themes) != 1 || themes[0].Decisions[0].TargetTheme != "aa" {
			t.Fatalf("run %d: exact tie resolved to %v, want aa every time", run, themes)
		}
	}
}

// TestGroupLearners_JudgeSeam verifies the T026 scaffold: a configured
// judge rules ties (its ruling wins even against the higher similarity,
// its identity lands in the decision log), a ruling naming a non-candidate
// is ignored in favor of the deterministic path, and the judge is never
// consulted without a tie.
func TestGroupLearners_JudgeSeam(t *testing.T) {
	emb := newMockEmbedder(true)
	emb.vectors["authx"] = []float32{1, 0}
	emb.vectors["near-a"] = []float32{1, 0}
	emb.vectors["near-b"] = []float32{99, 14}
	articles := []GroupArticle{
		{Name: "near-a", Path: ".uf/dewey/compiled/near-a.md", Text: "near-a"},
		{Name: "near-b", Path: ".uf/dewey/compiled/near-b.md", Text: "near-b"},
	}
	input := GroupInput{
		InScope:  []string{"auth-20260101T000001-alice"},
		Texts:    map[string]string{"auth-20260101T000001-alice": "authx"},
		Articles: articles,
	}

	// A ruling for near-b wins although near-a has the higher similarity.
	judge := &countingJudge{ruling: GroupJudgement{ArticleName: "near-b", Confidence: 0.7, JudgeIdentity: "jev-1"}}
	themes := GroupLearners(context.Background(), input, GroupOptions{Embedder: emb, Judge: judge})
	if judge.calls != 1 {
		t.Fatalf("judge calls = %d, want 1 (only ties consult the judge)", judge.calls)
	}
	d := themes[0].Decisions[0]
	if d.Source != DecisionLLMJudge || d.TargetTheme != "near-b" {
		t.Errorf("decision = %+v, want llm_judge for near-b", d)
	}
	if !strings.Contains(d.Details, "jev-1") {
		t.Errorf("details %q should name the judge identity (FR-010)", d.Details)
	}
	if math.Abs(d.Confidence-0.7) > 1e-9 {
		t.Errorf("confidence = %v, want the judge's 0.7", d.Confidence)
	}

	// A corrupt ruling (non-candidate article) falls back to the
	// deterministic closest-centroid winner.
	judge = &countingJudge{ruling: GroupJudgement{ArticleName: "does-not-exist", Confidence: 0.9, JudgeIdentity: "jev-1"}}
	themes = GroupLearners(context.Background(), input, GroupOptions{Embedder: emb, Judge: judge})
	d = themes[0].Decisions[0]
	if d.Source != DecisionEmbeddingDistance || d.TargetTheme != "near-a" {
		t.Errorf("corrupt ruling decision = %+v, want the deterministic embedding_distance for near-a", d)
	}

	// No tie (single strong candidate): the judge is never called.
	judge = &countingJudge{ruling: GroupJudgement{ArticleName: "near-b", JudgeIdentity: "jev-1"}}
	single := GroupInput{
		InScope:  []string{"auth-20260101T000001-alice"},
		Texts:    map[string]string{"auth-20260101T000001-alice": "authx"},
		Articles: []GroupArticle{{Name: "near-a", Path: ".uf/dewey/compiled/near-a.md", Text: "near-a"}},
	}
	GroupLearners(context.Background(), single, GroupOptions{Embedder: emb, Judge: judge})
	if judge.calls != 0 {
		t.Errorf("judge calls = %d without a tie, want 0 (no judge call when not needed)", judge.calls)
	}
}

// TestCollectGroupInput_GitLeg assembles GroupInput from the real-git
// fixture: in-scope texts from learning blobs, supersession edges decoded
// through the compile boundary, and the citing article with its text.
func TestCollectGroupInput_GitLeg(t *testing.T) {
	dir, _ := initDistillFixtureRepo(t)

	// Give one learning a supersedes edge, committed at the ref.
	const successor = "deploy-20260101T000003-carol"
	const target = "auth-20260101T000001-alice"
	content := "---\ntag: deploy\nauthor: alice\ntier: draft\nsupersedes: \"" + target + "\"\n---\nreplaces the auth note\n"
	path := filepath.Join(dir, ".uf/dewey/learnings", successor+".md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write learning: %v", err)
	}
	git := func(args ...string) {
		t.Helper()
		full := append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)
		if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("add", ".")
	git("commit", "-q", "-m", "add supersession edge")
	git("update-ref", "refs/remotes/origin/main", "HEAD")

	scope, err := ComputeScope(context.Background(), dir, nil, DistillPolicy{})
	if err != nil {
		t.Fatalf("ComputeScope: %v", err)
	}
	input, err := CollectGroupInput(context.Background(), dir, scope)
	if err != nil {
		t.Fatalf("CollectGroupInput: %v", err)
	}

	if len(input.Texts) != len(scope.InScope) {
		t.Errorf("texts = %d, want one per in-scope learning (%d)", len(input.Texts), len(scope.InScope))
	}
	if got := input.Supersedes[successor]; got != target {
		t.Errorf("Supersedes[%s] = %q, want %q", successor, got, target)
	}
	if len(input.Articles) != 1 || input.Articles[0].Name != "auth" {
		t.Fatalf("articles = %+v, want the auth article", input.Articles)
	}
	if !strings.Contains(input.Articles[0].Text, "Authentication") {
		t.Errorf("article text = %q, want the compiled content", input.Articles[0].Text)
	}
}

// TestCollectGroupInput_FSLeg assembles GroupInput from a non-git vault:
// same tree, degraded leg, no error.
func TestCollectGroupInput_FSLeg(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	const id = "auth-20260101T000001-alice"
	write(".uf/dewey/learnings/"+id+".md", "---\ntag: auth\ntier: draft\nsupersedes: \"auth-20260101T000002-bob\"\n---\nbody\n")
	// Cite a different identity: the citation pass must NOT exclude the
	// in-scope learning this test asserts on.
	write(".uf/dewey/compiled/auth.md", "---\ntier: draft\nsources:\n  - auth-20260101T000099-zoe\n---\n# Authentication\n")

	scope, err := ComputeScope(context.Background(), dir, nil, DistillPolicy{})
	if err != nil {
		t.Fatalf("ComputeScope: %v", err)
	}
	input, err := CollectGroupInput(context.Background(), dir, scope)
	if err != nil {
		t.Fatalf("CollectGroupInput: %v", err)
	}
	if input.Texts[id] == "" {
		t.Error("learning text missing from the FS leg")
	}
	if got := input.Supersedes[id]; got != "auth-20260101T000002-bob" {
		t.Errorf("Supersedes[%s] = %q, want the frontmatter edge", id, got)
	}
	if len(input.Articles) != 1 || input.Articles[0].Name != "auth" {
		t.Errorf("articles = %+v, want the auth article", input.Articles)
	}
}

// sortedCopy returns a sorted copy of ids.
func sortedCopy(ids []string) []string {
	out := append([]string(nil), ids...)
	sort.Strings(out)
	return out
}
