package tools

import (
	"fmt"
	"strings"
	"testing"
)

// Identity fixtures shared by the citation-coverage tests. The Alice/Bob
// pair exercises the timestamped identity form; covLegacy exercises the
// legacy pre-#146 {tag}-{sequence} form the store still holds.
const (
	covAlice  = "auth-20261001T000000-alice"
	covBob    = "auth-20261101T000000-bob"
	covLegacy = "auth-1"
)

// covArticle builds a compile-shaped article: frontmatter with a sources
// list, a title, the given body, and an auto-generated History table with
// one row per source — the shape buildCompiledArticle writes. Cases that
// need a different section layout build their article inline instead.
func covArticle(body string, sources ...string) string {
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString("tier: draft\n")
	sb.WriteString("sources:\n")
	for _, s := range sources {
		fmt.Fprintf(&sb, "  - %s\n", s)
	}
	sb.WriteString("topic: auth\n")
	sb.WriteString("---\n\n")
	sb.WriteString("# Auth\n\n")
	sb.WriteString(body)
	sb.WriteString("\n\n")
	sb.WriteString("## History\n\n")
	sb.WriteString("| Learning | Date | Category | Summary |\n")
	sb.WriteString("|----------|------|----------|--------|\n")
	for _, s := range sources {
		fmt.Fprintf(&sb, "| %s | 2026-11-01 | context | earlier finding |\n", strings.TrimPrefix(s, "learning/"))
	}
	return sb.String()
}

// covAssertSlice asserts that got equals want elementwise, in order, with
// a failure message naming the result field.
func covAssertSlice(t *testing.T, label string, want, got []string) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("%s: want %v, got %v", label, want, got)
	}
	for i := range want {
		if want[i] != got[i] {
			t.Errorf("%s[%d]: want %q, got %q", label, i, want[i], got[i])
		}
	}
}

// TestCheckCitationCoverage covers task 3.2 (ES-FR-010): forward coverage
// (every citation resolves to a declared source), backward coverage (every
// declared source is cited or, when superseded, accounted for), the
// section exclusions, and the non-citation bracket forms.
func TestCheckCitationCoverage(t *testing.T) {
	tests := []struct {
		name           string
		article        string
		sources        []CitationSource
		wantUnresolved []string
		wantUncited    []string
		wantCited      []string
		wantAccounted  []string
		wantComplete   bool
	}{
		{
			name:         "clean article",
			article:      covArticle("Uses tokens ["+covAlice+"] and rotation ["+covBob+"].", covAlice, covBob),
			sources:      []CitationSource{{Identity: covAlice}, {Identity: covBob}},
			wantCited:    []string{covAlice, covBob},
			wantComplete: true,
		},
		{
			name:           "truncated citation reported unresolved",
			article:        covArticle("Fact [auth-20261101T000000-al].", covAlice),
			sources:        []CitationSource{{Identity: covAlice}},
			wantUnresolved: []string{"auth-20261101T000000-al"},
			wantUncited:    []string{covAlice},
		},
		{
			name:           "unknown identity reported unresolved",
			article:        covArticle("Fact ["+covBob+"].", covAlice),
			sources:        []CitationSource{{Identity: covAlice}},
			wantUnresolved: []string{covBob},
			wantUncited:    []string{covAlice},
		},
		{
			name:        "uncited source reported",
			article:     covArticle("Fact ["+covAlice+"].", covAlice, covBob),
			sources:     []CitationSource{{Identity: covAlice}, {Identity: covBob}},
			wantUncited: []string{covBob},
			wantCited:   []string{covAlice},
		},
		{
			name:           "multi-identity group resolves elementwise",
			article:        covArticle("Shared fact ["+covAlice+", auth-20260101T000000-zed].", covAlice),
			sources:        []CitationSource{{Identity: covAlice}},
			wantUnresolved: []string{"auth-20260101T000000-zed"},
			wantCited:      []string{covAlice},
		},
		{
			name: "frontmatter History and proposals excluded from forward counting",
			article: "---\n" +
				"tier: draft\n" +
				"note: [auth-20260101T000000-zed]\n" +
				"sources:\n" +
				"  - " + covAlice + "\n" +
				"---\n\n" +
				"# Auth\n\n" +
				"Fact [" + covAlice + "].\n\n" +
				"## Supersession Proposals\n\n" +
				"- successor: " + covBob + "\n" +
				"  target: " + covAlice + "\n" +
				"  rationale: clearer replacement\n\n" +
				"## History\n\n" +
				"| Learning | Date | Category | Summary |\n" +
				"|----------|------|----------|--------|\n" +
				"| " + covAlice + " | 2026-11-01 | context | see [auth-20260201T000000-yuki] |\n",
			sources:      []CitationSource{{Identity: covAlice}},
			wantCited:    []string{covAlice},
			wantComplete: true,
		},
		{
			name: "early History heading does not fence the body",
			article: "---\ntier: draft\nsources:\n  - " + covAlice + "\n  - " + covBob + "\n---\n\n" +
				"# Auth\n\n" +
				"First fact [" + covAlice + "].\n\n" +
				"## History\n\nMid-document heading.\n\n" +
				"Second fact [" + covBob + "].\n\n" +
				"## History\n\n" +
				"| Learning | Date | Category | Summary |\n" +
				"|----------|------|----------|--------|\n" +
				"| " + covAlice + " | 2026-11-01 | context | x |\n" +
				"| " + covBob + " | 2026-11-01 | context | y |\n",
			sources:      []CitationSource{{Identity: covAlice}, {Identity: covBob}},
			wantCited:    []string{covAlice, covBob},
			wantComplete: true,
		},
		{
			name:         "learning-prefixed declared source normalized",
			article:      covArticle("Fact ["+covAlice+"].", "learning/"+covAlice),
			sources:      []CitationSource{{Identity: "learning/" + covAlice}},
			wantCited:    []string{covAlice},
			wantComplete: true,
		},
		{
			name:          "superseded source accounted for by History row",
			article:       covArticle("Current fact ["+covAlice+"].", covAlice, covBob),
			sources:       []CitationSource{{Identity: covAlice}, {Identity: covBob, Superseded: true}},
			wantCited:     []string{covAlice},
			wantAccounted: []string{covBob},
			wantComplete:  true,
		},
		{
			name: "superseded source accounted for by in-body mention",
			article: "---\ntier: draft\nsources:\n  - " + covAlice + "\n  - " + covBob + "\n---\n\n" +
				"# Auth\n\n" +
				"Current fact [" + covAlice + "]. The older position (" + covBob + ") is retired.\n",
			sources:       []CitationSource{{Identity: covAlice}, {Identity: covBob, Superseded: true}},
			wantCited:     []string{covAlice},
			wantAccounted: []string{covBob},
			wantComplete:  true,
		},
		{
			name:         "legacy identity citation counts in both directions",
			article:      covArticle("Old finding ["+covLegacy+"].", covLegacy),
			sources:      []CitationSource{{Identity: covLegacy}},
			wantCited:    []string{covLegacy},
			wantComplete: true,
		},
		{
			name:        "current source with only a History row stays uncited",
			article:     covArticle("Unrelated prose with no citations.", covAlice),
			sources:     []CitationSource{{Identity: covAlice}},
			wantUncited: []string{covAlice},
		},
		{
			name: "non-citation bracket forms ignored",
			article: covArticle("See [the docs](https://example.test), [ref-label][target], [1], [x], "+
				"[[wiki-link]], [], and [see the notes]. Fact ["+covAlice+"].", covAlice),
			sources:      []CitationSource{{Identity: covAlice}},
			wantCited:    []string{covAlice},
			wantComplete: true,
		},
		{
			name:        "mixed group dropped entirely by whole-group gate",
			article:     covArticle("Fact ["+covAlice+", not-an-identity].", covAlice),
			sources:     []CitationSource{{Identity: covAlice}},
			wantUncited: []string{covAlice},
		},
		{
			name: "article without frontmatter",
			article: "# Auth\n\nFact [" + covAlice + "].\n\n## History\n\n" +
				"| Learning | Date |\n|----------|------|\n| " + covAlice + " | 2026-11-01 |\n",
			sources:      []CitationSource{{Identity: covAlice}},
			wantCited:    []string{covAlice},
			wantComplete: true,
		},
		{
			name: "superseded source with no accounting still uncited",
			article: "---\ntier: draft\nsources:\n  - " + covAlice + "\n  - " + covBob + "\n---\n\n" +
				"# Auth\n\nFact [" + covAlice + "].\n\n## History\n\n" +
				"| Learning | Date |\n|----------|------|\n| " + covAlice + " | 2026-11-01 |\n",
			sources:     []CitationSource{{Identity: covAlice}, {Identity: covBob, Superseded: true}},
			wantCited:   []string{covAlice},
			wantUncited: []string{covBob},
		},
		{
			name:           "citation with no declared sources is unresolved",
			article:        "# Auth\n\nFact [" + covAlice + "].",
			wantUnresolved: []string{covAlice},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckCitationCoverage(CoverageOptions{Article: tt.article, Sources: tt.sources})
			covAssertSlice(t, "unresolved citations", tt.wantUnresolved, got.UnresolvedCitations)
			covAssertSlice(t, "uncited sources", tt.wantUncited, got.UncitedSources)
			covAssertSlice(t, "cited sources", tt.wantCited, got.CitedSources)
			covAssertSlice(t, "accounted superseded sources", tt.wantAccounted, got.AccountedSupersededSources)
			if got.Complete() != tt.wantComplete {
				t.Errorf("Complete(): want %t, got %t (unresolved=%v uncited=%v)",
					tt.wantComplete, got.Complete(), got.UnresolvedCitations, got.UncitedSources)
			}
		})
	}
}

// TestCheckCitationCoverage_Counts pins the result counters: distinct
// citations extracted, citations resolved, and distinct declared sources
// after normalization.
func TestCheckCitationCoverage_Counts(t *testing.T) {
	got := CheckCitationCoverage(CoverageOptions{
		Article: covArticle("Shared fact ["+covAlice+", "+covBob+"] and again ["+covAlice+"].", covAlice, "learning/"+covBob),
		Sources: []CitationSource{{Identity: covAlice}, {Identity: "learning/" + covBob}, {Identity: covAlice}},
	})
	if got.CitationCount != 2 {
		t.Errorf("CitationCount: want 2 (distinct), got %d", got.CitationCount)
	}
	if got.ResolvedCitations != 2 {
		t.Errorf("ResolvedCitations: want 2, got %d", got.ResolvedCitations)
	}
	if got.SourceCount != 2 {
		t.Errorf("SourceCount: want 2 (deduplicated), got %d", got.SourceCount)
	}

	dropped := CheckCitationCoverage(CoverageOptions{
		Article: covArticle("Fact ["+covAlice+", not-an-identity].", covAlice),
		Sources: []CitationSource{{Identity: covAlice}},
	})
	if dropped.CitationCount != 0 {
		t.Errorf("CitationCount after whole-group drop: want 0, got %d", dropped.CitationCount)
	}
	if dropped.ResolvedCitations != 0 {
		t.Errorf("ResolvedCitations after whole-group drop: want 0, got %d", dropped.ResolvedCitations)
	}
}

// TestSplitArticle covers the shape-anchored section extraction: the body
// ends at the earlier of the last "## History" heading and the first "##
// Supersession Proposals" heading, frontmatter is stripped, and the
// History section runs from the last History heading to the next heading.
func TestSplitArticle(t *testing.T) {
	t.Run("frontmatter stripped and body ends at last History heading", func(t *testing.T) {
		article := "---\ntier: draft\n---\n\n# Title\n\nBody text.\n\n## History\n\n| row |\n"
		body, history := splitArticle(article)
		// The body keeps the blank line after the closing delimiter.
		if body != "\n# Title\n\nBody text.\n\n" {
			t.Errorf("body: want %q, got %q", "\n# Title\n\nBody text.\n\n", body)
		}
		if !strings.Contains(history, "| row |") {
			t.Errorf("history: want table row, got %q", history)
		}
		if strings.Contains(body, "tier: draft") {
			t.Errorf("body: frontmatter leaked into body: %q", body)
		}
	})
	t.Run("early History heading does not fence the body", func(t *testing.T) {
		article := "# Title\n\nFirst.\n\n## History\n\nMid.\n\nSecond.\n\n## History\n\n| row |\n"
		body, _ := splitArticle(article)
		if !strings.Contains(body, "Mid.") || !strings.Contains(body, "Second.") {
			t.Errorf("body: content between History headings must stay in scope, got %q", body)
		}
		if strings.Contains(body, "| row |") {
			t.Errorf("body: trailing History table must be excluded, got %q", body)
		}
	})
	t.Run("proposals section ends the body", func(t *testing.T) {
		article := "# Title\n\nBody.\n\n## Supersession Proposals\n\n- successor: x\n\n## History\n\n| row |\n"
		body, history := splitArticle(article)
		if strings.Contains(body, "successor") {
			t.Errorf("body: proposals section must be excluded, got %q", body)
		}
		if !strings.Contains(history, "| row |") {
			t.Errorf("history: want table row, got %q", history)
		}
	})
	t.Run("no frontmatter and no sections", func(t *testing.T) {
		body, history := splitArticle("# Title\n\nBody only.\n")
		if body != "# Title\n\nBody only.\n" {
			t.Errorf("body: want unchanged article, got %q", body)
		}
		if history != "" {
			t.Errorf("history: want empty, got %q", history)
		}
	})
	t.Run("unclosed frontmatter treated as body", func(t *testing.T) {
		body, _ := splitArticle("---\nnot closed\n")
		if body != "---\nnot closed\n" {
			t.Errorf("body: unclosed delimiter must not strip, got %q", body)
		}
	})
}

// TestScanBrackets pins the bracket scanner's depth semantics: wikilink
// inner text never surfaces as outside text, and a depth-zero group's
// content keeps nested brackets so the identity grammar rejects it.
func TestScanBrackets(t *testing.T) {
	tests := []struct {
		name        string
		in          string
		wantOutside string
		wantGroups  []string
	}{
		{"plain prose", "a b c", "a b c", nil},
		{"single group", "a [b] c", "a  c", []string{"b"}},
		{"adjacent groups", "[x][y]", "", []string{"x", "y"}},
		{"wikilink", "see [[wiki-link]] here", "see  here", []string{"[wiki-link]"}},
		{"stray closing bracket", "x ] y", "x ] y", nil},
		{"unclosed group", "open [unclosed text", "open ", nil},
		{"empty group", "a [] b", "a  b", []string{""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outside, groups := scanBrackets(tt.in)
			if outside != tt.wantOutside {
				t.Errorf("outside: want %q, got %q", tt.wantOutside, outside)
			}
			covAssertSlice(t, "groups", tt.wantGroups, groups)
		})
	}
}

// TestExtractCitations covers the citation grammar directly (design D3):
// whole-group matching against the identity grammar, both identity
// formats, first-occurrence order, and deduplication.
func TestExtractCitations(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []string
	}{
		{"timestamped identity", "Fact [" + covAlice + "].", []string{covAlice}},
		{"legacy identity", "Old [" + covLegacy + "].", []string{covLegacy}},
		{"multi-identity group", "Fact [" + covAlice + ", " + covBob + "].", []string{covAlice, covBob}},
		{"deduplicated in first-occurrence order",
			"[" + covBob + "] then [" + covAlice + "] then [" + covBob + "]",
			[]string{covBob, covAlice}},
		{"markdown link text ignored", "See [the docs](https://example.test).", nil},
		{"reference label ignored", "See [ref-label][target].", nil},
		{"numeric footnote ignored", "See [1].", nil},
		{"checkbox ignored", "- [x] done", nil},
		{"wikilink ignored", "See [[wiki-link]].", nil},
		{"empty brackets ignored", "[]", nil},
		{"prose in brackets ignored", "[see the notes]", nil},
		{"mixed group dropped", "[" + covAlice + ", not-an-identity]", nil},
		{"whitespace around elements", "[ " + covAlice + " , " + covBob + " ]", []string{covAlice, covBob}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractCitations(tt.body)
			covAssertSlice(t, "citations", tt.want, got)
		})
	}
}
