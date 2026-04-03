package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/unbound-force/dewey/store"
	"github.com/unbound-force/dewey/types"
)

// newTestStore creates an in-memory store for learning tests.
// Registers a cleanup function to close the store when the test completes.
func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// TestStoreLearning_Basic verifies the happy path: storing a learning with
// a valid information string and nil embedder returns a successful result
// containing a UUID and page name with the "learning/" prefix.
func TestStoreLearning_Basic(t *testing.T) {
	s := newTestStore(t)
	l := NewLearning(nil, s)

	result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "The vault walker must build its ignore matcher in New()",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.IsError {
		t.Fatalf("StoreLearning returned error result: %s", resultText(result))
	}

	// Parse the JSON result to verify structure.
	text := resultText(result)
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	// Assert UUID is present and non-empty.
	uuid, ok := parsed["uuid"].(string)
	if !ok || uuid == "" {
		t.Errorf("expected non-empty uuid in result, got %v", parsed["uuid"])
	}

	// Assert page name has "learning/" prefix.
	page, ok := parsed["page"].(string)
	if !ok || !strings.HasPrefix(page, "learning/") {
		t.Errorf("expected page with 'learning/' prefix, got %q", page)
	}

	// Assert message indicates success.
	msg, ok := parsed["message"].(string)
	if !ok || !strings.Contains(msg, "stored successfully") {
		t.Errorf("expected success message, got %q", msg)
	}
}

// TestStoreLearning_EmptyInformation verifies that an empty information
// string returns an error result mentioning "information".
func TestStoreLearning_EmptyInformation(t *testing.T) {
	s := newTestStore(t)
	l := NewLearning(nil, s)

	result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for empty information")
	}

	text := resultText(result)
	if !strings.Contains(text, "information") {
		t.Errorf("error message = %q, should mention 'information'", text)
	}
}

// TestStoreLearning_NilStore verifies that a nil store returns an error
// result mentioning persistent storage.
func TestStoreLearning_NilStore(t *testing.T) {
	l := NewLearning(nil, nil)

	result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "test learning",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when store is nil")
	}

	text := resultText(result)
	if !strings.Contains(text, "persistent storage") {
		t.Errorf("error message = %q, should mention 'persistent storage'", text)
	}
}

// TestStoreLearning_WithTags verifies that tags are stored as page
// properties when provided.
func TestStoreLearning_WithTags(t *testing.T) {
	s := newTestStore(t)
	l := NewLearning(nil, s)

	result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "test learning with tags",
		Tags:        "gotcha, vault-walker",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if result.IsError {
		t.Fatalf("StoreLearning returned error result: %s", resultText(result))
	}

	// Extract the page name from the result to look it up in the store.
	text := resultText(result)
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	pageName, ok := parsed["page"].(string)
	if !ok || pageName == "" {
		t.Fatalf("expected non-empty page name in result, got %v", parsed["page"])
	}

	// Query the store directly to verify properties contain tags.
	page, err := s.GetPage(pageName)
	if err != nil {
		t.Fatalf("GetPage(%q): %v", pageName, err)
	}
	if page == nil {
		t.Fatalf("page %q not found in store", pageName)
	}

	// Verify the properties JSON contains the tags.
	var props map[string]string
	if err := json.Unmarshal([]byte(page.Properties), &props); err != nil {
		t.Fatalf("unmarshal properties: %v", err)
	}
	if props["tags"] != "gotcha, vault-walker" {
		t.Errorf("tags = %q, want %q", props["tags"], "gotcha, vault-walker")
	}
}

// TestStoreLearning_EmbedderUnavailable verifies that when the embedder
// reports Available() == false, the learning is still stored successfully
// and the message mentions embeddings.
func TestStoreLearning_EmbedderUnavailable(t *testing.T) {
	s := newTestStore(t)
	e := newMockEmbedder(false) // Available() returns false
	l := NewLearning(e, s)

	result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "learning with unavailable embedder",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	// Learning should still be stored — not an error result.
	if result.IsError {
		t.Fatalf("expected successful result even when embedder unavailable, got error: %s", resultText(result))
	}

	// Message should mention that embeddings were not generated.
	text := resultText(result)
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	msg, _ := parsed["message"].(string)
	if !strings.Contains(msg, "Embeddings") && !strings.Contains(msg, "embeddings") {
		t.Errorf("message = %q, should mention embeddings", msg)
	}
}

// TestStoreLearning_NilEmbedder verifies that when the embedder is nil,
// the learning is still stored successfully and the message mentions
// embeddings not being generated.
func TestStoreLearning_NilEmbedder(t *testing.T) {
	s := newTestStore(t)
	l := NewLearning(nil, s) // nil embedder

	result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "learning with nil embedder",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	// Learning should still be stored — not an error result.
	if result.IsError {
		t.Fatalf("expected successful result even when embedder is nil, got error: %s", resultText(result))
	}

	// Message should mention that embeddings were not generated.
	text := resultText(result)
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	msg, _ := parsed["message"].(string)
	if !strings.Contains(msg, "Embeddings") && !strings.Contains(msg, "embeddings") {
		t.Errorf("message = %q, should mention embeddings", msg)
	}
}

// TestStoreLearning_Searchable verifies that a stored learning creates a
// page in the store that can be found by listing pages, with source_id
// set to "learning".
func TestStoreLearning_Searchable(t *testing.T) {
	s := newTestStore(t)
	l := NewLearning(nil, s)

	result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "searchable learning content",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if result.IsError {
		t.Fatalf("StoreLearning returned error: %s", resultText(result))
	}

	// Extract the page name from the result.
	text := resultText(result)
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	pageName, _ := parsed["page"].(string)

	// Verify the page exists in the store.
	pages, err := s.ListPages()
	if err != nil {
		t.Fatalf("ListPages: %v", err)
	}

	var found bool
	for _, p := range pages {
		if p.Name == pageName {
			found = true
			if p.SourceID != "learning" {
				t.Errorf("page %q source_id = %q, want %q", pageName, p.SourceID, "learning")
			}
			break
		}
	}
	if !found {
		t.Errorf("page %q not found in store after StoreLearning", pageName)
	}

	// Verify blocks were persisted for the page.
	blocks, err := s.GetBlocksByPage(pageName)
	if err != nil {
		t.Fatalf("GetBlocksByPage(%q): %v", pageName, err)
	}
	if len(blocks) == 0 {
		t.Errorf("expected at least 1 block for page %q, got 0", pageName)
	}
}

// TestStoreLearning_FilterBySourceType verifies that the stored learning
// page has source_id = "learning", which enables filtering via
// dewey_semantic_search_filtered. This proves the learning is distinguishable
// from other content sources.
func TestStoreLearning_FilterBySourceType(t *testing.T) {
	s := newTestStore(t)
	l := NewLearning(nil, s)

	// Store a learning.
	result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "filterable learning",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if result.IsError {
		t.Fatalf("StoreLearning returned error: %s", resultText(result))
	}

	// Also insert a non-learning page to verify filtering.
	err = s.InsertPage(&store.Page{
		Name:         "regular-page",
		OriginalName: "regular-page",
		SourceID:     "disk-local",
		SourceDocID:  "regular.md",
		ContentHash:  "abc",
	})
	if err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	// List pages by source "learning" — only the learning page should appear.
	learningPages, err := s.ListPagesBySource("learning")
	if err != nil {
		t.Fatalf("ListPagesBySource: %v", err)
	}
	if len(learningPages) != 1 {
		t.Fatalf("expected 1 learning page, got %d", len(learningPages))
	}
	if learningPages[0].SourceID != "learning" {
		t.Errorf("source_id = %q, want %q", learningPages[0].SourceID, "learning")
	}

	// Verify the regular page is NOT in the learning source.
	diskPages, err := s.ListPagesBySource("disk-local")
	if err != nil {
		t.Fatalf("ListPagesBySource(disk-local): %v", err)
	}
	if len(diskPages) != 1 {
		t.Fatalf("expected 1 disk-local page, got %d", len(diskPages))
	}
	if diskPages[0].Name != "regular-page" {
		t.Errorf("disk page name = %q, want %q", diskPages[0].Name, "regular-page")
	}
}
