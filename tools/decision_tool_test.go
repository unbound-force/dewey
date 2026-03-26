package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

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

func TestDecisionResolve_Success(t *testing.T) {
	mb := newMockBackend()
	mb.addBlock(types.BlockEntity{
		UUID:    "resolve-uuid",
		Content: "DECIDE Should we use Go? #decision\ndeadline:: 2026-06-01",
	})
	d := NewDecision(mb)

	result, _, err := d.DecisionResolve(context.Background(), nil, types.DecisionResolveInput{
		UUID:    "resolve-uuid",
		Outcome: "Yes, Go is the right choice",
	})
	if err != nil {
		t.Fatalf("DecisionResolve() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("DecisionResolve() returned error result")
	}

	// Verify the block was updated.
	if len(mb.updatedBlocks) != 1 {
		t.Fatalf("expected 1 updated block, got %d", len(mb.updatedBlocks))
	}
	updated := mb.updatedBlocks[0]
	if updated.uuid != "resolve-uuid" {
		t.Errorf("updated uuid = %q, want %q", updated.uuid, "resolve-uuid")
	}

	// DECIDE marker should be replaced with DONE.
	if !containsSubstring(updated.content, "DONE ") {
		t.Errorf("updated content missing DONE marker: %s", updated.content)
	}
	if containsSubstring(updated.content, "DECIDE ") {
		t.Errorf("updated content still has DECIDE marker: %s", updated.content)
	}

	// Should have resolved property.
	if !containsSubstring(updated.content, "resolved::") {
		t.Errorf("updated content missing resolved property: %s", updated.content)
	}

	// Should have outcome property.
	if !containsSubstring(updated.content, "outcome:: Yes, Go is the right choice") {
		t.Errorf("updated content missing outcome: %s", updated.content)
	}

	// Verify JSON response.
	var parsed map[string]any
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if parsed["resolved"] != true {
		t.Errorf("resolved = %v, want true", parsed["resolved"])
	}
	if parsed["uuid"] != "resolve-uuid" {
		t.Errorf("uuid = %v, want %q", parsed["uuid"], "resolve-uuid")
	}
	if parsed["outcome"] != "Yes, Go is the right choice" {
		t.Errorf("outcome = %v, want %q", parsed["outcome"], "Yes, Go is the right choice")
	}
	if _, ok := parsed["date"]; !ok {
		t.Errorf("response missing date field")
	}
}

func TestDecisionResolve_WithoutOutcome(t *testing.T) {
	mb := newMockBackend()
	mb.addBlock(types.BlockEntity{
		UUID:    "resolve-no-outcome",
		Content: "DECIDE Pick a database #decision\ndeadline:: 2026-03-01",
	})
	d := NewDecision(mb)

	result, _, err := d.DecisionResolve(context.Background(), nil, types.DecisionResolveInput{
		UUID: "resolve-no-outcome",
	})
	if err != nil {
		t.Fatalf("DecisionResolve() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("DecisionResolve() returned error result")
	}

	// Should still update the block with DONE and resolved date.
	if len(mb.updatedBlocks) != 1 {
		t.Fatalf("expected 1 updated block, got %d", len(mb.updatedBlocks))
	}
	updated := mb.updatedBlocks[0]
	if !containsSubstring(updated.content, "DONE ") {
		t.Errorf("updated content missing DONE marker: %s", updated.content)
	}
	if !containsSubstring(updated.content, "resolved::") {
		t.Errorf("updated content missing resolved property: %s", updated.content)
	}
	// No outcome property should be added when outcome is empty.
	if containsSubstring(updated.content, "outcome::") {
		t.Errorf("updated content should not have outcome property when empty: %s", updated.content)
	}
}

func TestDecisionResolve_BlockNotFound(t *testing.T) {
	mb := newMockBackend()
	d := NewDecision(mb)

	result, _, err := d.DecisionResolve(context.Background(), nil, types.DecisionResolveInput{
		UUID:    "nonexistent-uuid",
		Outcome: "doesn't matter",
	})
	if err != nil {
		t.Fatalf("DecisionResolve() error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when block not found")
	}
}

func TestDecisionResolve_GetBlockError(t *testing.T) {
	mb := newMockBackend()
	mb.getBlockErr = fmt.Errorf("connection refused")
	d := NewDecision(mb)

	result, _, err := d.DecisionResolve(context.Background(), nil, types.DecisionResolveInput{
		UUID: "any-uuid",
	})
	if err != nil {
		t.Fatalf("DecisionResolve() error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when GetBlock fails")
	}
}

func TestDecisionResolve_UpdateBlockError(t *testing.T) {
	mb := newMockBackend()
	mb.addBlock(types.BlockEntity{
		UUID:    "update-fail-uuid",
		Content: "DECIDE Something #decision\ndeadline:: 2026-01-01",
	})
	mb.updateBlockErr = fmt.Errorf("write failed")
	d := NewDecision(mb)

	result, _, err := d.DecisionResolve(context.Background(), nil, types.DecisionResolveInput{
		UUID:    "update-fail-uuid",
		Outcome: "Decided but can't save",
	})
	if err != nil {
		t.Fatalf("DecisionResolve() error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when UpdateBlock fails")
	}
}

func TestDecisionDefer_Success(t *testing.T) {
	mb := newMockBackend()
	mb.addBlock(types.BlockEntity{
		UUID:    "defer-uuid",
		Content: "DECIDE Should we hire? #decision\ndeadline:: 2026-01-01",
	})
	d := NewDecision(mb)

	result, _, err := d.DecisionDefer(context.Background(), nil, types.DecisionDeferInput{
		UUID:        "defer-uuid",
		NewDeadline: "2026-06-01",
		Reason:      "Need more budget data",
	})
	if err != nil {
		t.Fatalf("DecisionDefer() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("DecisionDefer() returned error result")
	}

	// Verify the block was updated.
	if len(mb.updatedBlocks) != 1 {
		t.Fatalf("expected 1 updated block, got %d", len(mb.updatedBlocks))
	}
	updated := mb.updatedBlocks[0]
	if updated.uuid != "defer-uuid" {
		t.Errorf("updated uuid = %q, want %q", updated.uuid, "defer-uuid")
	}

	// Should update deadline.
	if !containsSubstring(updated.content, "deadline:: 2026-06-01") {
		t.Errorf("updated content missing new deadline: %s", updated.content)
	}

	// Should have deferred count of 1 (first deferral).
	if !containsSubstring(updated.content, "deferred:: 1") {
		t.Errorf("updated content missing deferred count: %s", updated.content)
	}

	// Should have deferred-on date.
	if !containsSubstring(updated.content, "deferred-on::") {
		t.Errorf("updated content missing deferred-on property: %s", updated.content)
	}

	// Reason should be inserted as child block.
	if len(mb.insertedBlocks) != 1 {
		t.Fatalf("expected 1 inserted block for reason, got %d", len(mb.insertedBlocks))
	}
	if mb.insertedBlocks[0].parent != "defer-uuid" {
		t.Errorf("inserted block parent = %q, want %q", mb.insertedBlocks[0].parent, "defer-uuid")
	}
	if !containsSubstring(mb.insertedBlocks[0].content, "Need more budget data") {
		t.Errorf("inserted block content missing reason: %s", mb.insertedBlocks[0].content)
	}

	// Verify JSON response.
	var parsed map[string]any
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if parsed["deferred"] != true {
		t.Errorf("deferred = %v, want true", parsed["deferred"])
	}
	if parsed["uuid"] != "defer-uuid" {
		t.Errorf("uuid = %v, want %q", parsed["uuid"], "defer-uuid")
	}
	if parsed["newDeadline"] != "2026-06-01" {
		t.Errorf("newDeadline = %v, want %q", parsed["newDeadline"], "2026-06-01")
	}
	if parsed["deferCount"] != float64(1) {
		t.Errorf("deferCount = %v, want 1", parsed["deferCount"])
	}
	if _, ok := parsed["warning"]; ok {
		t.Errorf("should not have warning on first deferral")
	}
}

func TestDecisionDefer_IncrementsExistingCount(t *testing.T) {
	mb := newMockBackend()
	mb.addBlock(types.BlockEntity{
		UUID:    "defer-again-uuid",
		Content: "DECIDE Launch timing #decision\ndeadline:: 2026-03-01\ndeferred:: 1\ndeferred-on:: 2026-01-15",
	})
	d := NewDecision(mb)

	result, _, err := d.DecisionDefer(context.Background(), nil, types.DecisionDeferInput{
		UUID:        "defer-again-uuid",
		NewDeadline: "2026-09-01",
		Reason:      "Market uncertainty",
	})
	if err != nil {
		t.Fatalf("DecisionDefer() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("DecisionDefer() returned error result")
	}

	// Verify defer count incremented to 2.
	var parsed map[string]any
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if parsed["deferCount"] != float64(2) {
		t.Errorf("deferCount = %v, want 2", parsed["deferCount"])
	}
	// Should not have warning at count 2.
	if _, ok := parsed["warning"]; ok {
		t.Errorf("should not have warning at deferCount 2")
	}
}

func TestDecisionDefer_WarningAtThirdDeferral(t *testing.T) {
	mb := newMockBackend()
	mb.addBlock(types.BlockEntity{
		UUID:    "defer-many-uuid",
		Content: "DECIDE Pricing model #decision\ndeadline:: 2026-05-01\ndeferred:: 2\ndeferred-on:: 2026-03-01",
	})
	d := NewDecision(mb)

	result, _, err := d.DecisionDefer(context.Background(), nil, types.DecisionDeferInput{
		UUID:        "defer-many-uuid",
		NewDeadline: "2026-12-01",
		Reason:      "Still undecided",
	})
	if err != nil {
		t.Fatalf("DecisionDefer() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("DecisionDefer() returned error result")
	}

	var parsed map[string]any
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if parsed["deferCount"] != float64(3) {
		t.Errorf("deferCount = %v, want 3", parsed["deferCount"])
	}
	warning, ok := parsed["warning"]
	if !ok {
		t.Fatal("expected warning at deferCount >= 3")
	}
	warnStr, _ := warning.(string)
	if !containsSubstring(warnStr, "3 times") {
		t.Errorf("warning = %q, expected to mention '3 times'", warnStr)
	}
}

func TestDecisionDefer_WithoutReason(t *testing.T) {
	mb := newMockBackend()
	mb.addBlock(types.BlockEntity{
		UUID:    "defer-no-reason",
		Content: "DECIDE Office location #decision\ndeadline:: 2026-02-01",
	})
	d := NewDecision(mb)

	result, _, err := d.DecisionDefer(context.Background(), nil, types.DecisionDeferInput{
		UUID:        "defer-no-reason",
		NewDeadline: "2026-08-01",
	})
	if err != nil {
		t.Fatalf("DecisionDefer() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("DecisionDefer() returned error result")
	}

	// Block should be updated.
	if len(mb.updatedBlocks) != 1 {
		t.Fatalf("expected 1 updated block, got %d", len(mb.updatedBlocks))
	}

	// No reason child block should be inserted when reason is empty.
	if len(mb.insertedBlocks) != 0 {
		t.Errorf("expected 0 inserted blocks when no reason, got %d", len(mb.insertedBlocks))
	}
}

func TestDecisionDefer_BlockNotFound(t *testing.T) {
	mb := newMockBackend()
	d := NewDecision(mb)

	result, _, err := d.DecisionDefer(context.Background(), nil, types.DecisionDeferInput{
		UUID:        "nonexistent-uuid",
		NewDeadline: "2026-12-01",
		Reason:      "doesn't matter",
	})
	if err != nil {
		t.Fatalf("DecisionDefer() error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when block not found")
	}
}

func TestDecisionDefer_GetBlockError(t *testing.T) {
	mb := newMockBackend()
	mb.getBlockErr = fmt.Errorf("connection refused")
	d := NewDecision(mb)

	result, _, err := d.DecisionDefer(context.Background(), nil, types.DecisionDeferInput{
		UUID:        "any-uuid",
		NewDeadline: "2026-12-01",
	})
	if err != nil {
		t.Fatalf("DecisionDefer() error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when GetBlock fails")
	}
}

func TestDecisionDefer_UpdateBlockError(t *testing.T) {
	mb := newMockBackend()
	mb.addBlock(types.BlockEntity{
		UUID:    "defer-update-fail",
		Content: "DECIDE Something #decision\ndeadline:: 2026-01-01",
	})
	mb.updateBlockErr = fmt.Errorf("write failed")
	d := NewDecision(mb)

	result, _, err := d.DecisionDefer(context.Background(), nil, types.DecisionDeferInput{
		UUID:        "defer-update-fail",
		NewDeadline: "2026-06-01",
		Reason:      "Deferred but can't save",
	})
	if err != nil {
		t.Fatalf("DecisionDefer() error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when UpdateBlock fails")
	}
}

func TestFindDecisionsViaTagSearch_Success(t *testing.T) {
	mb := newMockBackend()
	ts := &mockTagSearcher{
		results: []backend.TagResult{
			{
				Page: "strategy",
				Blocks: []types.BlockEntity{
					{UUID: "d1", Content: "DECIDE Should we launch? #decision\ndeadline:: 2099-12-31"},
					{UUID: "d2", Content: "DONE Chose React #decision\nresolved:: 2025-01-10"},
				},
			},
			{
				Page: "planning",
				Blocks: []types.BlockEntity{
					{UUID: "d3", Content: "TODO Review budget #decision"},
				},
			},
		},
	}
	combined := &mockBackendWithTagSearch{mockBackend: mb, mockTagSearcher: ts}
	d := NewDecision(combined)

	today := parseTestDate(t, "2025-06-15")
	decisions, err := d.findDecisionsViaTagSearch(context.Background(), ts, today)
	if err != nil {
		t.Fatalf("findDecisionsViaTagSearch() error: %v", err)
	}

	// Should find 3 blocks with markers: DECIDE, DONE, TODO.
	if len(decisions) != 3 {
		t.Fatalf("expected 3 decisions, got %d", len(decisions))
	}

	// Verify first decision has correct page and marker.
	if decisions[0].Page != "strategy" {
		t.Errorf("decisions[0].Page = %q, want %q", decisions[0].Page, "strategy")
	}
	if decisions[0].Marker != "DECIDE" {
		t.Errorf("decisions[0].Marker = %q, want %q", decisions[0].Marker, "DECIDE")
	}
	if decisions[0].UUID != "d1" {
		t.Errorf("decisions[0].UUID = %q, want %q", decisions[0].UUID, "d1")
	}

	// Verify resolved decision has DONE marker.
	if decisions[1].Marker != "DONE" {
		t.Errorf("decisions[1].Marker = %q, want %q", decisions[1].Marker, "DONE")
	}
	if decisions[1].Resolved != "2025-01-10" {
		t.Errorf("decisions[1].Resolved = %q, want %q", decisions[1].Resolved, "2025-01-10")
	}

	// Verify third decision from second page.
	if decisions[2].Page != "planning" {
		t.Errorf("decisions[2].Page = %q, want %q", decisions[2].Page, "planning")
	}
	if decisions[2].Marker != "TODO" {
		t.Errorf("decisions[2].Marker = %q, want %q", decisions[2].Marker, "TODO")
	}
}

func TestFindDecisionsViaTagSearch_NoMatchingBlocks(t *testing.T) {
	mb := newMockBackend()
	ts := &mockTagSearcher{
		results: []backend.TagResult{},
	}
	combined := &mockBackendWithTagSearch{mockBackend: mb, mockTagSearcher: ts}
	d := NewDecision(combined)

	today := parseTestDate(t, "2025-06-15")
	decisions, err := d.findDecisionsViaTagSearch(context.Background(), ts, today)
	if err != nil {
		t.Fatalf("findDecisionsViaTagSearch() error: %v", err)
	}

	if len(decisions) != 0 {
		t.Errorf("expected 0 decisions when no tag results, got %d", len(decisions))
	}
}

func TestFindDecisionsViaTagSearch_SkipsBlocksWithoutMarker(t *testing.T) {
	mb := newMockBackend()
	ts := &mockTagSearcher{
		results: []backend.TagResult{
			{
				Page: "docs",
				Blocks: []types.BlockEntity{
					// This block mentions #decision but has no marker — should be skipped.
					{UUID: "doc1", Content: "Read about #decision making process"},
					// This block has a DECIDE marker — should be included.
					{UUID: "d1", Content: "DECIDE on framework #decision"},
				},
			},
		},
	}
	combined := &mockBackendWithTagSearch{mockBackend: mb, mockTagSearcher: ts}
	d := NewDecision(combined)

	today := parseTestDate(t, "2025-06-15")
	decisions, err := d.findDecisionsViaTagSearch(context.Background(), ts, today)
	if err != nil {
		t.Fatalf("findDecisionsViaTagSearch() error: %v", err)
	}

	// Only the DECIDE block should be returned.
	if len(decisions) != 1 {
		t.Fatalf("expected 1 decision (marker block), got %d", len(decisions))
	}
	if decisions[0].UUID != "d1" {
		t.Errorf("decisions[0].UUID = %q, want %q", decisions[0].UUID, "d1")
	}
}

func TestFindDecisionsViaTagSearch_SearchError(t *testing.T) {
	ts := &mockTagSearcher{
		err: fmt.Errorf("tag search unavailable"),
	}
	mb := newMockBackend()
	combined := &mockBackendWithTagSearch{mockBackend: mb, mockTagSearcher: ts}
	d := NewDecision(combined)

	today := parseTestDate(t, "2025-06-15")
	_, err := d.findDecisionsViaTagSearch(context.Background(), ts, today)
	if err == nil {
		t.Fatal("expected error when tag search fails")
	}
	if !containsSubstring(err.Error(), "decision tag search failed") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "decision tag search failed")
	}
}

func TestFindDecisionsViaTagSearch_DeadlineCalculation(t *testing.T) {
	mb := newMockBackend()
	ts := &mockTagSearcher{
		results: []backend.TagResult{
			{
				Page: "deadlines",
				Blocks: []types.BlockEntity{
					// Overdue decision (deadline in the past).
					{UUID: "overdue1", Content: "DECIDE Past deadline #decision\ndeadline:: 2025-01-01"},
					// Future decision.
					{UUID: "future1", Content: "DECIDE Future deadline #decision\ndeadline:: 2099-12-31"},
				},
			},
		},
	}
	combined := &mockBackendWithTagSearch{mockBackend: mb, mockTagSearcher: ts}
	d := NewDecision(combined)

	today := parseTestDate(t, "2025-06-15")
	decisions, err := d.findDecisionsViaTagSearch(context.Background(), ts, today)
	if err != nil {
		t.Fatalf("findDecisionsViaTagSearch() error: %v", err)
	}

	if len(decisions) != 2 {
		t.Fatalf("expected 2 decisions, got %d", len(decisions))
	}

	// Overdue decision should have negative days left and Overdue=true.
	if decisions[0].DaysLeft == nil {
		t.Fatal("overdue decision DaysLeft should not be nil")
	}
	if *decisions[0].DaysLeft >= 0 {
		t.Errorf("overdue decision DaysLeft = %d, want negative", *decisions[0].DaysLeft)
	}
	if !decisions[0].Overdue {
		t.Error("overdue decision Overdue = false, want true")
	}

	// Future decision should have positive days left and Overdue=false.
	if decisions[1].DaysLeft == nil {
		t.Fatal("future decision DaysLeft should not be nil")
	}
	if *decisions[1].DaysLeft <= 0 {
		t.Errorf("future decision DaysLeft = %d, want positive", *decisions[1].DaysLeft)
	}
	if decisions[1].Overdue {
		t.Error("future decision Overdue = true, want false")
	}
}

func TestFindDecisions_DispatchesToTagSearcher(t *testing.T) {
	mb := newMockBackend()
	ts := &mockTagSearcher{
		results: []backend.TagResult{
			{
				Page: "via-tag",
				Blocks: []types.BlockEntity{
					{UUID: "t1", Content: "DECIDE via tag search #decision"},
				},
			},
		},
	}
	combined := &mockBackendWithTagSearch{mockBackend: mb, mockTagSearcher: ts}
	d := NewDecision(combined)

	decisions, err := d.findDecisions(context.Background())
	if err != nil {
		t.Fatalf("findDecisions() error: %v", err)
	}

	if len(decisions) != 1 {
		t.Fatalf("expected 1 decision via tag search dispatch, got %d", len(decisions))
	}
	if decisions[0].Page != "via-tag" {
		t.Errorf("decision page = %q, want %q", decisions[0].Page, "via-tag")
	}
}

// parseTestDate parses a YYYY-MM-DD date string for test use.
func parseTestDate(t *testing.T, s string) time.Time {
	t.Helper()
	parsed, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("parseTestDate(%q): %v", s, err)
	}
	return parsed
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
