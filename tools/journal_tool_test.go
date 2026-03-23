package tools

import (
	"context"
	"encoding/json"
	"fmt"
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
	json.Unmarshal([]byte(text), &parsed)

	if parsed["query"] != "meeting" {
		t.Errorf("query = %v, want %q", parsed["query"], "meeting")
	}
	if parsed["count"] != float64(1) {
		t.Errorf("count = %v, want 1 (only one block mentions meeting)", parsed["count"])
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
	json.Unmarshal([]byte(text), &parsed)

	if parsed["count"] != float64(1) {
		t.Errorf("count = %v, want 1", parsed["count"])
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
