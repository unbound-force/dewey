package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/unbound-force/dewey/backend"
	"github.com/unbound-force/dewey/types"
)

func TestSearch_BruteForce_Success(t *testing.T) {
	mb := newMockBackend()
	mb.addPage(types.PageEntity{Name: "page-a", OriginalName: "Page A"},
		types.BlockEntity{UUID: "b1", Content: "This contains the search term foo"},
		types.BlockEntity{UUID: "b2", Content: "This does not match"},
	)
	mb.addPage(types.PageEntity{Name: "page-b", OriginalName: "Page B"},
		types.BlockEntity{UUID: "b3", Content: "Another foo match here"},
	)
	s := NewSearch(mb)

	result, _, err := s.Search(context.Background(), nil, types.SearchInput{
		Query: "foo",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Search() returned error result")
	}

	var parsed map[string]any
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	count, _ := parsed["count"].(float64)
	if count != 2 {
		t.Errorf("count = %v, want 2", count)
	}

	results, _ := parsed["results"].([]any)
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestSearch_BruteForce_NoResults(t *testing.T) {
	mb := newMockBackend()
	mb.addPage(types.PageEntity{Name: "page-a"},
		types.BlockEntity{UUID: "b1", Content: "nothing relevant here"},
	)
	s := NewSearch(mb)

	result, _, err := s.Search(context.Background(), nil, types.SearchInput{
		Query: "zzz-nonexistent",
	})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if result.IsError {
		t.Fatal("no results is not an error")
	}
}

func TestSearch_BruteForce_LimitRespected(t *testing.T) {
	mb := newMockBackend()
	mb.addPage(types.PageEntity{Name: "page-a", OriginalName: "Page A"},
		types.BlockEntity{UUID: "b1", Content: "match one"},
		types.BlockEntity{UUID: "b2", Content: "match two"},
		types.BlockEntity{UUID: "b3", Content: "match three"},
	)
	s := NewSearch(mb)

	result, _, err := s.Search(context.Background(), nil, types.SearchInput{
		Query: "match",
		Limit: 2,
	})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Search() returned error result")
	}

	var parsed map[string]any
	text := extractText(t, result)
	_ = json.Unmarshal([]byte(text), &parsed)

	count, _ := parsed["count"].(float64)
	if count != 2 {
		t.Errorf("count = %v, want 2 (limited)", count)
	}
}

func TestSearch_BruteForce_Compact(t *testing.T) {
	mb := newMockBackend()
	mb.addPage(types.PageEntity{Name: "p1", OriginalName: "P1"},
		types.BlockEntity{UUID: "b1", Content: "found the keyword"},
	)
	s := NewSearch(mb)

	result, _, err := s.Search(context.Background(), nil, types.SearchInput{
		Query:   "keyword",
		Compact: true,
	})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Search() returned error result")
	}

	var parsed map[string]any
	text := extractText(t, result)
	_ = json.Unmarshal([]byte(text), &parsed)

	results, _ := parsed["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r, _ := results[0].(map[string]any)
	// Compact mode should NOT have "parsed" field.
	if _, ok := r["parsed"]; ok {
		t.Error("compact mode should not include parsed field")
	}
	if r["uuid"] != "b1" {
		t.Errorf("uuid = %v, want %q", r["uuid"], "b1")
	}
}

func TestSearch_Indexed(t *testing.T) {
	mb := newMockBackend()
	fts := &mockFullTextSearcher{
		results: []backend.SearchHit{
			{PageName: "index-page", UUID: "idx-1", Content: "indexed match"},
		},
	}
	combined := &mockBackendWithFullTextSearch{mockBackend: mb, mockFullTextSearcher: fts}
	s := NewSearch(combined)

	result, _, err := s.Search(context.Background(), nil, types.SearchInput{
		Query: "indexed",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Search() returned error result")
	}

	var parsed map[string]any
	text := extractText(t, result)
	_ = json.Unmarshal([]byte(text), &parsed)

	count, _ := parsed["count"].(float64)
	if count != 1 {
		t.Errorf("count = %v, want 1", count)
	}
}

func TestSearch_BruteForce_GetAllPagesError(t *testing.T) {
	mb := newMockBackend()
	mb.getAllPagesErr = fmt.Errorf("backend down")
	s := NewSearch(mb)

	result, _, err := s.Search(context.Background(), nil, types.SearchInput{
		Query: "anything",
	})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when GetAllPages fails")
	}
}

func TestFindByTag_ViaDataScript(t *testing.T) {
	mb := newMockBackend()
	// Mock DataScript query for tag search.
	query := `[:find (pull ?b [:block/uuid :block/content {:block/page [:block/name :block/original-name]}])
		:where
		[?b :block/refs ?ref]
		[?ref :block/name "project"]]`
	mb.queryResults[query] = json.RawMessage(`[[{"uuid":"tag-block-1","content":"Important #project item","page":{"name":"tasks"}}]]`)

	s := NewSearch(mb)

	result, _, err := s.FindByTag(context.Background(), nil, types.FindByTagInput{
		Tag: "project",
	})
	if err != nil {
		t.Fatalf("FindByTag() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("FindByTag() returned error result")
	}

	var parsed map[string]any
	text := extractText(t, result)
	_ = json.Unmarshal([]byte(text), &parsed)

	if parsed["tag"] != "project" {
		t.Errorf("tag = %v, want %q", parsed["tag"], "project")
	}
	if parsed["count"] != float64(1) {
		t.Errorf("count = %v, want 1", parsed["count"])
	}

	// Verify result contains the matching block content and source page.
	results, ok := parsed["results"].([]any)
	if !ok {
		t.Fatalf("results missing or wrong type: %T", parsed["results"])
	}
	if len(results) != 1 {
		t.Fatalf("results length = %d, want 1", len(results))
	}
	match, ok := results[0].(map[string]any)
	if !ok {
		t.Fatalf("results[0] is not a map: %T", results[0])
	}
	content, ok := match["content"].(string)
	if !ok || content == "" {
		t.Error("results[0].content missing or empty")
	}
	if !strings.Contains(content, "project") {
		t.Errorf("results[0].content = %q, want it to contain 'project'", content)
	}
	page, _ := match["page"].(string)
	if page != "tasks" {
		t.Errorf("results[0].page = %q, want %q", page, "tasks")
	}
}

func TestFindByTag_ViaTagSearcher(t *testing.T) {
	mb := newMockBackend()
	ts := &mockTagSearcher{
		results: []backend.TagResult{
			{
				Page: "projects-page",
				Blocks: []types.BlockEntity{
					{UUID: "ts-1", Content: "Build #project A"},
					{UUID: "ts-2", Content: "Ship #project B"},
				},
			},
		},
	}
	combined := &mockBackendWithTagSearch{mockBackend: mb, mockTagSearcher: ts}
	s := NewSearch(combined)

	result, _, err := s.FindByTag(context.Background(), nil, types.FindByTagInput{
		Tag: "project",
	})
	if err != nil {
		t.Fatalf("FindByTag() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("FindByTag() returned error result")
	}

	var parsed map[string]any
	text := extractText(t, result)
	_ = json.Unmarshal([]byte(text), &parsed)

	if parsed["count"] != float64(2) {
		t.Errorf("count = %v, want 2", parsed["count"])
	}

	// Verify results contain blocks from the mock tag searcher with page context.
	results, ok := parsed["results"].([]any)
	if !ok {
		t.Fatalf("results missing or wrong type: %T", parsed["results"])
	}
	if len(results) != 2 {
		t.Fatalf("results length = %d, want 2", len(results))
	}
	for i, r := range results {
		rm, ok := r.(map[string]any)
		if !ok {
			t.Fatalf("results[%d] is not a map: %T", i, r)
		}
		content, ok := rm["content"].(string)
		if !ok || content == "" {
			t.Errorf("results[%d].content missing or empty", i)
		}
		if !strings.Contains(content, "project") {
			t.Errorf("results[%d].content = %q, want it to contain 'project'", i, content)
		}
	}
}

func TestFindByTag_QueryError(t *testing.T) {
	mb := newMockBackend()
	mb.queryErr = fmt.Errorf("query failed")
	s := NewSearch(mb)

	result, _, err := s.FindByTag(context.Background(), nil, types.FindByTagInput{
		Tag: "broken",
	})
	if err != nil {
		t.Fatalf("FindByTag() error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when query fails")
	}
}

func TestQueryProperties_ViaDataScript(t *testing.T) {
	mb := newMockBackend()
	// Mock the DataScript query for property search (with value).
	query := `[:find (pull ?b [:block/uuid :block/content :block/properties {:block/page [:block/name :block/original-name]}])
			:where
			[?b :block/properties ?props]
			[(get ?props :type) ?v]
			[(str ?v) ?vs]
			[(clojure.string/includes? ?vs "analysis")]]`
	mb.queryResults[query] = json.RawMessage(`[[{"uuid":"prop-1","content":"Analysis page","properties":{"type":"analysis"}}]]`)

	s := NewSearch(mb)

	result, _, err := s.QueryProperties(context.Background(), nil, types.QueryPropertiesInput{
		Property: "type",
		Value:    "analysis",
	})
	if err != nil {
		t.Fatalf("QueryProperties() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("QueryProperties() returned error result")
	}
}

func TestQueryProperties_ViaPropertySearcher(t *testing.T) {
	mb := newMockBackend()
	ps := &mockPropertySearcher{
		results: map[string][]backend.PropertyResult{
			"type:analysis:eq": {
				{Type: "page", Name: "my-analysis", Properties: map[string]any{"type": "analysis"}},
			},
		},
	}
	combined := &mockBackendWithPropertySearch{mockBackend: mb, mockPropertySearcher: ps}
	s := NewSearch(combined)

	result, _, err := s.QueryProperties(context.Background(), nil, types.QueryPropertiesInput{
		Property: "type",
		Value:    "analysis",
	})
	if err != nil {
		t.Fatalf("QueryProperties() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("QueryProperties() returned error result")
	}

	var parsed map[string]any
	text := extractText(t, result)
	_ = json.Unmarshal([]byte(text), &parsed)

	if parsed["count"] != float64(1) {
		t.Errorf("count = %v, want 1", parsed["count"])
	}
}

func TestQueryProperties_NoValue(t *testing.T) {
	mb := newMockBackend()
	// Query with no value specified — just check property exists.
	query := `[:find (pull ?b [:block/uuid :block/content :block/properties {:block/page [:block/name :block/original-name]}])
			:where
			[?b :block/properties ?props]
			[(get ?props :status)]]`
	mb.queryResults[query] = json.RawMessage(`[]`)

	s := NewSearch(mb)

	result, _, err := s.QueryProperties(context.Background(), nil, types.QueryPropertiesInput{
		Property: "status",
	})
	if err != nil {
		t.Fatalf("QueryProperties() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("QueryProperties() returned error result")
	}
}

func TestQueryProperties_QueryError(t *testing.T) {
	mb := newMockBackend()
	mb.queryErr = fmt.Errorf("query error")
	s := NewSearch(mb)

	result, _, err := s.QueryProperties(context.Background(), nil, types.QueryPropertiesInput{
		Property: "type",
		Value:    "broken",
	})
	if err != nil {
		t.Fatalf("QueryProperties() error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error when query fails")
	}
}
