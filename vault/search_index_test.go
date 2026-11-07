package vault

import (
	"strings"
	"testing"

	"github.com/unbound-force/dewey/v3/types"
)

func TestSearchIndex_BasicSearch(t *testing.T) {
	si := NewSearchIndex()

	pages := map[string]*cachedPage{
		"test page": {
			entity:    types.PageEntity{Name: "Test Page", OriginalName: "Test Page"},
			lowerName: "test page",
			filePath:  "Test Page.md",
			blocks: []types.BlockEntity{
				{UUID: "uuid-1", Content: "Hello world this is a test"},
				{UUID: "uuid-2", Content: "Another block about [[GraphQL]]"},
				{UUID: "uuid-3", Content: "Something completely different"},
			},
		},
		"second page": {
			entity:    types.PageEntity{Name: "Second Page", OriginalName: "Second Page"},
			lowerName: "second page",
			filePath:  "Second Page.md",
			blocks: []types.BlockEntity{
				{UUID: "uuid-4", Content: "Hello from the second page"},
				{UUID: "uuid-5", Content: "GraphQL is great for APIs"},
			},
		},
	}

	si.BuildFrom(pages)

	// Single term search.
	results, _ := si.Search("hello", 20)
	if len(results) != 2 {
		t.Errorf("expected 2 results for 'hello', got %d", len(results))
	}

	// Multi-term AND search.
	results, _ = si.Search("hello world", 20)
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'hello world', got %d", len(results))
	}
	if len(results) > 0 && results[0].UUID != "uuid-1" {
		t.Errorf("expected uuid-1, got %s", results[0].UUID)
	}

	// Search across pages.
	results, _ = si.Search("graphql", 20)
	if len(results) != 2 {
		t.Errorf("expected 2 results for 'graphql', got %d", len(results))
	}

	// No results.
	results, _ = si.Search("nonexistent", 20)
	if len(results) != 0 {
		t.Errorf("expected 0 results for 'nonexistent', got %d", len(results))
	}
}

func TestSearchIndex_Limit(t *testing.T) {
	si := NewSearchIndex()

	var blocks []types.BlockEntity
	for i := 0; i < 50; i++ {
		blocks = append(blocks, types.BlockEntity{
			UUID:    "uuid-" + string(rune('a'+i%26)),
			Content: "common term in every block",
		})
	}

	pages := map[string]*cachedPage{
		"big page": {
			entity:    types.PageEntity{Name: "Big Page", OriginalName: "Big Page"},
			lowerName: "big page",
			filePath:  "Big Page.md",
			blocks:    blocks,
		},
	}

	si.BuildFrom(pages)

	results, _ := si.Search("common", 5)
	if len(results) != 5 {
		t.Errorf("expected 5 results with limit, got %d", len(results))
	}
}

func TestSearchIndex_ReindexPage(t *testing.T) {
	si := NewSearchIndex()

	pages := map[string]*cachedPage{
		"test page": {
			entity:    types.PageEntity{Name: "Test Page", OriginalName: "Test Page"},
			lowerName: "test page",
			filePath:  "Test Page.md",
			blocks: []types.BlockEntity{
				{UUID: "uuid-1", Content: "Original content here"},
			},
		},
	}

	si.BuildFrom(pages)

	// Verify original content is searchable.
	results, _ := si.Search("original", 20)
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'original', got %d", len(results))
	}

	// Update the page.
	updated := &cachedPage{
		entity:    types.PageEntity{Name: "Test Page", OriginalName: "Test Page"},
		lowerName: "test page",
		filePath:  "Test Page.md",
		blocks: []types.BlockEntity{
			{UUID: "uuid-1", Content: "Updated content instead"},
		},
	}
	si.ReindexPage(updated)

	// Old content should not be found.
	results, _ = si.Search("original", 20)
	if len(results) != 0 {
		t.Errorf("expected 0 results for 'original' after reindex, got %d", len(results))
	}

	// New content should be found.
	results, _ = si.Search("updated", 20)
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'updated' after reindex, got %d", len(results))
	}
}

func TestSearchIndex_RemovePage(t *testing.T) {
	si := NewSearchIndex()

	pages := map[string]*cachedPage{
		"test page": {
			entity:    types.PageEntity{Name: "Test Page", OriginalName: "Test Page"},
			lowerName: "test page",
			filePath:  "Test Page.md",
			blocks: []types.BlockEntity{
				{UUID: "uuid-1", Content: "Searchable content"},
			},
		},
	}

	si.BuildFrom(pages)

	results, _ := si.Search("searchable", 20)
	if len(results) != 1 {
		t.Errorf("expected 1 result before removal, got %d", len(results))
	}

	si.RemovePage("test page")

	results, _ = si.Search("searchable", 20)
	if len(results) != 0 {
		t.Errorf("expected 0 results after removal, got %d", len(results))
	}
}

// mkBlock returns a block entity with the given UUID and content, for
// building ordered fixture slices (issue #149 tests).
func mkBlock(uuid, content string) types.BlockEntity {
	return types.BlockEntity{UUID: uuid, Content: content}
}

// compoundIndex builds an index over one page with the given blocks in
// fixed order. Map iteration order is randomized in Go, so an ordered
// slice keeps posting-list order — and therefore same-pass result order —
// deterministic (issue #149 tests).
func compoundIndex(blocks ...types.BlockEntity) *SearchIndex {
	page := &cachedPage{
		entity:    types.PageEntity{Name: "KB", OriginalName: "KB"},
		lowerName: "kb",
		filePath:  "KB.md",
	}
	page.blocks = blocks
	si := NewSearchIndex()
	si.BuildFrom(map[string]*cachedPage{"kb": page})
	return si
}

func TestSearchIndex_CompoundIDForms(t *testing.T) {
	si := compoundIndex(
		mkBlock("claim", "## KB-REVIEW-ADJUD-01: Verdict gates use two layers"),
		mkBlock("other", "## KB-GOV-CI-03: Pin actions by SHA"),
		mkBlock("snake", "Set max_crap_load in config"),
		mkBlock("noise", "Unrelated review notes"),
		mkBlock("single", "Use the a-b test pattern"),
	)

	for _, query := range []string{"KB-REVIEW-ADJUD-01", "KB-REVIEW-ADJUD", "KB REVIEW ADJUD 01", "kb-review-adjud-01"} {
		results, _ := si.Search(query, 20)
		if len(results) != 1 || results[0].UUID != "claim" {
			t.Errorf("Search(%q) = %v, want only the claim block", query, results)
		}
	}
	if results, _ := si.Search("crap_load", 20); len(results) != 1 || results[0].UUID != "snake" {
		t.Errorf("Search(crap_load) = %v, want the snake_case block", results)
	}
	// A compound with no indexable component stays a whole-term lookup.
	if results, _ := si.Search("a-b", 20); len(results) != 1 || results[0].UUID != "single" {
		t.Errorf("Search(a-b) = %v, want the a-b block", results)
	}
	// A mixed compound keeps only its qualifying components: x-kb expands
	// to [kb] (the 1-char part is dropped), matching every block indexed
	// under the kb component.
	if results, _ := si.Search("x-kb", 20); len(results) != 2 || results[0].UUID != "claim" || results[1].UUID != "other" {
		t.Errorf("Search(x-kb) = %v, want the claim and other blocks via the kb component", results)
	}
}

func TestSearchIndex_CompoundExactMatchesFirst(t *testing.T) {
	si := compoundIndex(
		mkBlock("apart", "foo is near bar here"),
		mkBlock("exact", "the foo-bar widget"),
	)
	results, _ := si.Search("foo-bar", 20)
	if len(results) != 2 {
		t.Fatalf("Search(foo-bar) returned %d results, want 2 (each block once): %v", len(results), results)
	}
	if results[0].UUID != "exact" || results[1].UUID != "apart" {
		t.Errorf("order = %s, %s; want exact, apart", results[0].UUID, results[1].UUID)
	}

	// Exact matches that fill the limit skip the component pass.
	if results, _ := si.Search("foo-bar", 1); len(results) != 1 || results[0].UUID != "exact" {
		t.Errorf("Search(foo-bar, 1) = %v, want only the exact block", results)
	}
}

// The pass-two component fill respects the remaining slots after the exact
// pass: with a limit of 2, one exact match plus three component-only
// matches return exactly the exact match and one component match.
func TestSearchIndex_CompoundPassTwoRespectsLimit(t *testing.T) {
	si := compoundIndex(
		mkBlock("exact", "the foo-bar widget"),
		mkBlock("c1", "foo and bar one"),
		mkBlock("c2", "foo and bar two"),
		mkBlock("c3", "foo and bar three"),
	)
	results, _ := si.Search("foo-bar", 2)
	if len(results) != 2 {
		t.Fatalf("Search(foo-bar, 2) returned %d results, want 2: %v", len(results), results)
	}
	if results[0].UUID != "exact" || results[1].UUID != "c1" {
		t.Errorf("order = %s, %s; want exact, c1 (fills respect remaining slots in posting order)", results[0].UUID, results[1].UUID)
	}
}

// A query mixing a plain term with a compound term requires both: the
// compound's components and the plain term (AND semantics, design D2).
func TestSearchIndex_CompoundAndPlainTermAND(t *testing.T) {
	si := compoundIndex(
		mkBlock("claim", "## KB-REVIEW-ADJUD-01: Verdict gates use two layers"),
		mkBlock("decoy", "## KB-REVIEW-ADJUD-02: unrelated subject"),
	)
	// The claim block contains "verdict"; the decoy has the components
	// but not the word, so only the claim block matches.
	results, _ := si.Search("verdict KB-REVIEW-ADJUD", 20)
	if len(results) != 1 || results[0].UUID != "claim" {
		t.Errorf("Search(verdict KB-REVIEW-ADJUD) = %v, want only the claim block", results)
	}
}

func TestSearchIndex_CompoundComponentsRemovedWithPage(t *testing.T) {
	si := compoundIndex(mkBlock("claim", "KB-REVIEW-ADJUD-01"))
	si.RemovePage("kb")
	if results, _ := si.Search("adjud", 20); len(results) != 0 {
		t.Errorf("Search(adjud) after removal = %v, want none", results)
	}
}

func TestSplitCompound(t *testing.T) {
	for term, want := range map[string]string{
		"kb-review-adjud-01": "kb,review,adjud,01",
		"max_crap_load":      "max,crap,load",
		"x-kb":               "kb",
		"a-b":                "",
		"plain":              "",
	} {
		if got := strings.Join(splitCompound(term), ","); got != want {
			t.Errorf("splitCompound(%q) = %q, want %q", term, got, want)
		}
	}
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"Hello World", []string{"hello", "world"}},
		{"[[Page Link]]", []string{"page", "link"}},
		{"key:: value", []string{"key", "value"}},
		{"#tag content", []string{"tag", "content"}},
		{"a b", []string{}},                     // single chars filtered
		{"über cool", []string{"über", "cool"}}, // Unicode support
	}

	for _, tt := range tests {
		got := tokenize(tt.input)
		if len(got) != len(tt.expected) {
			t.Errorf("tokenize(%q) = %v (len %d), want %v (len %d)", tt.input, got, len(got), tt.expected, len(tt.expected))
			continue
		}
		for i, g := range got {
			if g != tt.expected[i] {
				t.Errorf("tokenize(%q)[%d] = %q, want %q", tt.input, i, g, tt.expected[i])
			}
		}
	}
}
