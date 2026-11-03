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
	results := si.Search("hello", 20)
	if len(results) != 2 {
		t.Errorf("expected 2 results for 'hello', got %d", len(results))
	}

	// Multi-term AND search.
	results = si.Search("hello world", 20)
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'hello world', got %d", len(results))
	}
	if len(results) > 0 && results[0].UUID != "uuid-1" {
		t.Errorf("expected uuid-1, got %s", results[0].UUID)
	}

	// Search across pages.
	results = si.Search("graphql", 20)
	if len(results) != 2 {
		t.Errorf("expected 2 results for 'graphql', got %d", len(results))
	}

	// No results.
	results = si.Search("nonexistent", 20)
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

	results := si.Search("common", 5)
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
	results := si.Search("original", 20)
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
	results = si.Search("original", 20)
	if len(results) != 0 {
		t.Errorf("expected 0 results for 'original' after reindex, got %d", len(results))
	}

	// New content should be found.
	results = si.Search("updated", 20)
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

	results := si.Search("searchable", 20)
	if len(results) != 1 {
		t.Errorf("expected 1 result before removal, got %d", len(results))
	}

	si.RemovePage("test page")

	results = si.Search("searchable", 20)
	if len(results) != 0 {
		t.Errorf("expected 0 results after removal, got %d", len(results))
	}
}

// compoundIndex builds an index over one page whose blocks have the given
// UUID → content pairs (issue #149 tests).
func compoundIndex(blocks map[string]string) *SearchIndex {
	page := &cachedPage{
		entity:    types.PageEntity{Name: "KB", OriginalName: "KB"},
		lowerName: "kb",
		filePath:  "KB.md",
	}
	for uuid, content := range blocks {
		page.blocks = append(page.blocks, types.BlockEntity{UUID: uuid, Content: content})
	}
	si := NewSearchIndex()
	si.BuildFrom(map[string]*cachedPage{"kb": page})
	return si
}

func TestSearchIndex_CompoundIDForms(t *testing.T) {
	si := compoundIndex(map[string]string{
		"claim":  "## KB-REVIEW-ADJUD-01: Verdict gates use two layers",
		"other":  "## KB-GOV-CI-03: Pin actions by SHA",
		"snake":  "Set max_crap_load in config",
		"noise":  "Unrelated review notes",
		"single": "Use the a-b test pattern",
	})

	for _, query := range []string{"KB-REVIEW-ADJUD-01", "KB-REVIEW-ADJUD", "KB REVIEW ADJUD 01", "kb-review-adjud-01"} {
		results := si.Search(query, 20)
		if len(results) != 1 || results[0].UUID != "claim" {
			t.Errorf("Search(%q) = %v, want only the claim block", query, results)
		}
	}
	if results := si.Search("crap_load", 20); len(results) != 1 || results[0].UUID != "snake" {
		t.Errorf("Search(crap_load) = %v, want the snake_case block", results)
	}
	// A compound with no indexable component stays a whole-term lookup.
	if results := si.Search("a-b", 20); len(results) != 1 || results[0].UUID != "single" {
		t.Errorf("Search(a-b) = %v, want the a-b block", results)
	}
}

func TestSearchIndex_CompoundExactMatchesFirst(t *testing.T) {
	si := compoundIndex(map[string]string{
		"apart": "foo is near bar here",
		"exact": "the foo-bar widget",
	})
	results := si.Search("foo-bar", 20)
	if len(results) != 2 {
		t.Fatalf("Search(foo-bar) returned %d results, want 2 (each block once): %v", len(results), results)
	}
	if results[0].UUID != "exact" || results[1].UUID != "apart" {
		t.Errorf("order = %s, %s; want exact, apart", results[0].UUID, results[1].UUID)
	}

	// Exact matches that fill the limit skip the component pass.
	if results := si.Search("foo-bar", 1); len(results) != 1 || results[0].UUID != "exact" {
		t.Errorf("Search(foo-bar, 1) = %v, want only the exact block", results)
	}
}

func TestSearchIndex_CompoundComponentsRemovedWithPage(t *testing.T) {
	si := compoundIndex(map[string]string{"claim": "KB-REVIEW-ADJUD-01"})
	si.RemovePage("kb")
	if results := si.Search("adjud", 20); len(results) != 0 {
		t.Errorf("Search(adjud) after removal = %v, want none", results)
	}
}

func TestSplitCompound(t *testing.T) {
	for term, want := range map[string]string{
		"kb-review-adjud-01": "kb,review,adjud,01",
		"max_crap_load":      "max,crap,load",
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
