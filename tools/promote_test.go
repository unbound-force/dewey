package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/unbound-force/dewey/store"
	"github.com/unbound-force/dewey/types"
)

// parsePromoteResult unmarshals the JSON text from a promote CallToolResult.
func parsePromoteResult(t *testing.T, text string) map[string]any {
	t.Helper()
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal promote result: %v\ntext: %s", err, text)
	}
	return parsed
}

// TestPromote_NilStore verifies that a nil store returns an error result
// mentioning persistent storage.
func TestPromote_NilStore(t *testing.T) {
	p := NewPromote(nil)

	result, _, err := p.Promote(context.Background(), nil, types.PromoteInput{
		Page: "learning/auth-1",
	})
	if err != nil {
		t.Fatalf("Promote error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when store is nil")
	}
	text := resultText(result)
	if !strings.Contains(text, "persistent storage") {
		t.Errorf("error message = %q, should mention 'persistent storage'", text)
	}
}

// TestPromote_Success verifies the happy path: a draft page is promoted
// to validated tier.
func TestPromote_Success(t *testing.T) {
	s := newTestStore(t)
	p := NewPromote(s)

	// Insert a draft learning page.
	page := &store.Page{
		Name:         "learning/authentication-3",
		OriginalName: "learning/authentication-3",
		SourceID:     "learning",
		SourceDocID:  "learning-authentication-3",
		Tier:         "draft",
		Category:     "decision",
	}
	if err := s.InsertPage(page); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	result, _, err := p.Promote(context.Background(), nil, types.PromoteInput{
		Page: "learning/authentication-3",
	})
	if err != nil {
		t.Fatalf("Promote error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", resultText(result))
	}

	parsed := parsePromoteResult(t, resultText(result))

	// Verify response fields.
	if parsed["page"] != "learning/authentication-3" {
		t.Errorf("page = %v, want 'learning/authentication-3'", parsed["page"])
	}
	if parsed["previous_tier"] != "draft" {
		t.Errorf("previous_tier = %v, want 'draft'", parsed["previous_tier"])
	}
	if parsed["new_tier"] != "validated" {
		t.Errorf("new_tier = %v, want 'validated'", parsed["new_tier"])
	}
	msg, _ := parsed["message"].(string)
	if !strings.Contains(msg, "promoted to validated tier") {
		t.Errorf("message = %q, should mention 'promoted to validated tier'", msg)
	}

	// Verify the page in the store has been updated.
	storedPage, err := s.GetPage("learning/authentication-3")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if storedPage == nil {
		t.Fatal("page not found after promotion")
	}
	if storedPage.Tier != "validated" {
		t.Errorf("stored tier = %q, want 'validated'", storedPage.Tier)
	}
}

// TestPromote_AlreadyValidated verifies that promoting an already-validated
// page returns an error.
func TestPromote_AlreadyValidated(t *testing.T) {
	s := newTestStore(t)
	p := NewPromote(s)

	// Insert a validated page.
	page := &store.Page{
		Name:         "learning/auth-1",
		OriginalName: "learning/auth-1",
		SourceID:     "learning",
		SourceDocID:  "learning-auth-1",
		Tier:         "validated",
	}
	if err := s.InsertPage(page); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	result, _, err := p.Promote(context.Background(), nil, types.PromoteInput{
		Page: "learning/auth-1",
	})
	if err != nil {
		t.Fatalf("Promote error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for already-validated page")
	}

	text := resultText(result)
	if !strings.Contains(text, "validated") {
		t.Errorf("error message = %q, should mention 'validated'", text)
	}
	if !strings.Contains(text, "Only 'draft' pages") {
		t.Errorf("error message = %q, should mention 'Only draft pages'", text)
	}
}

// TestPromote_AuthoredPage verifies that promoting an authored page
// returns an error (invariant 2: authored pages must not be promotable).
func TestPromote_AuthoredPage(t *testing.T) {
	s := newTestStore(t)
	p := NewPromote(s)

	// Insert an authored page (default tier).
	page := &store.Page{
		Name:         "specs/001-core",
		OriginalName: "specs/001-core",
		SourceID:     "disk-local",
		SourceDocID:  "specs-001-core",
		Tier:         "authored",
	}
	if err := s.InsertPage(page); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	result, _, err := p.Promote(context.Background(), nil, types.PromoteInput{
		Page: "specs/001-core",
	})
	if err != nil {
		t.Fatalf("Promote error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for authored page")
	}

	text := resultText(result)
	if !strings.Contains(text, "authored") {
		t.Errorf("error message = %q, should mention 'authored'", text)
	}
	if !strings.Contains(text, "cannot be promoted") {
		t.Errorf("error message = %q, should mention 'cannot be promoted'", text)
	}
}

// TestPromote_PageNotFound verifies that promoting a non-existent page
// returns an error.
func TestPromote_PageNotFound(t *testing.T) {
	s := newTestStore(t)
	p := NewPromote(s)

	result, _, err := p.Promote(context.Background(), nil, types.PromoteInput{
		Page: "learning/nonexistent-1",
	})
	if err != nil {
		t.Fatalf("Promote error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for non-existent page")
	}

	text := resultText(result)
	if !strings.Contains(text, "not found") {
		t.Errorf("error message = %q, should mention 'not found'", text)
	}
	if !strings.Contains(text, "nonexistent-1") {
		t.Errorf("error message = %q, should include the page name", text)
	}
}

// TestPromote_EmptyPageName verifies that an empty page name returns
// an error.
func TestPromote_EmptyPageName(t *testing.T) {
	s := newTestStore(t)
	p := NewPromote(s)

	result, _, err := p.Promote(context.Background(), nil, types.PromoteInput{
		Page: "",
	})
	if err != nil {
		t.Fatalf("Promote error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for empty page name")
	}

	text := resultText(result)
	if !strings.Contains(text, "required") {
		t.Errorf("error message = %q, should mention 'required'", text)
	}
}

// TestPromote_UpdatedAtRefreshed verifies that the updated_at timestamp
// is refreshed when a page is promoted (invariant 4).
func TestPromote_UpdatedAtRefreshed(t *testing.T) {
	s := newTestStore(t)
	p := NewPromote(s)

	// Insert a draft page with an old timestamp.
	page := &store.Page{
		Name:         "learning/old-page-1",
		OriginalName: "learning/old-page-1",
		SourceID:     "learning",
		SourceDocID:  "learning-old-page-1",
		Tier:         "draft",
		CreatedAt:    1000000, // very old
		UpdatedAt:    1000000, // very old
	}
	if err := s.InsertPage(page); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	// Record the old updated_at.
	beforePage, _ := s.GetPage("learning/old-page-1")
	oldUpdatedAt := beforePage.UpdatedAt

	// Promote.
	result, _, err := p.Promote(context.Background(), nil, types.PromoteInput{
		Page: "learning/old-page-1",
	})
	if err != nil {
		t.Fatalf("Promote error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", resultText(result))
	}

	// Verify updated_at was refreshed.
	afterPage, err := s.GetPage("learning/old-page-1")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if afterPage.UpdatedAt <= oldUpdatedAt {
		t.Errorf("updated_at was not refreshed: before=%d, after=%d", oldUpdatedAt, afterPage.UpdatedAt)
	}
}

// TestPromote_CompiledPage verifies that compiled articles (tier=draft)
// can also be promoted — promote works on both learning and compiled pages
// (invariant 6).
func TestPromote_CompiledPage(t *testing.T) {
	s := newTestStore(t)
	p := NewPromote(s)

	// Insert a compiled article with tier=draft.
	page := &store.Page{
		Name:         "compiled/authentication",
		OriginalName: "Authentication",
		SourceID:     "compiled",
		SourceDocID:  "authentication",
		Tier:         "draft",
	}
	if err := s.InsertPage(page); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	result, _, err := p.Promote(context.Background(), nil, types.PromoteInput{
		Page: "compiled/authentication",
	})
	if err != nil {
		t.Fatalf("Promote error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", resultText(result))
	}

	// Verify the compiled page was promoted.
	storedPage, err := s.GetPage("compiled/authentication")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if storedPage.Tier != "validated" {
		t.Errorf("tier = %q, want 'validated'", storedPage.Tier)
	}
}
