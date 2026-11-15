// Package tools — theme grouping for the unattended weekly distill
// (specs/018-weekly-distill-automation, issue #193; contracts/grouping.md).
//
// Grouping composes three mechanisms (research.md D-003/D-004):
// must-colocate components merged from explicit supersession edges before
// any assignment (FR-009), embeddings-first assignment onto existing
// compiled articles with a deterministic closest-centroid tie-breaker
// (FR-008/FR-010), and a degraded tag-fallback mode when embeddings are
// unavailable. The decision log is the output's spine: every member
// carries an AssignmentDecision naming its source, confidence, and
// evidence, so a human reviewer can audit the grouping without re-running
// it. The LLM judge is a nil-by-default seam — MVP grouping never
// consults one (Out of Scope; T026).
package tools

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"gopkg.in/yaml.v3"

	"github.com/unbound-force/dewey/v3/embed"
	"github.com/unbound-force/dewey/v3/gitref"
)

// Grouping defaults (contracts/grouping.md Behavior steps 2-4): an
// embedding assignment needs cosine similarity at or above the threshold,
// and the top two candidates within the tie margin count as a tie the
// deterministic tie-breaker (or a configured judge) resolves.
const (
	// defaultSimilarityThreshold is the minimum cosine similarity for an
	// embedding-distance assignment when GroupOptions does not set one.
	defaultSimilarityThreshold = 0.5
	// defaultTieMargin is the similarity window within which the top two
	// candidates tie when GroupOptions does not set one.
	defaultTieMargin = 0.05
	// tagMatchConfidence is the confidence of an exact tag→article
	// assignment: a strong signal whose provenance is degraded (no
	// semantic evidence), so it never reaches 1.0.
	tagMatchConfidence = 0.8
	// coercionConfidence is the confidence of a member riding its
	// component's supersession edge: the link is explicit human intent,
	// the one grouping input that needs no inference.
	coercionConfidence = 1.0
)

// GroupArticle is one existing compiled article the grouping can assign
// onto (contracts/grouping.md Input: existing compiled articles). The
// article's theme name is its filename minus .md — the same name compiled
// articles are keyed by (compiled/<name>.md) — so a tag matching the name
// is an exact tag match.
type GroupArticle struct {
	// Name is the article's theme name.
	Name string `json:"name"`
	// Path is the vault-relative article path.
	Path string `json:"path"`
	// Text is the article's content, the embedding input.
	Text string `json:"-"`
}

// GroupInput carries one grouping run's data. Assembled from the git
// enumeration by CollectGroupInput, or constructed directly in tests: the
// grouping core is pure over this struct.
type GroupInput struct {
	// InScope lists the identities to group (ScopeSet.InScope), sorted.
	InScope []string `json:"in_scope"`
	// Supersedes maps in-scope identity → the identity it supersedes,
	// from the learning's frontmatter at the enumerated source.
	Supersedes map[string]string `json:"supersedes,omitempty"`
	// SupersededBy maps in-scope identity → its recorded successor — the
	// two-way link's other half. The successor's own supersedes claim is
	// authoritative, but consuming both directions heals legacy
	// half-links (D-004: supersedes/superseded_by links).
	SupersededBy map[string]string `json:"superseded_by,omitempty"`
	// Articles lists the existing compiled articles, sorted by name.
	Articles []GroupArticle `json:"articles,omitempty"`
	// Texts maps in-scope identity → its learning text, the embedding
	// input; a missing entry contributes no embedding and the component
	// falls back to tag logic for that member.
	Texts map[string]string `json:"-"`
}

// GroupOptions configures one grouping run.
type GroupOptions struct {
	// Embedder generates member and article vectors; nil or an
	// unavailable embedder selects the degraded tag-fallback mode
	// (contracts/grouping.md Tests; edge case embeddings unavailable).
	Embedder embed.Embedder
	// SimilarityThreshold is the minimum cosine similarity for an
	// embedding assignment; zero selects defaultSimilarityThreshold.
	SimilarityThreshold float64
	// TieMargin is the similarity window within which the top two
	// candidates count as a tie; zero selects defaultTieMargin.
	TieMargin float64
	// Judge is the future LLM-judge seam (T026): nil — the default and
	// the MVP — never consults a model and grouping stays deterministic;
	// non-nil rules narrow-margin ties server-side with every decision
	// logged.
	Judge GroupJudge
}

// GroupJudgement is a judge's ruling on one ambiguous component.
type GroupJudgement struct {
	// ArticleName names the winning article; empty votes for a new
	// theme.
	ArticleName string
	// Confidence is the judge's confidence in [0,1].
	Confidence float64
	// JudgeIdentity names the judge for the decision log (FR-010).
	JudgeIdentity string
}

// GroupJudge is the seam for a future server-side LLM judge (contracts/
// grouping.md step 5; research.md D-003): given a component whose top
// candidates tie within the margin, it rules which article wins or votes
// for a new theme. Nothing in the MVP constructs one — the judge is
// flag-gated off per Out of Scope — so grouping never calls a model
// unless a future change wires a judge here; the seam exists so that
// wiring touches no grouping logic.
type GroupJudge interface {
	// Judge rules on one ambiguous component. candidates and
	// similarities are parallel arrays sorted best-first; the ruling
	// names an article from candidates or votes new-theme with an empty
	// ArticleName.
	Judge(ctx context.Context, component []string, candidates []GroupArticle, similarities []float64) GroupJudgement
}

// groupComponent is one must-colocate unit: members sorted, the
// representative (lexicographically smallest identity — deterministic)
// whose assignment decision the other members ride, and the supersession
// edges whose successor is a member. A multi-member component exists only
// through supersession edges, so every non-representative member colocates
// by coercion (FR-009).
type groupComponent struct {
	members        []string
	representative string
	edges          []SupersessionLink
}

// buildComponents merges the in-scope identities into must-colocate
// components (contracts/grouping.md Behavior step 1; D-004): union-find
// over the supersession edges, both directions, restricted to pairs whose
// ends are both in scope — an edge whose target is cited or dangling
// records in the successor's component metadata but unions nothing, since
// the target is not part of this run. Components are returned sorted by
// representative; members are sorted.
func buildComponents(input GroupInput) []groupComponent {
	inScope := make(map[string]struct{}, len(input.InScope))
	for _, id := range input.InScope {
		inScope[id] = struct{}{}
	}

	uf := make(unionFind, len(inScope))
	for id := range inScope {
		uf[id] = id
	}
	edges := unionSupersessionEdges(uf, inScope, input)

	byRoot := make(map[string][]string)
	for _, id := range input.InScope { // InScope is sorted; members stay sorted
		root := uf.find(id)
		byRoot[root] = append(byRoot[root], id)
	}
	edgesBySuccessor := make(map[string][]SupersessionLink, len(edges))
	for _, e := range edges {
		edgesBySuccessor[e.Successor] = append(edgesBySuccessor[e.Successor], e)
	}

	components := make([]groupComponent, 0, len(byRoot))
	for _, members := range byRoot {
		components = append(components, assembleComponent(members, edgesBySuccessor))
	}
	sort.Slice(components, func(i, j int) bool {
		return components[i].representative < components[j].representative
	})
	return components
}

// unionSupersessionEdges runs the supersession links through the
// union-find in deterministic order (by successor, then target) and
// returns every link whose successor is in scope. A link whose target is
// also in scope unions the pair; a cited or dangling target only records
// in the successor's metadata, since it is not part of this run.
func unionSupersessionEdges(uf unionFind, inScope map[string]struct{}, input GroupInput) []SupersessionLink {
	var edges []SupersessionLink
	link := func(successor, target string) {
		if _, ok := inScope[successor]; !ok {
			return
		}
		edges = append(edges, SupersessionLink{Successor: successor, Target: target})
		if _, ok := inScope[target]; ok {
			uf.union(successor, target)
		}
	}
	successors := make([]string, 0, len(input.Supersedes))
	for s := range input.Supersedes {
		successors = append(successors, s)
	}
	sort.Strings(successors)
	for _, s := range successors {
		link(s, input.Supersedes[s])
	}
	succeeded := make([]string, 0, len(input.SupersededBy))
	for s := range input.SupersededBy {
		succeeded = append(succeeded, s)
	}
	sort.Strings(succeeded)
	for _, t := range succeeded {
		link(input.SupersededBy[t], t)
	}
	return edges
}

// assembleComponent builds one component from its sorted member list and
// the supersession edges whose successor is a member. A both-ends-in-
// scope pair lands in one component, so recording it once per component
// (deduplicated by pair) keeps Theme.Supersessions free of duplicates
// when the two-way link yields the same edge twice.
func assembleComponent(members []string, edgesBySuccessor map[string][]SupersessionLink) groupComponent {
	comp := groupComponent{
		members:        members,
		representative: members[0],
	}
	seen := make(map[string]struct{}, len(members))
	for _, m := range members {
		seen[m] = struct{}{}
	}
	for _, m := range members {
		for _, e := range edgesBySuccessor[m] {
			if _, dup := seen[e.Successor+"\x00"+e.Target]; dup {
				continue
			}
			seen[e.Successor+"\x00"+e.Target] = struct{}{}
			comp.edges = append(comp.edges, e)
		}
	}
	sort.Slice(comp.edges, func(i, j int) bool {
		if comp.edges[i].Successor != comp.edges[j].Successor {
			return comp.edges[i].Successor < comp.edges[j].Successor
		}
		return comp.edges[i].Target < comp.edges[j].Target
	})
	return comp
}

// unionFind is a path-compressing union-find over string identities.
type unionFind map[string]string

// find returns x's root, compressing the walked path.
func (u unionFind) find(x string) string {
	root := x
	for u[root] != root {
		root = u[root]
	}
	for u[x] != root {
		next := u[x]
		u[x] = root
		x = next
	}
	return root
}

// union merges a's and b's sets.
func (u unionFind) union(a, b string) {
	ra, rb := u.find(a), u.find(b)
	if ra == rb {
		return
	}
	// Deterministic merge direction: the lexicographically smaller root
	// wins, so the same edges always build the same components.
	if rb < ra {
		ra, rb = rb, ra
	}
	u[rb] = ra
}

// componentTag derives a component's dominant tag from its members'
// identity shapes (IdentityTag, FR-005's properties-free parse): the most
// common tag wins, ties break to the lexicographically smallest — the
// same determinism compile's clustering gets from sorted iteration.
func componentTag(members []string) string {
	counts := make(map[string]int, len(members))
	for _, m := range members {
		counts[IdentityTag(m)]++
	}
	best := ""
	bestCount := 0
	// Sorted tag iteration keeps the tie-break deterministic without
	// sorting the map.
	tags := make([]string, 0, len(counts))
	for tag := range counts {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	for _, tag := range tags {
		if counts[tag] > bestCount {
			best, bestCount = tag, counts[tag]
		}
	}
	return best
}

// GroupLearners groups one run's in-scope learnings into themes
// (contracts/grouping.md Behavior): supersession components first, then
// embeddings-first assignment onto existing compiled articles with a
// deterministic tie-breaker, tag fallback when embeddings are
// unavailable, and a nil-by-default judge seam. The returned themes are
// sorted by name; members, decisions, and supersession links within each
// theme are sorted. Grouping is deterministic for a given input and
// options — the one nondeterminism an Embedder brings is its vectors,
// which the decision log records.
func GroupLearners(ctx context.Context, input GroupInput, opts GroupOptions) []Theme {
	threshold := opts.SimilarityThreshold
	if threshold <= 0 {
		threshold = defaultSimilarityThreshold
	}
	margin := opts.TieMargin
	if margin <= 0 {
		margin = defaultTieMargin
	}

	components := buildComponents(input)
	useEmbeddings := opts.Embedder != nil && opts.Embedder.Available()

	// Pre-compute the article vectors once; articles without a vector
	// stay tag-match candidates but leave the similarity ranking.
	var articleVecs map[string][]float32
	if useEmbeddings {
		articleVecs = embedGroupArticles(ctx, opts.Embedder, input.Articles)
	}

	themes := make(map[string]*Theme)
	for _, comp := range components {
		tag := componentTag(comp.members)
		name, existingPath, decision := assignComponent(ctx, comp, tag, input, opts, useEmbeddings, articleVecs, threshold, margin)
		theme := themes[name]
		if theme == nil {
			theme = &Theme{Name: name, ExistingArticle: existingPath}
			themes[name] = theme
		}
		theme.Members = append(theme.Members, comp.members...)
		theme.Supersessions = append(theme.Supersessions, comp.edges...)
		theme.Decisions = append(theme.Decisions, decision)
		for _, m := range comp.members {
			if m == comp.representative {
				continue
			}
			theme.Decisions = append(theme.Decisions, AssignmentDecision{
				Identity:    m,
				TargetTheme: name,
				Source:      DecisionSupersessionCoercion,
				Confidence:  coercionConfidence,
				Details:     "rode component of " + comp.representative + " (" + decision.Source + ")",
			})
		}
	}

	out := make([]Theme, 0, len(themes))
	for _, theme := range themes {
		sort.Strings(theme.Members)
		sort.Slice(theme.Decisions, func(i, j int) bool {
			return theme.Decisions[i].Identity < theme.Decisions[j].Identity
		})
		sort.Slice(theme.Supersessions, func(i, j int) bool {
			if theme.Supersessions[i].Successor != theme.Supersessions[j].Successor {
				return theme.Supersessions[i].Successor < theme.Supersessions[j].Successor
			}
			return theme.Supersessions[i].Target < theme.Supersessions[j].Target
		})
		out = append(out, *theme)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// assignComponent decides one component's theme: the article it joins
// (name + path, empty path for a new theme) and the representative's
// AssignmentDecision. Embedding mode ranks articles by cosine similarity
// of the component centroid against each article vector; a tie within the
// margin goes to the judge when configured, else the deterministic
// closest centroid (candidates are pre-sorted best-first, so the head is
// the deterministic winner). Below the threshold — or in degraded mode —
// the exact tag match applies, else the component opens a new theme named
// by its sanitized dominant tag.
func assignComponent(ctx context.Context, comp groupComponent, tag string, input GroupInput, opts GroupOptions, useEmbeddings bool, articleVecs map[string][]float32, threshold, margin float64) (name, existingPath string, decision AssignmentDecision) {
	decision.Identity = comp.representative
	tagArticle := findTagArticle(input.Articles, tag)

	if !useEmbeddings {
		return tagThemeDecision(tag, tagArticle, input.Articles, decision,
			"degraded mode: exact tag match", "degraded mode: new theme for tag "+tag)
	}

	compVec, vecOk := componentCentroid(ctx, opts.Embedder, comp, input.Texts)
	if vecOk {
		candidates := rankArticleCandidates(compVec, input.Articles, articleVecs)
		// A tie only matters among viable candidates: when the best
		// similarity sits below the threshold, no candidate wins and
		// the tag logic decides (a tie at similarity 0 must open a new
		// theme, not pick the least-bad article).
		if len(candidates) > 0 && candidates[0].sim >= threshold {
			return chooseCandidateTheme(ctx, comp, tag, candidates, opts, threshold, margin, decision)
		}
	}
	// No vector (or no candidate above threshold): tag logic decides.
	return tagThemeDecision(tag, tagArticle, input.Articles, decision,
		"exact tag match (embedding evidence below threshold)", "new theme for tag "+tag)
}

// findTagArticle returns the index of the article named tag, or -1.
func findTagArticle(articles []GroupArticle, tag string) int {
	for i, a := range articles {
		if a.Name == tag {
			return i
		}
	}
	return -1
}

// tagThemeDecision decides by exact tag match: the article named tag
// when one exists, else a new theme named by the sanitized tag. The
// detail strings are per call site (degraded mode and the embedding
// fallback word them differently).
func tagThemeDecision(tag string, tagArticle int, articles []GroupArticle, decision AssignmentDecision, matchDetail, newThemeDetail string) (string, string, AssignmentDecision) {
	decision.Source = DecisionTagFallback
	decision.Confidence = tagMatchConfidence
	if tagArticle >= 0 {
		decision.TargetTheme = articles[tagArticle].Name
		decision.Details = matchDetail
		return articles[tagArticle].Name, articles[tagArticle].Path, decision
	}
	decision.TargetTheme = normalizeThemeName(tag)
	decision.Details = newThemeDetail
	return normalizeThemeName(tag), "", decision
}

// articleCandidate is one ranked (article, centroid-similarity) pair.
type articleCandidate struct {
	article GroupArticle
	sim     float64
}

// rankArticleCandidates ranks the articles with embedded vectors by
// cosine similarity of their vectors against centroid, best-first; ties
// break by article name, so the head is the deterministic winner.
func rankArticleCandidates(centroid []float32, articles []GroupArticle, articleVecs map[string][]float32) []articleCandidate {
	candidates := make([]articleCandidate, 0, len(articles))
	for _, a := range articles {
		if vec, ok := articleVecs[a.Name]; ok {
			candidates = append(candidates, articleCandidate{article: a, sim: cosineSim(centroid, vec)})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].sim != candidates[j].sim {
			return candidates[i].sim > candidates[j].sim
		}
		return candidates[i].article.Name < candidates[j].article.Name
	})
	return candidates
}

// chooseCandidateTheme decides among viable embedding candidates: a tie
// within the margin goes to the judge when configured — its ruling,
// winner or new theme, is final — else the deterministic closest
// centroid (candidates are pre-sorted best-first, so the head is the
// deterministic winner).
func chooseCandidateTheme(ctx context.Context, comp groupComponent, tag string, candidates []articleCandidate, opts GroupOptions, threshold, margin float64, decision AssignmentDecision) (string, string, AssignmentDecision) {
	best := candidates[0]
	tied := len(candidates) > 1 && best.sim-candidates[1].sim <= margin
	if tied && opts.Judge != nil {
		// The judge seam (T026): only a configured judge reaches here,
		// and its ruling — winner or new theme — is logged with its
		// identity per FR-010. MVP runs never take this branch: Judge
		// is nil.
		ruling := opts.Judge.Judge(ctx, comp.members, candidateArticles(candidates), candidateSims(candidates))
		if name, path, judged, ok := applyJudgeRuling(tag, candidates, ruling, margin, decision); ok {
			return name, path, judged
		}
		// A ruling naming a non-candidate is corrupt: fall through to
		// the deterministic path rather than honor it.
		return deterministicCandidateTheme(best, threshold, decision)
	}
	if tied {
		return tieBreakerCandidateTheme(best, candidates[1], margin, decision)
	}
	return deterministicCandidateTheme(best, threshold, decision)
}

// candidateArticles projects the ranked candidates into the judge's
// article slice.
func candidateArticles(candidates []articleCandidate) []GroupArticle {
	arts := make([]GroupArticle, len(candidates))
	for i, c := range candidates {
		arts[i] = c.article
	}
	return arts
}

// candidateSims projects the ranked candidates into the judge's
// similarity slice.
func candidateSims(candidates []articleCandidate) []float64 {
	sims := make([]float64, len(candidates))
	for i, c := range candidates {
		sims[i] = c.sim
	}
	return sims
}

// applyJudgeRuling translates a judge's tie ruling into a theme
// decision: a new-theme ruling (empty article name) opens the tag theme;
// a ruling naming a candidate joins it; a ruling naming a non-candidate
// is corrupt and reports ok=false so the deterministic path decides.
func applyJudgeRuling(tag string, candidates []articleCandidate, ruling GroupJudgement, margin float64, decision AssignmentDecision) (string, string, AssignmentDecision, bool) {
	decision.Source = DecisionLLMJudge
	decision.Confidence = clamp01(ruling.Confidence)
	decision.Details = "judge " + ruling.JudgeIdentity + " ruled a tie within " + formatFloat(margin)
	if ruling.ArticleName == "" {
		decision.TargetTheme = normalizeThemeName(tag)
		return normalizeThemeName(tag), "", decision, true
	}
	for _, c := range candidates {
		if c.article.Name == ruling.ArticleName {
			decision.TargetTheme = c.article.Name
			return c.article.Name, c.article.Path, decision, true
		}
	}
	return "", "", decision, false
}

// deterministicCandidateTheme joins the best candidate as a plain
// embedding-distance assignment.
func deterministicCandidateTheme(best articleCandidate, threshold float64, decision AssignmentDecision) (string, string, AssignmentDecision) {
	decision.Source = DecisionEmbeddingDistance
	decision.Confidence = best.sim
	decision.TargetTheme = best.article.Name
	decision.Details = "cosine " + formatFloat(best.sim) + " ≥ threshold " + formatFloat(threshold)
	return best.article.Name, best.article.Path, decision
}

// tieBreakerCandidateTheme joins the best candidate by closest
// centroid, logging the runner-up it beat.
func tieBreakerCandidateTheme(best, runnerUp articleCandidate, margin float64, decision AssignmentDecision) (string, string, AssignmentDecision) {
	decision.Source = DecisionTieBreaker
	decision.Confidence = best.sim
	decision.TargetTheme = best.article.Name
	decision.Details = "tie within " + formatFloat(margin) + ": " + best.article.Name + " (" + formatFloat(best.sim) + ") over " + runnerUp.article.Name + " (" + formatFloat(runnerUp.sim) + ") by closest centroid"
	return best.article.Name, best.article.Path, decision
}

// embedGroupArticles embeds every article text, best-effort: an article
// whose embedding fails stays a tag-match candidate without a vector, and
// the run proceeds — one bad article never blocks grouping.
func embedGroupArticles(ctx context.Context, e embed.Embedder, articles []GroupArticle) map[string][]float32 {
	vecs := make(map[string][]float32, len(articles))
	for _, a := range articles {
		if a.Text == "" {
			continue
		}
		if v, err := e.Embed(ctx, a.Text); err == nil && len(v) > 0 {
			vecs[a.Name] = v
		}
	}
	return vecs
}

// componentCentroid computes the component's embedding centroid: the
// element-wise mean of its members' vectors (members without text or
// whose embedding fails contribute nothing). The boolean reports whether
// at least one member vector exists.
func componentCentroid(ctx context.Context, e embed.Embedder, comp groupComponent, texts map[string]string) ([]float32, bool) {
	var withText []string
	for _, m := range comp.members {
		if texts[m] != "" {
			withText = append(withText, m)
		}
	}
	if len(withText) == 0 {
		return nil, false
	}
	vecs, err := e.EmbedBatch(ctx, memberTexts(withText, texts))
	if err != nil || len(vecs) != len(withText) {
		return nil, false
	}
	var centroid []float32
	for _, v := range vecs {
		if len(v) == 0 {
			continue
		}
		if centroid == nil {
			centroid = make([]float32, len(v))
		}
		for i := range v {
			if i < len(centroid) {
				centroid[i] += v[i]
			}
		}
	}
	if centroid == nil {
		return nil, false
	}
	for i := range centroid {
		centroid[i] /= float32(len(withText))
	}
	return centroid, true
}

// memberTexts collects the embedded texts in member order.
func memberTexts(members []string, texts map[string]string) []string {
	out := make([]string, len(members))
	for i, m := range members {
		out[i] = texts[m]
	}
	return out
}

// cosineSim is the tools-side cosine similarity — the same computation as
// store.cosineSimilarity, kept unexported here per the package's
// self-containment convention (see frontmatterBlock's note).
func cosineSim(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}

// clamp01 bounds v to [0,1] — a judge's confidence is contract data, not
// trusted arithmetic.
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// formatFloat renders a similarity for the decision log without
// locale-dependent formatting; two decimals match the review-queue body's
// confidence rendering.
func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', 2, 64)
}

// --- Input assembly (the grouping legs over git or filesystem) ---

// CollectGroupInput assembles GroupInput for one run over vaultDir at the
// same source ComputeScope enumerated (research.md D-004: "same ls-tree
// pass"): every in-scope learning's text and supersedes/superseded_by
// frontmatter edges, plus the existing compiled articles with their text.
// The git leg reads blobs at their ls-tree shas through the shared Runner
// seam; when the ref does not resolve or enumeration fails, the
// filesystem leg reads the same tree directly (degraded: no blob-equality
// guarantees, same data). A learning whose blob or file cannot be read
// keeps its identity in scope — it groups by tag — and loses only its
// embedding vote; the miss is logged, never fatal.
func CollectGroupInput(ctx context.Context, vaultDir string, scope ScopeSet) (GroupInput, error) {
	if vaultDir == "" {
		return GroupInput{}, fmt.Errorf("distill group: vault dir is empty")
	}
	info, err := os.Stat(vaultDir)
	if err != nil {
		return GroupInput{}, fmt.Errorf("distill group: stat vault dir: %w", err)
	}
	if !info.IsDir() {
		return GroupInput{}, fmt.Errorf("distill group: vault dir %s is not a directory", vaultDir)
	}

	input := GroupInput{
		InScope:      append([]string(nil), scope.InScope...),
		Supersedes:   make(map[string]string),
		SupersededBy: make(map[string]string),
		Texts:        make(map[string]string),
	}

	res := gitref.New(gitref.Options{VaultDir: vaultDir}).Resolve(ctx)
	if res.Resolved() {
		if tree, ok := enumerateDistillTreeGit(ctx, nil, vaultDir, res); ok {
			collectGroupInputGit(ctx, vaultDir, &input, tree)
			return input, nil
		}
		scopeLogger.Warn("distill group: git enumeration failed at resolved ref, assembling from filesystem",
			"ref", res.Ref, "reason", res.Reason)
	} else {
		scopeLogger.Warn("distill group: default-branch ref unresolved, assembling from filesystem",
			"ref", res.Ref, "reason", res.Reason)
	}
	collectGroupInputFS(vaultDir, &input)
	return input, nil
}

// collectGroupInputGit fills input from the enumerated git tree: learning
// and article texts via blob reads at their ls-tree shas, supersession
// edges from learning frontmatter.
func collectGroupInputGit(ctx context.Context, vaultDir string, input *GroupInput, tree *gitDistillTree) {
	run := gitref.ExecRunner{}
	for _, id := range input.InScope {
		sha, ok := tree.blobs[distillLearningsPrefix+id+".md"]
		if !ok {
			continue
		}
		content, ok := blobContent(ctx, run, vaultDir, sha)
		if !ok {
			scopeLogger.Warn("distill group: learning blob unreadable, grouping by tag", "identity", id)
			continue
		}
		input.Texts[id] = content
		recordLearningEdges(input, id, content)
	}
	for _, p := range tree.compiledPaths() {
		sha := tree.blobs[p]
		content, ok := blobContent(ctx, run, vaultDir, sha)
		if !ok {
			scopeLogger.Warn("distill group: compiled blob unreadable, article stays a tag-match candidate", "path", p)
			continue
		}
		input.Articles = append(input.Articles, GroupArticle{
			Name: learningIdentityFromPath(p),
			Path: p,
			Text: content,
		})
	}
}

// collectGroupInputFS fills input from the filesystem: the degraded leg
// for non-git vaults, reading the same tree ComputeScope's fallback
// scanned.
func collectGroupInputFS(vaultDir string, input *GroupInput) {
	for _, id := range input.InScope {
		content, err := os.ReadFile(filepath.Join(vaultDir, distillLearningsPrefix+id+".md"))
		if err != nil {
			scopeLogger.Warn("distill group: learning file unreadable, grouping by tag", "identity", id)
			continue
		}
		input.Texts[id] = string(content)
		recordLearningEdges(input, id, string(content))
	}
	for _, name := range listMarkdownFiles(filepath.Join(vaultDir, distillCompiledPrefix)) {
		rel := distillCompiledPrefix + name
		content, err := os.ReadFile(filepath.Join(vaultDir, rel))
		if err != nil {
			scopeLogger.Warn("distill group: compiled file unreadable, article stays a tag-match candidate", "path", rel)
			continue
		}
		input.Articles = append(input.Articles, GroupArticle{
			Name: learningIdentityFromPath(rel),
			Path: rel,
			Text: string(content),
		})
	}
}

// recordLearningEdges decodes one learning's supersession edges from its
// markdown frontmatter through the compile decode boundary
// (decodeTemporalEntryFields): values are normalized and
// learning-identity-validated, so a hostile or corrupt frontmatter can
// contribute no edge.
func recordLearningEdges(input *GroupInput, identity, content string) {
	block, ok := frontmatterBlock(content)
	if !ok {
		return
	}
	var props map[string]any
	if err := yaml.Unmarshal([]byte(block), &props); err != nil {
		return
	}
	fields := decodeTemporalEntryFields(props)
	if fields.Supersedes != "" {
		input.Supersedes[identity] = fields.Supersedes
	}
	if fields.SupersededBy != "" {
		input.SupersededBy[identity] = fields.SupersededBy
	}
}
