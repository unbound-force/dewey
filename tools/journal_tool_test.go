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

func TestJournalRange_Success(t *testing.T) {
	mb := newMockBackend()
	// Add journal pages for a 3-day range. Use the "2006-01-02" format.
	mb.addPage(types.PageEntity{Name: "2026-01-15"},
		types.BlockEntity{UUID: "j1", Content: "Morning standup"},
	)
	mb.addPage(types.PageEntity{Name: "2026-01-17"},
		types.BlockEntity{UUID: "j2", Content: "Afternoon review"},
	)

	j := NewJournal(mb)

	result, _, err := j.JournalRange(context.Background(), nil, types.JournalRangeInput{
		From:          "2026-01-15",
		To:            "2026-01-17",
		IncludeBlocks: true,
	})
	if err != nil {
		t.Fatalf("JournalRange() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("JournalRange() returned error result")
	}

	var parsed map[string]any
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if parsed["from"] != "2026-01-15" {
		t.Errorf("from = %v, want %q", parsed["from"], "2026-01-15")
	}
	if parsed["to"] != "2026-01-17" {
		t.Errorf("to = %v, want %q", parsed["to"], "2026-01-17")
	}

	// Verify entriesFound reflects matched journal pages.
	entriesFound, ok := parsed["entriesFound"].(float64)
	if !ok {
		t.Fatalf("entriesFound missing or wrong type: %v", parsed["entriesFound"])
	}
	if entriesFound != 2 {
		t.Errorf("entriesFound = %v, want 2", entriesFound)
	}

	// Verify journals array structure: each entry has date and pageName.
	journals, ok := parsed["journals"].([]any)
	if !ok {
		t.Fatalf("journals missing or wrong type: %T", parsed["journals"])
	}
	if len(journals) != 2 {
		t.Fatalf("journals length = %d, want 2", len(journals))
	}
	for i, entry := range journals {
		m, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("journals[%d] is not a map: %T", i, entry)
		}
		if _, ok := m["date"].(string); !ok {
			t.Errorf("journals[%d].date missing or not a string", i)
		}
		if _, ok := m["pageName"].(string); !ok {
			t.Errorf("journals[%d].pageName missing or not a string", i)
		}
	}
}

func TestJournalRange_InvalidFrom(t *testing.T) {
	mb := newMockBackend()
	j := NewJournal(mb)

	result, _, err := j.JournalRange(context.Background(), nil, types.JournalRangeInput{
		From: "not-a-date",
		To:   "2026-01-15",
	})
	if err != nil {
		t.Fatalf("JournalRange() error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid from date")
	}
}

func TestJournalRange_InvalidTo(t *testing.T) {
	mb := newMockBackend()
	j := NewJournal(mb)

	result, _, err := j.JournalRange(context.Background(), nil, types.JournalRangeInput{
		From: "2026-01-15",
		To:   "not-a-date",
	})
	if err != nil {
		t.Fatalf("JournalRange() error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid to date")
	}
}

func TestJournalRange_ToBeforeFrom(t *testing.T) {
	mb := newMockBackend()
	j := NewJournal(mb)

	result, _, err := j.JournalRange(context.Background(), nil, types.JournalRangeInput{
		From: "2026-01-20",
		To:   "2026-01-15",
	})
	if err != nil {
		t.Fatalf("JournalRange() error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error when to is before from")
	}
}

func TestJournalSearch_ViaDataScript(t *testing.T) {
	mb := newMockBackend()
	query := `[:find (pull ?b [:block/uuid :block/content {:block/page [:block/name :block/original-name :block/journal-day]}])
		:where
		[?b :block/content ?content]
		[?b :block/page ?p]
		[?p :block/journal? true]]`
	mb.queryResults[query] = json.RawMessage(`[
		[{"content":"Had a meeting about launch","page":{"name":"jan-15","original-name":"Jan 15th, 2026"}}],
		[{"content":"Reviewed financials","page":{"name":"jan-16","original-name":"Jan 16th, 2026"}}]
	]`)

	j := NewJournal(mb)

	result, _, err := j.JournalSearch(context.Background(), nil, types.JournalSearchInput{
		Query: "meeting",
	})
	if err != nil {
		t.Fatalf("JournalSearch() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("JournalSearch() returned error result")
	}

	var parsed map[string]any
	text := extractText(t, result)
	_ = json.Unmarshal([]byte(text), &parsed)

	if parsed["query"] != "meeting" {
		t.Errorf("query = %v, want %q", parsed["query"], "meeting")
	}
	if parsed["count"] != float64(1) {
		t.Errorf("count = %v, want 1 (only one block mentions meeting)", parsed["count"])
	}

	// Verify results array structure: each result has content, page, and parsed fields.
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
		t.Errorf("results[0].content missing or empty")
	}
	if !strings.Contains(content, "meeting") {
		t.Errorf("results[0].content = %q, want it to contain 'meeting'", content)
	}
	page, ok := match["page"].(string)
	if !ok {
		t.Error("results[0].page missing")
	}
	if page != "jan-15" && page != "Jan 15th, 2026" {
		t.Errorf("results[0].page = %q, want journal page name", page)
	}
}

func TestJournalSearch_ViaJournalSearcher(t *testing.T) {
	mb := newMockBackend()
	js := &mockJournalSearcher{
		results: []backend.JournalResult{
			{
				Date: "2026-01-15",
				Page: "Jan 15th, 2026",
				Blocks: []types.BlockEntity{
					{UUID: "j1", Content: "Had a meeting about launch"},
				},
			},
		},
	}
	combined := &mockBackendWithJournalSearch{mockBackend: mb, mockJournalSearcher: js}

	j := NewJournal(combined)

	result, _, err := j.JournalSearch(context.Background(), nil, types.JournalSearchInput{
		Query: "launch",
	})
	if err != nil {
		t.Fatalf("JournalSearch() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("JournalSearch() returned error result")
	}

	var parsed map[string]any
	text := extractText(t, result)
	_ = json.Unmarshal([]byte(text), &parsed)

	if parsed["count"] != float64(1) {
		t.Errorf("count = %v, want 1", parsed["count"])
	}

	// Verify results array contains the matching block with date, page, and content.
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
	if !ok {
		t.Error("results[0].content missing or not a string")
	}
	if !strings.Contains(content, "launch") {
		t.Errorf("results[0].content = %q, want it to contain 'launch'", content)
	}
	page, ok := match["page"].(string)
	if !ok {
		t.Error("results[0].page missing or not a string")
	}
	if page != "Jan 15th, 2026" {
		t.Errorf("results[0].page = %q, want %q", page, "Jan 15th, 2026")
	}
	date, ok := match["date"].(string)
	if !ok {
		t.Error("results[0].date missing or not a string")
	}
	if date != "2026-01-15" {
		t.Errorf("results[0].date = %q, want %q", date, "2026-01-15")
	}
}

func TestJournalRange_EmptyRange(t *testing.T) {
	mb := newMockBackend()
	// No journal pages added — should return empty array, not error.
	j := NewJournal(mb)

	result, _, err := j.JournalRange(context.Background(), nil, types.JournalRangeInput{
		From: "2026-06-01",
		To:   "2026-06-03",
	})
	if err != nil {
		t.Fatalf("JournalRange() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("JournalRange() returned error for empty range")
	}

	var parsed map[string]any
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	entriesFound, ok := parsed["entriesFound"].(float64)
	if !ok {
		t.Fatalf("entriesFound missing or wrong type")
	}
	if entriesFound != 0 {
		t.Errorf("entriesFound = %v, want 0", entriesFound)
	}

	journals, ok := parsed["journals"].([]any)
	if !ok {
		// Null is also acceptable for empty results.
		if parsed["journals"] != nil {
			t.Fatalf("journals should be empty array or null, got %T", parsed["journals"])
		}
	} else if len(journals) != 0 {
		t.Errorf("journals length = %d, want 0", len(journals))
	}
}

func TestJournalSearch_QueryError(t *testing.T) {
	mb := newMockBackend()
	mb.queryErr = fmt.Errorf("query failed")
	j := NewJournal(mb)

	result, _, err := j.JournalSearch(context.Background(), nil, types.JournalSearchInput{
		Query: "anything",
	})
	if err != nil {
		t.Fatalf("JournalSearch() error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when query fails")
	}
}
