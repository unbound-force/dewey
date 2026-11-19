package tools

import (
	"context"
	"encoding/json"
	"slices"
	"sort"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/unbound-force/dewey/v3/backend"
	"github.com/unbound-force/dewey/v3/store"
	"github.com/unbound-force/dewey/v3/types"
)

// newScopeStore returns an in-memory store with three pages across two
// sources: "alpha-notes" (disk-alpha, tagged research in properties),
// "alpha-misc" (disk-alpha, untagged) and "beta-notes" (disk-beta,
// tagged research). Each page has one embedded block.
func newScopeStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	pages := []*store.Page{
		{Name: "alpha-notes", OriginalName: "Alpha-Notes", SourceID: "disk-alpha", SourceDocID: "notes.md", Properties: `{"tags":["research"]}`},
		{Name: "alpha-misc", OriginalName: "alpha-misc", SourceID: "disk-alpha", SourceDocID: "misc.md"},
		{Name: "beta-notes", OriginalName: "beta-notes", SourceID: "disk-beta", SourceDocID: "notes.md", Properties: `{"tags":["research"]}`},
	}
	for _, p := range pages {
		if err := s.InsertPage(p); err != nil {
			t.Fatalf("InsertPage(%s): %v", p.Name, err)
		}
	}
	blocks := []struct {
		uuid, page, content string
		vec                 []float32
	}{
		{"blk-alpha", "alpha-notes", "alpha needle", []float32{1, 0, 0}},
		{"blk-misc", "alpha-misc", "misc needle #research", []float32{0.95, 0.05, 0}},
		{"blk-beta", "beta-notes", "beta needle", []float32{0.9, 0.1, 0}},
	}
	for _, b := range blocks {
		if err := s.InsertBlock(&store.Block{UUID: b.uuid, PageName: b.page, Content: b.content}); err != nil {
			t.Fatalf("InsertBlock(%s): %v", b.uuid, err)
		}
		if err := s.InsertEmbedding(b.uuid, "test-model", b.vec, b.content); err != nil {
			t.Fatalf("InsertEmbedding(%s): %v", b.uuid, err)
		}
	}
	return s
}

// scopedSemanticPages runs a semantic tool result through JSON and returns the
// sorted page names.
func scopedSemanticPages(t *testing.T, result *mcp.CallToolResult) []string {
	t.Helper()
	if result.IsError {
		t.Fatalf("unexpected error result: %s", resultText(result))
	}
	var out []types.SemanticSearchResult
	if err := json.Unmarshal([]byte(resultText(result)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	pages := make([]string, 0, len(out))
	for _, r := range out {
		pages = append(pages, r.Page)
	}
	sort.Strings(pages)
	return pages
}

// resultPages returns the sorted "page" field of each entry in a
// search / find_by_tag JSON result.
func resultPages(t *testing.T, result *mcp.CallToolResult) []string {
	t.Helper()
	if result.IsError {
		t.Fatalf("unexpected error result: %s", resultText(result))
	}
	var parsed struct {
		Results []map[string]any `json:"results"`
	}
	text := resultText(result)
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		// "No results found" is plain text.
		return nil
	}
	pages := make([]string, 0, len(parsed.Results))
	for _, r := range parsed.Results {
		p, _ := r["page"].(string)
		pages = append(pages, p)
	}
	sort.Strings(pages)
	return pages
}

func TestSemanticSearch_SourceScope(t *testing.T) {
	sem := NewSemantic(newMockEmbedder(true), newScopeStore(t))
	tests := []struct {
		name  string
		input types.SemanticSearchInput
		want  []string
	}{
		{"unscoped", types.SemanticSearchInput{Query: "q"}, []string{"alpha-misc", "alpha-notes", "beta-notes"}},
		{"source_id", types.SemanticSearchInput{Query: "q", SourceID: "disk-alpha"}, []string{"alpha-misc", "alpha-notes"}},
		{"has_tag", types.SemanticSearchInput{Query: "q", HasTag: "research"}, []string{"alpha-misc", "alpha-notes", "beta-notes"}},
		{"source_id and has_tag", types.SemanticSearchInput{Query: "q", SourceID: "disk-beta", HasTag: "research"}, []string{"beta-notes"}},
		{"unknown source", types.SemanticSearchInput{Query: "q", SourceID: "disk-none"}, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, _, err := sem.SemanticSearch(context.Background(), nil, tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if got := scopedSemanticPages(t, res); !slices.Equal(got, tt.want) {
				t.Errorf("pages = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSemanticSearch_ScopeMatchesFiltered pins parity: a scoped
// semantic_search returns exactly what semantic_search_filtered returns
// for the same source_id / has_tag.
func TestSemanticSearch_ScopeMatchesFiltered(t *testing.T) {
	sem := NewSemantic(newMockEmbedder(true), newScopeStore(t))
	for _, f := range []struct{ src, tag string }{{"disk-alpha", ""}, {"", "research"}, {"disk-alpha", "research"}} {
		plain, _, _ := sem.SemanticSearch(context.Background(), nil, types.SemanticSearchInput{Query: "q", SourceID: f.src, HasTag: f.tag})
		filtered, _, _ := sem.SemanticSearchFiltered(context.Background(), nil, types.SemanticSearchFilteredInput{Query: "q", SourceID: f.src, HasTag: f.tag})
		if a, b := scopedSemanticPages(t, plain), scopedSemanticPages(t, filtered); !slices.Equal(a, b) {
			t.Errorf("source_id=%q has_tag=%q: semantic_search %v != semantic_search_filtered %v", f.src, f.tag, a, b)
		}
	}
}

func TestSimilar_SourceScope(t *testing.T) {
	sem := NewSemantic(newMockEmbedder(true), newScopeStore(t))

	res, _, err := sem.Similar(context.Background(), nil, types.SimilarInput{UUID: "blk-alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := scopedSemanticPages(t, res), []string{"alpha-misc", "beta-notes"}; !slices.Equal(got, want) {
		t.Errorf("unscoped pages = %v, want %v", got, want)
	}

	// The query block is in disk-alpha; results are scoped to disk-beta.
	res, _, err = sem.Similar(context.Background(), nil, types.SimilarInput{UUID: "blk-alpha", SourceID: "disk-beta"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := scopedSemanticPages(t, res), []string{"beta-notes"}; !slices.Equal(got, want) {
		t.Errorf("source-scoped pages = %v, want %v", got, want)
	}

	res, _, err = sem.Similar(context.Background(), nil, types.SimilarInput{Page: "beta-notes", SourceID: "disk-alpha", HasTag: "research"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := scopedSemanticPages(t, res), []string{"alpha-misc", "alpha-notes"}; !slices.Equal(got, want) {
		t.Errorf("source+tag pages = %v, want %v", got, want)
	}
}

func TestSearch_Indexed_SourceScope(t *testing.T) {
	st := newScopeStore(t)
	fts := &mockFullTextSearcher{results: []backend.SearchHit{
		{PageName: "Alpha-Notes", UUID: "blk-alpha", Content: "alpha needle"},
		{PageName: "alpha-misc", UUID: "blk-misc", Content: "misc needle #research"},
		{PageName: "beta-notes", UUID: "blk-beta", Content: "beta needle"},
		{PageName: "vault-only", UUID: "blk-x", Content: "needle not in store"},
	}}
	s := NewSearch(&mockBackendWithFullTextSearch{mockBackend: newMockBackend(), mockFullTextSearcher: fts}).WithStore(st)

	tests := []struct {
		name  string
		input types.SearchInput
		want  []string
	}{
		{"unscoped keeps every hit", types.SearchInput{Query: "needle"}, []string{"Alpha-Notes", "alpha-misc", "beta-notes", "vault-only"}},
		{"source_id, case-insensitive page lookup", types.SearchInput{Query: "needle", SourceID: "disk-alpha"}, []string{"Alpha-Notes", "alpha-misc"}},
		{"has_tag via properties or #tag content", types.SearchInput{Query: "needle", HasTag: "RESEARCH"}, []string{"Alpha-Notes", "alpha-misc", "beta-notes"}},
		{"source_id and has_tag", types.SearchInput{Query: "needle", SourceID: "disk-beta", HasTag: "research"}, []string{"beta-notes"}},
		{"limit applies after scope", types.SearchInput{Query: "needle", SourceID: "disk-alpha", Limit: 1}, []string{"Alpha-Notes"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, _, err := s.Search(context.Background(), nil, tt.input)
			if err != nil {
				t.Fatal(err)
			}
			got := resultPages(t, res)
			sort.Strings(tt.want)
			if !slices.Equal(got, tt.want) {
				t.Errorf("pages = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSearch_Indexed_ScopeOverFetches verifies that a scoped search asks
// the backend for more than limit hits, so in-scope hits ranked below
// out-of-scope ones still fill the limit.
func TestSearch_Indexed_ScopeOverFetches(t *testing.T) {
	st := newScopeStore(t)
	hits := make([]backend.SearchHit, 0, 30)
	for range 25 {
		hits = append(hits, backend.SearchHit{PageName: "beta-notes", UUID: "blk-beta", Content: "beta needle"})
	}
	hits = append(hits, backend.SearchHit{PageName: "alpha-notes", UUID: "blk-alpha", Content: "alpha needle"})
	fts := &mockFullTextSearcher{results: hits}
	s := NewSearch(&mockBackendWithFullTextSearch{mockBackend: newMockBackend(), mockFullTextSearcher: fts}).WithStore(st)

	res, _, err := s.Search(context.Background(), nil, types.SearchInput{Query: "needle", SourceID: "disk-alpha", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if got := resultPages(t, res); !slices.Equal(got, []string{"alpha-notes"}) {
		t.Errorf("pages = %v, want [alpha-notes]", got)
	}
}

func TestSearch_BruteForce_SourceScope(t *testing.T) {
	st := newScopeStore(t)
	mb := newMockBackend()
	mb.addPage(types.PageEntity{Name: "alpha-notes", OriginalName: "Alpha-Notes"},
		types.BlockEntity{UUID: "b1", Content: "foo alpha"})
	mb.addPage(types.PageEntity{Name: "beta-notes", OriginalName: "beta-notes"},
		types.BlockEntity{UUID: "b2", Content: "foo beta"})
	s := NewSearch(mb).WithStore(st)

	res, _, err := s.Search(context.Background(), nil, types.SearchInput{Query: "foo", SourceID: "disk-beta", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got := resultPages(t, res); !slices.Equal(got, []string{"beta-notes"}) {
		t.Errorf("pages = %v, want [beta-notes] (out-of-scope matches must not consume the limit)", got)
	}
}

func TestSearch_ScopeWithoutStore(t *testing.T) {
	s := NewSearch(newMockBackend())
	res, _, err := s.Search(context.Background(), nil, types.SearchInput{Query: "foo", SourceID: "disk-alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || resultText(res) != scopeUnavailableMessage {
		t.Errorf("want error %q, got %q (IsError=%v)", scopeUnavailableMessage, resultText(res), res.IsError)
	}
	res, _, _ = s.FindByTag(context.Background(), nil, types.FindByTagInput{Tag: "x", SourceID: "disk-alpha"})
	if !res.IsError {
		t.Error("find_by_tag: want error result without a store")
	}
}

func TestFindByTag_TagSearcher_SourceScope(t *testing.T) {
	st := newScopeStore(t)
	ts := &mockTagSearcher{results: []backend.TagResult{
		{Page: "alpha-notes", Blocks: []types.BlockEntity{{UUID: "a1", Content: "#research a"}}},
		{Page: "beta-notes", Blocks: []types.BlockEntity{{UUID: "b1", Content: "#research b"}, {UUID: "b2", Content: "#research c"}}},
	}}
	s := NewSearch(&mockBackendWithTagSearch{mockBackend: newMockBackend(), mockTagSearcher: ts}).WithStore(st)

	res, _, err := s.FindByTag(context.Background(), nil, types.FindByTagInput{Tag: "research", SourceID: "disk-beta"})
	if err != nil {
		t.Fatal(err)
	}
	if got := resultPages(t, res); !slices.Equal(got, []string{"beta-notes", "beta-notes"}) {
		t.Errorf("pages = %v, want both beta-notes blocks", got)
	}

	res, _, _ = s.FindByTag(context.Background(), nil, types.FindByTagInput{Tag: "research"})
	if got := resultPages(t, res); len(got) != 3 {
		t.Errorf("unscoped pages = %v, want 3 blocks", got)
	}
}

func TestFindByTag_DataScript_SourceScope(t *testing.T) {
	st := newScopeStore(t)
	mb := newMockBackend()
	query := `[:find (pull ?b [:block/uuid :block/content {:block/page [:block/name :block/original-name]}])
		:where
		[?b :block/refs ?ref]
		[?ref :block/name "research"]]`
	mb.queryResults[query] = json.RawMessage(`[[{"uuid":"a1","content":"#research a","page":{"name":"alpha-notes"}}],[{"uuid":"b1","content":"#research b","page":{"name":"beta-notes"}}]]`)
	s := NewSearch(mb).WithStore(st)

	res, _, err := s.FindByTag(context.Background(), nil, types.FindByTagInput{Tag: "research", SourceID: "disk-alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if got := resultPages(t, res); !slices.Equal(got, []string{"alpha-notes"}) {
		t.Errorf("pages = %v, want [alpha-notes]", got)
	}
}

func TestScopedFetchLimit(t *testing.T) {
	if got := scopedFetchLimit(20, pageScope{}); got != 20 {
		t.Errorf("unscoped = %d, want 20", got)
	}
	if got := scopedFetchLimit(5, pageScope{SourceID: "s"}); got != 200 {
		t.Errorf("scoped small = %d, want 200", got)
	}
	if got := scopedFetchLimit(50, pageScope{HasTag: "t"}); got != 500 {
		t.Errorf("scoped large = %d, want 500", got)
	}
}
