package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/unbound-force/dewey/v3/store"
	"github.com/unbound-force/dewey/v3/types"
)

// newValidityStore creates a store with a superseded learning whose block
// is closest to the query vector [1,0,0] and its current replacement.
func newValidityStore(t *testing.T) *store.Store {
	t.Helper()
	s := newTestStore(t)
	for _, p := range []struct {
		name, props string
		vec         []float32
	}{
		{"learning/gate-old", `{"tag":"gate","validity":"superseded","as_of":"kb@aaa"}`, []float32{1, 0, 0}},
		{"learning/gate-new", `{"tag":"gate","validity":"current","as_of":"kb@bbb"}`, []float32{0.9, 0.3, 0}},
	} {
		if err := s.InsertPage(&store.Page{Name: p.name, OriginalName: p.name, SourceID: store.SourceLearning, SourceDocID: p.name, Properties: p.props, Tier: "draft"}); err != nil {
			t.Fatalf("InsertPage: %v", err)
		}
		uuid := p.name + "-b"
		if err := s.InsertBlock(&store.Block{UUID: uuid, PageName: p.name, Content: "gate layers"}); err != nil {
			t.Fatalf("InsertBlock: %v", err)
		}
		if err := s.InsertEmbedding(uuid, "test-model", p.vec, "gate layers"); err != nil {
			t.Fatalf("InsertEmbedding: %v", err)
		}
	}
	return s
}

// semanticPages decodes a semantic search result into results.
func semanticPages(t *testing.T, text string) []types.SemanticSearchResult {
	t.Helper()
	var out []types.SemanticSearchResult
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal %q: %v", text, err)
	}
	return out
}

func TestSemanticSearch_ValidityRanking(t *testing.T) {
	s := newValidityStore(t)
	e := newMockEmbedder(true)
	e.vectors["gate"] = []float32{1, 0, 0}
	sem := NewSemantic(e, s)

	res, _, err := sem.SemanticSearch(context.Background(), nil, types.SemanticSearchInput{Query: "gate"})
	if err != nil || res.IsError {
		t.Fatalf("SemanticSearch = %v, %v", resultText(res), err)
	}
	ranked := semanticPages(t, resultText(res))
	if len(ranked) != 2 || ranked[0].Page != "learning/gate-new" {
		t.Fatalf("default order = %+v, want gate-new first", ranked)
	}
	if ranked[1].Similarity <= ranked[0].Similarity {
		t.Errorf("reported similarity must stay raw: old %.3f, new %.3f", ranked[1].Similarity, ranked[0].Similarity)
	}
	if ranked[0].Validity != "current" || ranked[0].AsOf != "kb@bbb" || ranked[1].Validity != "superseded" {
		t.Errorf("validity enrichment = %+v", ranked)
	}

	res, _, err = sem.SemanticSearch(context.Background(), nil, types.SemanticSearchInput{Query: "gate", IgnoreValidity: true})
	if err != nil || res.IsError {
		t.Fatalf("SemanticSearch raw = %v, %v", resultText(res), err)
	}
	if raw := semanticPages(t, resultText(res)); raw[0].Page != "learning/gate-old" {
		t.Errorf("ignore_validity order = %+v, want gate-old first", raw)
	}

	res, _, err = sem.SemanticSearch(context.Background(), nil, types.SemanticSearchInput{Query: "gate", Limit: 1})
	if err != nil || res.IsError {
		t.Fatalf("SemanticSearch limit = %v, %v", resultText(res), err)
	}
	if top := semanticPages(t, resultText(res)); len(top) != 1 || top[0].Page != "learning/gate-new" {
		t.Errorf("limit 1 = %+v, want only gate-new (over-fetch lets it displace gate-old)", top)
	}
}

func TestSemanticSearchFiltered_ValidityFilter(t *testing.T) {
	s := newValidityStore(t)
	e := newMockEmbedder(true)
	e.vectors["gate"] = []float32{1, 0, 0}
	sem := NewSemantic(e, s)

	res, _, err := sem.SemanticSearchFiltered(context.Background(), nil, types.SemanticSearchFilteredInput{Query: "gate", Validity: "current"})
	if err != nil || res.IsError {
		t.Fatalf("SemanticSearchFiltered = %v, %v", resultText(res), err)
	}
	if got := semanticPages(t, resultText(res)); len(got) != 1 || got[0].Page != "learning/gate-new" {
		t.Errorf("validity=current = %+v, want only gate-new", got)
	}

	res, _, err = sem.SemanticSearchFiltered(context.Background(), nil, types.SemanticSearchFilteredInput{Query: "gate", IgnoreValidity: true})
	if err != nil || res.IsError {
		t.Fatalf("SemanticSearchFiltered raw = %v, %v", resultText(res), err)
	}
	if got := semanticPages(t, resultText(res)); len(got) != 2 || got[0].Page != "learning/gate-old" {
		t.Errorf("ignore_validity order = %+v, want gate-old first", got)
	}
}

func TestValidityHelpers(t *testing.T) {
	for v, want := range map[string]float64{"superseded": 0.6, "stale-unverified": 0.85, "current": 1, "historical": 1, "": 1} {
		if got := validityWeight(v); got != want {
			t.Errorf("validityWeight(%q) = %v, want %v", v, got, want)
		}
	}

	results := []types.SemanticSearchResult{
		{Page: "a", Similarity: 0.5},
		{Page: "b", Similarity: 0.5},
		{Page: "c", Similarity: 0.9, Validity: "superseded"},
	}
	rankByValidity(results)
	if results[0].Page != "c" || results[1].Page != "a" || results[2].Page != "b" {
		t.Errorf("rankByValidity = %s %s %s, want c a b (0.54 beats 0.5; ties stable)", results[0].Page, results[1].Page, results[2].Page)
	}

	if got := filterByValidity(results, ""); len(got) != 3 {
		t.Errorf("empty filter kept %d, want 3", len(got))
	}
	if got := filterByValidity(results, "superseded"); len(got) != 1 || got[0].Page != "c" {
		t.Errorf("superseded filter = %+v", got)
	}

	for _, tc := range []struct {
		limit     int
		overFetch bool
		want      int
	}{{10, false, 10}, {10, true, 30}, {20, true, 60}} {
		if got := semanticFetchLimit(tc.limit, tc.overFetch); got != tc.want {
			t.Errorf("semanticFetchLimit(%d, %v) = %d, want %d", tc.limit, tc.overFetch, got, tc.want)
		}
	}

	if got := finishSemanticResults(results, "", true, 2); len(got) != 2 {
		t.Errorf("finishSemanticResults trimmed to %d, want 2", len(got))
	}

	if v, a := pageValidity(&store.Page{Properties: "not json"}); v != "" || a != "" {
		t.Errorf("pageValidity(bad json) = %q, %q", v, a)
	}
}
