package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/unbound-force/dewey/v3/store"
	"github.com/unbound-force/dewey/v3/types"
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

// insertPromotePage seeds a page row for promote tests, failing the test
// on insert errors. The source id mirrors how internal store pages are
// written (the first path segment of the name, e.g. "compiled" or
// "learning") and the doc id is the full page name — unique per row,
// which satisfies the UNIQUE(source_id, source_doc_id) constraint.
func insertPromotePage(t *testing.T, s *store.Store, name, tier string) {
	t.Helper()
	sourceID := name
	if i := strings.Index(name, "/"); i >= 0 {
		sourceID = name[:i]
	}
	if err := s.InsertPage(&store.Page{
		Name:         name,
		OriginalName: name,
		SourceID:     sourceID,
		SourceDocID:  name,
		Tier:         tier,
	}); err != nil {
		t.Fatalf("InsertPage(%q): %v", name, err)
	}
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
	if !strings.Contains(text, "promote requires persistent storage") {
		t.Errorf("error message = %q, should mention 'promote requires persistent storage'", text)
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

// --- Promote internal-store regression tests (mcp-internal-store-access task 3.3) ---

// TestPromote_PromotesInternalPagesByExactName verifies the #91 MCP path:
// promote locates internal store pages — both compiled/* and learning/* —
// by exact name and flips their tier from draft to validated, reporting
// the previous and new tier (spec: Promote Locates Internal Store Pages).
func TestPromote_PromotesInternalPagesByExactName(t *testing.T) {
	tests := []struct {
		name string
		page string
	}{
		{"compiled page", "compiled/gateway"},
		{"learning page", "learning/auth-20260101-alice"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore(t)
			p := NewPromote(s)
			insertPromotePage(t, s, tt.page, "draft")

			result, _, err := p.Promote(context.Background(), nil, types.PromoteInput{Page: tt.page})
			if err != nil {
				t.Fatalf("Promote error: %v", err)
			}
			if result.IsError {
				t.Fatalf("expected success, got error: %s", resultText(result))
			}

			parsed := parsePromoteResult(t, resultText(result))
			if parsed["page"] != tt.page {
				t.Errorf("page = %v, want %q", parsed["page"], tt.page)
			}
			if parsed["previous_tier"] != "draft" {
				t.Errorf("previous_tier = %v, want 'draft'", parsed["previous_tier"])
			}
			if parsed["new_tier"] != "validated" {
				t.Errorf("new_tier = %v, want 'validated'", parsed["new_tier"])
			}

			stored, err := s.GetPage(tt.page)
			if err != nil {
				t.Fatalf("GetPage: %v", err)
			}
			if stored == nil {
				t.Fatalf("page %q missing after promotion", tt.page)
			}
			if stored.Tier != "validated" {
				t.Errorf("stored tier = %q, want 'validated'", stored.Tier)
			}
		})
	}
}

// TestPromote_ShorthandResolution verifies bare-name resolution: when the
// exact name misses and the input contains no "/", promote tries
// "compiled/{name}" then "learning/{name}" and promotes the first existing
// candidate — compiled wins when both exist. When no candidate exists the
// not-found error names the original input (spec: Promote Name
// Resolution; issue #91 records agents calling promote with the bare tag).
func TestPromote_ShorthandResolution(t *testing.T) {
	tests := []struct {
		name     string
		seed     []string // draft pages to seed before the call
		input    string   // page name passed to promote
		wantPage string   // resolved page expected on success; empty means not found
	}{
		{
			name:     "bare name resolves to compiled page",
			seed:     []string{"compiled/gateway"},
			input:    "gateway",
			wantPage: "compiled/gateway",
		},
		{
			name:     "compiled prefix wins when both exist",
			seed:     []string{"compiled/gateway", "learning/gateway"},
			input:    "gateway",
			wantPage: "compiled/gateway",
		},
		{
			name:     "learning prefix resolves when only learning exists",
			seed:     []string{"learning/gateway"},
			input:    "gateway",
			wantPage: "learning/gateway",
		},
		{
			name:  "no candidate reports the original name as not found",
			seed:  []string{"compiled/unrelated"},
			input: "gateway",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore(t)
			p := NewPromote(s)
			for _, page := range tt.seed {
				insertPromotePage(t, s, page, "draft")
			}

			result, _, err := p.Promote(context.Background(), nil, types.PromoteInput{Page: tt.input})
			if err != nil {
				t.Fatalf("Promote error: %v", err)
			}

			if tt.wantPage == "" {
				if !result.IsError {
					t.Fatalf("expected error result, got success: %s", resultText(result))
				}
				text := resultText(result)
				if !strings.Contains(text, "not found") {
					t.Errorf("error message = %q, should mention 'not found'", text)
				}
				if !strings.Contains(text, "'"+tt.input+"'") {
					t.Errorf("error message = %q, should name the original input %q", text, tt.input)
				}
				return
			}

			if result.IsError {
				t.Fatalf("expected success, got error: %s", resultText(result))
			}
			parsed := parsePromoteResult(t, resultText(result))
			if parsed["page"] != tt.wantPage {
				t.Errorf("page = %v, want %q", parsed["page"], tt.wantPage)
			}

			// The resolved page is promoted; every other seeded page stays draft.
			for _, page := range tt.seed {
				stored, err := s.GetPage(page)
				if err != nil {
					t.Fatalf("GetPage(%q): %v", page, err)
				}
				if stored == nil {
					t.Fatalf("page %q missing", page)
				}
				wantTier := "draft"
				if page == tt.wantPage {
					wantTier = "validated"
				}
				if stored.Tier != wantTier {
					t.Errorf("page %q tier = %q, want %q", page, stored.Tier, wantTier)
				}
			}
		})
	}
}

// TestPromote_StoreErrorSurfaces verifies that a failing store lookup is
// reported as a lookup error and never masked as "not found" (spec:
// Promote Error Distinction — the #91 bug class). The underlying database
// is closed after creation so GetPage fails, mirroring the pattern in
// vault.TestGetPage_StoreErrorSurfaces.
func TestPromote_StoreErrorSurfaces(t *testing.T) {
	s := newTestStore(t)
	// Close the underlying database so every lookup fails. The t.Cleanup
	// close registered by newTestStore is a safe no-op — database/sql
	// Close is idempotent.
	if err := s.Close(); err != nil {
		t.Fatalf("Close store: %v", err)
	}
	p := NewPromote(s)

	result, _, err := p.Promote(context.Background(), nil, types.PromoteInput{
		Page: "compiled/gateway",
	})
	if err != nil {
		t.Fatalf("Promote error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when the store lookup fails")
	}

	text := resultText(result)
	if !strings.Contains(text, "failed to look up page") {
		t.Errorf("error message = %q, should mention 'failed to look up page'", text)
	}
	if !strings.Contains(text, "compiled/gateway") {
		t.Errorf("error message = %q, should name the queried page", text)
	}
	if strings.Contains(text, "not found") {
		t.Errorf("error message = %q, a store failure must not be masked as not-found", text)
	}
}

// TestPromote_NonDraftRejectionKeepsTier verifies the tier invariant from
// both directions: authored and validated pages are rejected with the
// tier-invariant error naming the resolved page, and the rejected attempt
// leaves the stored tier unchanged. The rejection also applies to pages
// located via the shorthand (spec: Promote Locates Internal Store Pages,
// non-draft scenario).
func TestPromote_NonDraftRejectionKeepsTier(t *testing.T) {
	tests := []struct {
		name      string
		seed      string // page to seed
		tier      string // tier to seed
		input     string // page name passed to promote
		wantInErr string // resolved page name the error must report
	}{
		{
			name:      "authored page",
			seed:      "specs/001-core",
			tier:      "authored",
			input:     "specs/001-core",
			wantInErr: "specs/001-core",
		},
		{
			name:      "validated page",
			seed:      "compiled/gateway",
			tier:      "validated",
			input:     "compiled/gateway",
			wantInErr: "compiled/gateway",
		},
		{
			name:      "shorthand-resolved authored page",
			seed:      "compiled/gateway",
			tier:      "authored",
			input:     "gateway",
			wantInErr: "compiled/gateway",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore(t)
			p := NewPromote(s)
			insertPromotePage(t, s, tt.seed, tt.tier)

			result, _, err := p.Promote(context.Background(), nil, types.PromoteInput{Page: tt.input})
			if err != nil {
				t.Fatalf("Promote error: %v", err)
			}
			if !result.IsError {
				t.Fatalf("expected error result for %q-tier page", tt.tier)
			}

			text := resultText(result)
			if !strings.Contains(text, "cannot be promoted") {
				t.Errorf("error message = %q, should mention 'cannot be promoted'", text)
			}
			if !strings.Contains(text, "'"+tt.tier+"'") {
				t.Errorf("error message = %q, should mention the current tier %q", text, tt.tier)
			}
			if !strings.Contains(text, tt.wantInErr) {
				t.Errorf("error message = %q, should name the resolved page %q", text, tt.wantInErr)
			}

			// The rejected attempt must leave the stored tier untouched.
			stored, err := s.GetPage(tt.seed)
			if err != nil {
				t.Fatalf("GetPage: %v", err)
			}
			if stored == nil {
				t.Fatalf("page %q missing after rejected promote", tt.seed)
			}
			if stored.Tier != tt.tier {
				t.Errorf("stored tier = %q, want unchanged %q", stored.Tier, tt.tier)
			}
		})
	}
}
