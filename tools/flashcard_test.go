package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/unbound-force/dewey/types"
)

func TestFlashcardOverview_Success(t *testing.T) {
	mb := newMockBackend()
	query := `[:find (pull ?b [:block/uuid :block/content :block/properties
	                           {:block/page [:block/name :block/original-name]}])
		:where
		[?b :block/refs ?ref]
		[?ref :block/name "card"]]`
	mb.queryResults[query] = json.RawMessage(`[
		[{"uuid":"card-1","content":"What is Go? #card","properties":{"card-next-schedule":"2020-01-01T00:00:00Z","card-repeats":3},"page":{"name":"go-notes"}}],
		[{"uuid":"card-2","content":"What is Rust? #card","properties":null,"page":{"name":"rust-notes"}}],
		[{"uuid":"card-3","content":"What is Python? #card","properties":{"card-next-schedule":"2099-01-01T00:00:00Z","card-repeats":1},"page":{"name":"python-notes"}}]
	]`)

	f := NewFlashcard(mb)

	result, _, err := f.FlashcardOverview(context.Background(), nil, types.FlashcardOverviewInput{})
	if err != nil {
		t.Fatalf("FlashcardOverview() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("FlashcardOverview() returned error result")
	}

	var parsed map[string]any
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if parsed["totalCards"] != float64(3) {
		t.Errorf("totalCards = %v, want 3", parsed["totalCards"])
	}
	// card-2 has no schedule → new card.
	if parsed["newCards"] != float64(1) {
		t.Errorf("newCards = %v, want 1", parsed["newCards"])
	}
	// card-1 is due (schedule in the past), card-3 is not due (schedule in 2099).
	if parsed["dueNow"] != float64(1) {
		t.Errorf("dueNow = %v, want 1", parsed["dueNow"])
	}
	// 2 cards have been reviewed (have card-next-schedule).
	if parsed["reviewedCards"] != float64(2) {
		t.Errorf("reviewedCards = %v, want 2", parsed["reviewedCards"])
	}
}

func TestFlashcardOverview_NoCards(t *testing.T) {
	mb := newMockBackend()
	query := `[:find (pull ?b [:block/uuid :block/content :block/properties
	                           {:block/page [:block/name :block/original-name]}])
		:where
		[?b :block/refs ?ref]
		[?ref :block/name "card"]]`
	mb.queryResults[query] = json.RawMessage(`[]`)

	f := NewFlashcard(mb)

	result, _, err := f.FlashcardOverview(context.Background(), nil, types.FlashcardOverviewInput{})
	if err != nil {
		t.Fatalf("FlashcardOverview() error: %v", err)
	}
	// Not an error, just a text message about no cards.
	if result.IsError {
		t.Fatal("no cards is not an error")
	}
}

func TestFlashcardOverview_QueryError(t *testing.T) {
	mb := newMockBackend()
	mb.queryErr = fmt.Errorf("query failed")
	f := NewFlashcard(mb)

	result, _, err := f.FlashcardOverview(context.Background(), nil, types.FlashcardOverviewInput{})
	if err != nil {
		t.Fatalf("FlashcardOverview() error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when query fails")
	}
}

func TestFlashcardDue_Success(t *testing.T) {
	mb := newMockBackend()
	query := `[:find (pull ?b [:block/uuid :block/content :block/properties
	                           {:block/page [:block/name :block/original-name]}])
		:where
		[?b :block/refs ?ref]
		[?ref :block/name "card"]]`
	mb.queryResults[query] = json.RawMessage(`[
		[{"uuid":"due-1","content":"Due card #card","properties":{"card-next-schedule":"2020-01-01T00:00:00Z"},"page":{"name":"notes"}}],
		[{"uuid":"not-due","content":"Future card #card","properties":{"card-next-schedule":"2099-01-01T00:00:00Z"},"page":{"name":"notes"}}],
		[{"uuid":"new-1","content":"New card #card","page":{"name":"notes"}}]
	]`)

	f := NewFlashcard(mb)

	result, _, err := f.FlashcardDue(context.Background(), nil, types.FlashcardDueInput{
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("FlashcardDue() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("FlashcardDue() returned error result")
	}

	var parsed map[string]any
	text := extractText(t, result)
	json.Unmarshal([]byte(text), &parsed)

	dueCount := parsed["dueCount"].(float64)
	if dueCount != 2 {
		t.Errorf("dueCount = %v, want 2 (1 overdue + 1 new)", dueCount)
	}
}

func TestFlashcardCreate_Success(t *testing.T) {
	mb := newMockBackend()
	mb.appendBlockResult = &types.BlockEntity{UUID: "front-uuid"}
	f := NewFlashcard(mb)

	result, _, err := f.FlashcardCreate(context.Background(), nil, types.FlashcardCreateInput{
		Page:  "study-page",
		Front: "What is Go?",
		Back:  "A systems programming language by Google",
	})
	if err != nil {
		t.Fatalf("FlashcardCreate() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("FlashcardCreate() returned error result")
	}

	// Verify front block was created with #card tag.
	if len(mb.appendedBlocks) != 1 {
		t.Fatalf("expected 1 appended block, got %d", len(mb.appendedBlocks))
	}
	if mb.appendedBlocks[0].content != "What is Go? #card" {
		t.Errorf("front content = %q, want %q", mb.appendedBlocks[0].content, "What is Go? #card")
	}

	// Verify answer was inserted as child.
	if len(mb.insertedBlocks) != 1 {
		t.Fatalf("expected 1 inserted block (answer), got %d", len(mb.insertedBlocks))
	}

	var parsed map[string]any
	text := extractText(t, result)
	json.Unmarshal([]byte(text), &parsed)

	if parsed["created"] != true {
		t.Errorf("created = %v, want true", parsed["created"])
	}
	if parsed["uuid"] != "front-uuid" {
		t.Errorf("uuid = %v, want %q", parsed["uuid"], "front-uuid")
	}
}

func TestFlashcardCreate_AppendError(t *testing.T) {
	mb := newMockBackend()
	mb.appendBlockErr = fmt.Errorf("write failed")
	f := NewFlashcard(mb)

	result, _, err := f.FlashcardCreate(context.Background(), nil, types.FlashcardCreateInput{
		Page:  "study-page",
		Front: "Will fail",
		Back:  "Should not reach here",
	})
	if err != nil {
		t.Fatalf("FlashcardCreate() error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when append fails")
	}
}
