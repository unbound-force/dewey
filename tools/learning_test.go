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

// parseLearningResult unmarshals the JSON text from a CallToolResult into a map.
func parseLearningResult(t *testing.T, text string) map[string]any {
	t.Helper()
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	return parsed
}

// TestStoreLearning_Basic verifies the happy path: storing a learning with
// a valid information string and nil embedder returns a successful result
// containing a UUID, identity, and page name with the "learning/" prefix.
// Updated for 013-knowledge-compile: now uses default tag "general" when
// no tag is provided.
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
	parsed := parseLearningResult(t, resultText(result))

	// Assert UUID is present and non-empty.
	uuid, ok := parsed["uuid"].(string)
	if !ok || uuid == "" {
		t.Errorf("expected non-empty uuid in result, got %v", parsed["uuid"])
	}

	// Assert identity follows {tag}-{sequence} format with default tag "general".
	identity, ok := parsed["identity"].(string)
	if !ok || identity != "general-1" {
		t.Errorf("expected identity %q, got %q", "general-1", identity)
	}

	// Assert page name has "learning/" prefix and matches identity.
	page, ok := parsed["page"].(string)
	if !ok || page != "learning/general-1" {
		t.Errorf("expected page %q, got %q", "learning/general-1", page)
	}

	// Assert tag defaults to "general".
	tag, ok := parsed["tag"].(string)
	if !ok || tag != "general" {
		t.Errorf("expected tag %q, got %q", "general", tag)
	}

	// Assert message indicates success.
	msg, ok := parsed["message"].(string)
	if !ok || !strings.Contains(msg, "stored successfully") {
		t.Errorf("expected success message, got %q", msg)
	}

	// Assert created_at is present and non-empty.
	createdAt, ok := parsed["created_at"].(string)
	if !ok || createdAt == "" {
		t.Errorf("expected non-empty created_at, got %v", parsed["created_at"])
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

// TestStoreLearning_WithTag verifies that the tag parameter produces a
// {tag}-{sequence} identity and stores the tag in page properties.
func TestStoreLearning_WithTag(t *testing.T) {
	s := newTestStore(t)
	l := NewLearning(nil, s)

	result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "OAuth tokens should be rotated every 24 hours",
		Tag:         "authentication",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if result.IsError {
		t.Fatalf("StoreLearning returned error result: %s", resultText(result))
	}

	parsed := parseLearningResult(t, resultText(result))

	// Assert identity is {tag}-1 for the first learning with this tag.
	identity, ok := parsed["identity"].(string)
	if !ok || identity != "authentication-1" {
		t.Errorf("expected identity %q, got %q", "authentication-1", identity)
	}

	// Assert page name matches.
	page, ok := parsed["page"].(string)
	if !ok || page != "learning/authentication-1" {
		t.Errorf("expected page %q, got %q", "learning/authentication-1", page)
	}

	// Assert tag is returned.
	tag, ok := parsed["tag"].(string)
	if !ok || tag != "authentication" {
		t.Errorf("expected tag %q, got %q", "authentication", tag)
	}

	// Verify the page in the store has the correct properties.
	storedPage, err := s.GetPage("learning/authentication-1")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if storedPage == nil {
		t.Fatal("page not found in store")
	}

	var props map[string]string
	if err := json.Unmarshal([]byte(storedPage.Properties), &props); err != nil {
		t.Fatalf("unmarshal properties: %v", err)
	}
	if props["tag"] != "authentication" {
		t.Errorf("stored tag = %q, want %q", props["tag"], "authentication")
	}
	if props["created_at"] == "" {
		t.Error("expected non-empty created_at in properties")
	}

	// Verify tier is "draft".
	if storedPage.Tier != "draft" {
		t.Errorf("tier = %q, want %q", storedPage.Tier, "draft")
	}
}

// TestStoreLearning_EmptyTag verifies that when both tag and tags are empty,
// the default tag "general" is used (not an error).
func TestStoreLearning_EmptyTag(t *testing.T) {
	s := newTestStore(t)
	l := NewLearning(nil, s)

	result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "a learning without any tag",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success with default tag, got error: %s", resultText(result))
	}

	parsed := parseLearningResult(t, resultText(result))

	// Should default to "general" tag.
	tag, ok := parsed["tag"].(string)
	if !ok || tag != "general" {
		t.Errorf("expected default tag %q, got %q", "general", tag)
	}

	identity, ok := parsed["identity"].(string)
	if !ok || identity != "general-1" {
		t.Errorf("expected identity %q, got %q", "general-1", identity)
	}
}

// TestStoreLearning_BackwardCompat verifies that the deprecated Tags field
// (comma-separated) falls back to the first tag when Tag is empty.
func TestStoreLearning_BackwardCompat(t *testing.T) {
	s := newTestStore(t)
	l := NewLearning(nil, s)

	result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "test learning with old tags field",
		Tags:        "gotcha, vault-walker, 006-unified-ignore",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if result.IsError {
		t.Fatalf("StoreLearning returned error result: %s", resultText(result))
	}

	parsed := parseLearningResult(t, resultText(result))

	// Should use first tag from comma-separated list.
	tag, ok := parsed["tag"].(string)
	if !ok || tag != "gotcha" {
		t.Errorf("expected tag %q from backward-compat fallback, got %q", "gotcha", tag)
	}

	identity, ok := parsed["identity"].(string)
	if !ok || identity != "gotcha-1" {
		t.Errorf("expected identity %q, got %q", "gotcha-1", identity)
	}

	// Verify the page in the store preserves the original tags in properties.
	pageName, _ := parsed["page"].(string)
	storedPage, err := s.GetPage(pageName)
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if storedPage == nil {
		t.Fatal("page not found in store")
	}

	var props map[string]string
	if err := json.Unmarshal([]byte(storedPage.Properties), &props); err != nil {
		t.Fatalf("unmarshal properties: %v", err)
	}
	if props["tags"] != "gotcha, vault-walker, 006-unified-ignore" {
		t.Errorf("tags = %q, want %q", props["tags"], "gotcha, vault-walker, 006-unified-ignore")
	}
}

// TestStoreLearning_TagPriorityOverTags verifies that when both Tag and Tags
// are provided, Tag takes priority.
func TestStoreLearning_TagPriorityOverTags(t *testing.T) {
	s := newTestStore(t)
	l := NewLearning(nil, s)

	result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "test learning with both fields",
		Tag:         "deployment",
		Tags:        "gotcha, vault-walker",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if result.IsError {
		t.Fatalf("StoreLearning returned error result: %s", resultText(result))
	}

	parsed := parseLearningResult(t, resultText(result))

	// Tag field should take priority over Tags.
	tag, ok := parsed["tag"].(string)
	if !ok || tag != "deployment" {
		t.Errorf("expected tag %q (Tag takes priority), got %q", "deployment", tag)
	}
}

// TestStoreLearning_WithCategory verifies that a valid category is stored
// correctly on the page and returned in the response.
func TestStoreLearning_WithCategory(t *testing.T) {
	s := newTestStore(t)
	l := NewLearning(nil, s)

	result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "Always rotate OAuth tokens after 24 hours",
		Tag:         "authentication",
		Category:    "decision",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if result.IsError {
		t.Fatalf("StoreLearning returned error result: %s", resultText(result))
	}

	parsed := parseLearningResult(t, resultText(result))

	// Assert category is returned in the response.
	category, ok := parsed["category"].(string)
	if !ok || category != "decision" {
		t.Errorf("expected category %q, got %q", "decision", category)
	}

	// Verify the page in the store has the correct category.
	pageName, _ := parsed["page"].(string)
	storedPage, err := s.GetPage(pageName)
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if storedPage == nil {
		t.Fatal("page not found in store")
	}
	if storedPage.Category != "decision" {
		t.Errorf("stored category = %q, want %q", storedPage.Category, "decision")
	}

	// Verify category is also in properties JSON.
	var props map[string]string
	if err := json.Unmarshal([]byte(storedPage.Properties), &props); err != nil {
		t.Fatalf("unmarshal properties: %v", err)
	}
	if props["category"] != "decision" {
		t.Errorf("properties category = %q, want %q", props["category"], "decision")
	}
}

// TestStoreLearning_InvalidCategory verifies that an invalid category
// returns an MCP error result with a descriptive message.
func TestStoreLearning_InvalidCategory(t *testing.T) {
	s := newTestStore(t)
	l := NewLearning(nil, s)

	result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "test learning",
		Tag:         "test",
		Category:    "invalid-category",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for invalid category")
	}

	text := resultText(result)
	if !strings.Contains(text, "invalid category") {
		t.Errorf("error message = %q, should mention 'invalid category'", text)
	}
	if !strings.Contains(text, "invalid-category") {
		t.Errorf("error message = %q, should include the invalid value", text)
	}
}

// TestStoreLearning_EmptyCategory verifies that an empty category is
// allowed and stored as an empty string.
func TestStoreLearning_EmptyCategory(t *testing.T) {
	s := newTestStore(t)
	l := NewLearning(nil, s)

	result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "test learning without category",
		Tag:         "test",
		Category:    "",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success with empty category, got error: %s", resultText(result))
	}

	parsed := parseLearningResult(t, resultText(result))

	// Empty category should be returned as empty string.
	category, ok := parsed["category"].(string)
	if !ok || category != "" {
		t.Errorf("expected empty category, got %q", category)
	}
}

// TestStoreLearning_AllValidCategories verifies that all valid category
// values are accepted.
func TestStoreLearning_AllValidCategories(t *testing.T) {
	categories := []string{"decision", "pattern", "gotcha", "context", "reference"}

	for _, cat := range categories {
		t.Run(cat, func(t *testing.T) {
			s := newTestStore(t)
			l := NewLearning(nil, s)

			result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
				Information: "test learning for " + cat,
				Tag:         "test",
				Category:    cat,
			})
			if err != nil {
				t.Fatalf("StoreLearning error: %v", err)
			}
			if result.IsError {
				t.Fatalf("expected success for category %q, got error: %s", cat, resultText(result))
			}

			parsed := parseLearningResult(t, resultText(result))
			if parsed["category"] != cat {
				t.Errorf("expected category %q, got %v", cat, parsed["category"])
			}
		})
	}
}

// TestStoreLearning_SequenceIncrement verifies that storing multiple
// learnings with the same tag produces monotonically increasing sequence
// numbers: tag-1, tag-2, tag-3.
func TestStoreLearning_SequenceIncrement(t *testing.T) {
	s := newTestStore(t)
	l := NewLearning(nil, s)

	expectedIdentities := []string{"deployment-1", "deployment-2", "deployment-3"}

	for i, expected := range expectedIdentities {
		result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
			Information: strings.Repeat("learning content ", i+1), // unique content
			Tag:         "deployment",
		})
		if err != nil {
			t.Fatalf("StoreLearning[%d] error: %v", i, err)
		}
		if result.IsError {
			t.Fatalf("StoreLearning[%d] returned error: %s", i, resultText(result))
		}

		parsed := parseLearningResult(t, resultText(result))
		identity, ok := parsed["identity"].(string)
		if !ok || identity != expected {
			t.Errorf("learning[%d] identity = %q, want %q", i, identity, expected)
		}

		page, ok := parsed["page"].(string)
		if !ok || page != "learning/"+expected {
			t.Errorf("learning[%d] page = %q, want %q", i, page, "learning/"+expected)
		}
	}

	// Verify all 3 pages exist in the store.
	pages, err := s.ListPagesBySource("learning")
	if err != nil {
		t.Fatalf("ListPagesBySource: %v", err)
	}
	if len(pages) != 3 {
		t.Errorf("expected 3 learning pages, got %d", len(pages))
	}
}

// TestStoreLearning_TagNormalization verifies that tags are normalized:
// lowercase, spaces replaced with hyphens, non-alphanumeric stripped.
func TestStoreLearning_TagNormalization(t *testing.T) {
	tests := []struct {
		name     string
		inputTag string
		wantTag  string
	}{
		{"uppercase", "Authentication", "authentication"},
		{"spaces", "My Tag Name", "my-tag-name"},
		{"leading trailing spaces", "  auth  ", "auth"},
		{"special chars", "auth@#$%enti!cation", "authentication"},
		{"mixed", "  My Tag!  ", "my-tag"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore(t)
			l := NewLearning(nil, s)

			result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
				Information: "test learning",
				Tag:         tt.inputTag,
			})
			if err != nil {
				t.Fatalf("StoreLearning error: %v", err)
			}
			if result.IsError {
				t.Fatalf("StoreLearning returned error: %s", resultText(result))
			}

			parsed := parseLearningResult(t, resultText(result))
			tag, ok := parsed["tag"].(string)
			if !ok || tag != tt.wantTag {
				t.Errorf("tag = %q, want %q", tag, tt.wantTag)
			}
		})
	}
}

// TestStoreLearning_TierDraft verifies that all stored learnings have
// tier "draft" regardless of input.
func TestStoreLearning_TierDraft(t *testing.T) {
	s := newTestStore(t)
	l := NewLearning(nil, s)

	result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "test learning for tier check",
		Tag:         "test",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if result.IsError {
		t.Fatalf("StoreLearning returned error: %s", resultText(result))
	}

	parsed := parseLearningResult(t, resultText(result))
	pageName, _ := parsed["page"].(string)

	storedPage, err := s.GetPage(pageName)
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if storedPage == nil {
		t.Fatal("page not found in store")
	}
	if storedPage.Tier != "draft" {
		t.Errorf("tier = %q, want %q", storedPage.Tier, "draft")
	}
}

// TestStoreLearning_CreatedAtInResponse verifies that the response includes
// a non-empty created_at field in ISO 8601 format.
func TestStoreLearning_CreatedAtInResponse(t *testing.T) {
	s := newTestStore(t)
	l := NewLearning(nil, s)

	result, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "test learning for created_at",
		Tag:         "test",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if result.IsError {
		t.Fatalf("StoreLearning returned error: %s", resultText(result))
	}

	parsed := parseLearningResult(t, resultText(result))
	createdAt, ok := parsed["created_at"].(string)
	if !ok || createdAt == "" {
		t.Errorf("expected non-empty created_at, got %v", parsed["created_at"])
	}

	// Verify it contains a 'T' (ISO 8601 separator) and 'Z' (UTC).
	if !strings.Contains(createdAt, "T") || !strings.Contains(createdAt, "Z") {
		t.Errorf("created_at = %q, expected ISO 8601 format with T and Z", createdAt)
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
		Tag:         "test",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	// Learning should still be stored — not an error result.
	if result.IsError {
		t.Fatalf("expected successful result even when embedder unavailable, got error: %s", resultText(result))
	}

	// Message should mention that embeddings were not generated.
	parsed := parseLearningResult(t, resultText(result))
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
		Tag:         "test",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	// Learning should still be stored — not an error result.
	if result.IsError {
		t.Fatalf("expected successful result even when embedder is nil, got error: %s", resultText(result))
	}

	// Message should mention that embeddings were not generated.
	parsed := parseLearningResult(t, resultText(result))
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
		Tag:         "search-test",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if result.IsError {
		t.Fatalf("StoreLearning returned error: %s", resultText(result))
	}

	// Extract the page name from the result.
	parsed := parseLearningResult(t, resultText(result))
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
		Tag:         "filter-test",
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

// TestStoreLearning_DifferentTagSequences verifies that sequence numbers
// are independent per tag namespace — two different tags each start at 1.
func TestStoreLearning_DifferentTagSequences(t *testing.T) {
	s := newTestStore(t)
	l := NewLearning(nil, s)

	// Store learning with tag "auth".
	result1, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "auth learning 1",
		Tag:         "auth",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if result1.IsError {
		t.Fatalf("StoreLearning returned error: %s", resultText(result1))
	}

	// Store learning with tag "deploy".
	result2, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "deploy learning 1",
		Tag:         "deploy",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if result2.IsError {
		t.Fatalf("StoreLearning returned error: %s", resultText(result2))
	}

	// Store another learning with tag "auth".
	result3, _, err := l.StoreLearning(context.Background(), nil, types.StoreLearningInput{
		Information: "auth learning 2",
		Tag:         "auth",
	})
	if err != nil {
		t.Fatalf("StoreLearning error: %v", err)
	}
	if result3.IsError {
		t.Fatalf("StoreLearning returned error: %s", resultText(result3))
	}

	parsed1 := parseLearningResult(t, resultText(result1))
	parsed2 := parseLearningResult(t, resultText(result2))
	parsed3 := parseLearningResult(t, resultText(result3))

	if parsed1["identity"] != "auth-1" {
		t.Errorf("first auth identity = %v, want %q", parsed1["identity"], "auth-1")
	}
	if parsed2["identity"] != "deploy-1" {
		t.Errorf("deploy identity = %v, want %q", parsed2["identity"], "deploy-1")
	}
	if parsed3["identity"] != "auth-2" {
		t.Errorf("second auth identity = %v, want %q", parsed3["identity"], "auth-2")
	}
}
