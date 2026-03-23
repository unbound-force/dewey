package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/unbound-force/dewey/backend"
	"github.com/unbound-force/dewey/types"
)

func TestDecisionCheck_Success(t *testing.T) {
	mb := newMockBackend()
	// Mock DataScript query returning decision blocks.
	query := `[:find (pull ?b [:block/uuid :block/content
		{:block/page [:block/name :block/original-name]}])
		:where
		[?b :block/refs ?ref]
		[?ref :block/name "decision"]]`
	mb.queryResults[query] = json.RawMessage(`[
		[{"uuid":"d1","content":"DECIDE Should we launch? #decision\ndeadline:: 2099-12-31","page":{"name":"strategy"}}],
		[{"uuid":"d2","content":"DONE Chose React #decision\ndeadline:: 2025-01-01\nresolved:: 2025-01-10","page":{"name":"tech"}}]
	]`)

	d := NewDecision(mb)

	result, _, err := d.DecisionCheck(context.Background(), nil, types.DecisionCheckInput{})
	if err != nil {
		t.Fatalf("DecisionCheck() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("DecisionCheck() returned error result")
	}

	var parsed map[string]any
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if parsed["total"] != float64(2) {
		t.Errorf("total = %v, want 2", parsed["total"])
	}
	if parsed["open"] != float64(1) {
		t.Errorf("open = %v, want 1", parsed["open"])
	}
	if parsed["resolved"] != float64(1) {
		t.Errorf("resolved = %v, want 1", parsed["resolved"])
	}
}

func TestDecisionCheck_IncludeResolved(t *testing.T) {
	mb := newMockBackend()
	query := `[:find (pull ?b [:block/uuid :block/content
		{:block/page [:block/name :block/original-name]}])
		:where
		[?b :block/refs ?ref]
		[?ref :block/name "decision"]]`
	mb.queryResults[query] = json.RawMessage(`[
		[{"uuid":"d1","content":"DECIDE Open decision #decision\ndeadline:: 2099-12-31","page":{"name":"p1"}}],
		[{"uuid":"d2","content":"DONE Resolved decision #decision\nresolved:: 2025-01-01","page":{"name":"p2"}}]
	]`)

	d := NewDecision(mb)

	result, _, err := d.DecisionCheck(context.Background(), nil, types.DecisionCheckInput{
		IncludeResolved: true,
	})
	if err != nil {
		t.Fatalf("DecisionCheck() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("DecisionCheck() returned error result")
	}

	var parsed map[string]any
	text := extractText(t, result)
	_ = json.Unmarshal([]byte(text), &parsed)

	decisions, _ := parsed["decisions"].([]any)
	if len(decisions) != 2 {
		t.Errorf("expected 2 decisions with includeResolved, got %d", len(decisions))
	}
}

func TestDecisionCheck_NoDecisions(t *testing.T) {
	mb := newMockBackend()
	query := `[:find (pull ?b [:block/uuid :block/content
		{:block/page [:block/name :block/original-name]}])
		:where
		[?b :block/refs ?ref]
		[?ref :block/name "decision"]]`
	mb.queryResults[query] = json.RawMessage(`[]`)

	d := NewDecision(mb)

	result, _, err := d.DecisionCheck(context.Background(), nil, types.DecisionCheckInput{})
	if err != nil {
		t.Fatalf("DecisionCheck() error: %v", err)
	}
	// No decisions is not an error, just a text message.
	if result.IsError {
		t.Fatal("no decisions should not be an error")
	}
}

func TestDecisionCheck_QueryError(t *testing.T) {
	mb := newMockBackend()
	mb.queryErr = fmt.Errorf("query failed")
	d := NewDecision(mb)

	result, _, err := d.DecisionCheck(context.Background(), nil, types.DecisionCheckInput{})
	if err != nil {
		t.Fatalf("DecisionCheck() error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when query fails")
	}
}

func TestDecisionCreate_Success(t *testing.T) {
	mb := newMockBackend()
	mb.appendBlockResult = &types.BlockEntity{UUID: "new-decision-uuid"}
	d := NewDecision(mb)

	result, _, err := d.DecisionCreate(context.Background(), nil, types.DecisionCreateInput{
		Page:     "strategy",
		Question: "Should we pivot?",
		Deadline: "2026-06-01",
		Options:  []string{"Yes", "No", "Partially"},
		Context:  "Market conditions changed",
	})
	if err != nil {
		t.Fatalf("DecisionCreate() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("DecisionCreate() returned error result")
	}

	// Verify the block was appended with proper content.
	if len(mb.appendedBlocks) != 1 {
		t.Fatalf("expected 1 appended block, got %d", len(mb.appendedBlocks))
	}
	content := mb.appendedBlocks[0].content
	if mb.appendedBlocks[0].page != "strategy" {
		t.Errorf("page = %q, want %q", mb.appendedBlocks[0].page, "strategy")
	}

	// Should contain DECIDE marker, #decision tag, deadline, options, context.
	for _, want := range []string{"DECIDE", "#decision", "deadline:: 2026-06-01", "options::", "context::"} {
		if !containsSubstring(content, want) {
			t.Errorf("block content missing %q: %s", want, content)
		}
	}

	var parsed map[string]any
	text := extractText(t, result)
	_ = json.Unmarshal([]byte(text), &parsed)

	if parsed["created"] != true {
		t.Errorf("created = %v, want true", parsed["created"])
	}
	if parsed["uuid"] != "new-decision-uuid" {
		t.Errorf("uuid = %v, want %q", parsed["uuid"], "new-decision-uuid")
	}
}

func TestDecisionCreate_Error(t *testing.T) {
	mb := newMockBackend()
	mb.appendBlockErr = fmt.Errorf("append failed")
	d := NewDecision(mb)

	result, _, err := d.DecisionCreate(context.Background(), nil, types.DecisionCreateInput{
		Page:     "strategy",
		Question: "Will fail",
		Deadline: "2026-01-01",
	})
	if err != nil {
		t.Fatalf("DecisionCreate() error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when backend fails")
	}
}

func TestAnalysisHealth_Success(t *testing.T) {
	mb := newMockBackend()
	// Set up DataScript query to find analysis pages.
	analysisQuery := `[:find (pull ?p [:block/name :block/original-name])
		:where
		[?p :block/name]
		[?p :block/properties ?props]
		[(get ?props :type) ?t]
		[(contains? #{"analysis" "strategy" "assessment"} ?t)]]`
	mb.queryResults[analysisQuery] = json.RawMessage(`[[{"name":"market-analysis"}],[{"name":"tech-strategy"}]]`)

	// Set up pages with blocks.
	mb.addPage(types.PageEntity{Name: "market-analysis"},
		types.BlockEntity{UUID: "b1", Content: "See [[competitor-a]] and [[competitor-b]] and [[market-trends]]"},
	)
	mb.addPage(types.PageEntity{Name: "tech-strategy"},
		types.BlockEntity{UUID: "b2", Content: "DECIDE something #decision"},
	)

	d := NewDecision(mb)

	result, _, err := d.AnalysisHealth(context.Background(), nil, types.AnalysisHealthInput{})
	if err != nil {
		t.Fatalf("AnalysisHealth() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("AnalysisHealth() returned error result")
	}

	var parsed map[string]any
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if parsed["total"] != float64(2) {
		t.Errorf("total = %v, want 2", parsed["total"])
	}
	// market-analysis has 3+ links → healthy.
	// tech-strategy has a decision → healthy.
	if parsed["healthy"] != float64(2) {
		t.Errorf("healthy = %v, want 2", parsed["healthy"])
	}
}

func TestAnalysisHealth_ViaPropertySearcher(t *testing.T) {
	mb := newMockBackend()
	ps := &mockPropertySearcher{
		results: map[string][]backend.PropertyResult{
			"type:analysis:eq": {
				{Type: "page", Name: "found-analysis", Properties: map[string]any{"type": "analysis"}},
			},
			"type:strategy:eq":   {},
			"type:assessment:eq": {},
		},
	}
	combined := &mockBackendWithPropertySearch{mockBackend: mb, mockPropertySearcher: ps}

	// Set up the page with blocks for health check.
	mb.addPage(types.PageEntity{Name: "found-analysis"},
		types.BlockEntity{UUID: "b1", Content: "Links to [[a]] and [[b]] and [[c]]"},
	)

	d := NewDecision(combined)

	result, _, err := d.AnalysisHealth(context.Background(), nil, types.AnalysisHealthInput{})
	if err != nil {
		t.Fatalf("AnalysisHealth() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("AnalysisHealth() returned error result")
	}

	var parsed map[string]any
	text := extractText(t, result)
	_ = json.Unmarshal([]byte(text), &parsed)

	if parsed["total"] != float64(1) {
		t.Errorf("total = %v, want 1", parsed["total"])
	}
}

func TestAnalysisHealth_Error(t *testing.T) {
	mb := newMockBackend()
	mb.queryErr = fmt.Errorf("query failed")
	d := NewDecision(mb)

	result, _, err := d.AnalysisHealth(context.Background(), nil, types.AnalysisHealthInput{})
	if err != nil {
		t.Fatalf("AnalysisHealth() error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when query fails")
	}
}

func TestFindAnalysisPages_ViaDataScript(t *testing.T) {
	mb := newMockBackend()
	query := `[:find (pull ?p [:block/name :block/original-name])
		:where
		[?p :block/name]
		[?p :block/properties ?props]
		[(get ?props :type) ?t]
		[(contains? #{"analysis" "strategy" "assessment"} ?t)]]`
	mb.queryResults[query] = json.RawMessage(`[[{"name":"analysis-page"}],[{"name":"strategy-page"}]]`)

	d := NewDecision(mb)

	names, err := d.findAnalysisPages(context.Background())
	if err != nil {
		t.Fatalf("findAnalysisPages() error: %v", err)
	}
	if len(names) != 2 {
		t.Fatalf("expected 2 analysis pages, got %d", len(names))
	}
	if names[0] != "analysis-page" {
		t.Errorf("first page = %q, want %q", names[0], "analysis-page")
	}
}

func TestFindAnalysisPages_QueryError(t *testing.T) {
	mb := newMockBackend()
	mb.queryErr = fmt.Errorf("query failed")
	d := NewDecision(mb)

	_, err := d.findAnalysisPages(context.Background())
	if err == nil {
		t.Fatal("expected error when query fails")
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstringHelper(s, substr))
}

func containsSubstringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
