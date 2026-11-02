package tools

import (
	"fmt"
	"strings"
	"testing"
)

// --- Promote page-name resolution (dewey-distill, PR 145 items 10+14) ---
//
// resolvePromotePage moved the page-name ladder out of store.FindPage into
// the tools package, where each tier issues one narrow, index-backed store
// query (store.ListPagesByNameFoldCase, store.ListKnowledgePagesFoldCase,
// store.GetPageBySourceDoc). These tests migrated from store_test.go's
// TestFindPage_* suite keep the ladder's resolution semantics pinned:
//
//	tier 1: exact name (store.GetPage)
//	tier 2: case-insensitive name match
//	tier 3: bare tags — compiled/{name}, learning/{name}, knowledge/**/{name}
//	tier 4: source doc IDs — {source}/{doc}, or bare {doc} in compiled/learning

// TestResolvePromotePage_Exact verifies tier 1: the exact page name wins.
func TestResolvePromotePage_Exact(t *testing.T) {
	s := newTestStore(t)
	insertPromotePage(t, s, "compiled/gateway", "compiled", "gateway", "draft")

	page, err := resolvePromotePage(s, "compiled/gateway")
	if err != nil {
		t.Fatalf("resolvePromotePage: %v", err)
	}
	if page == nil {
		t.Fatal("expected page, got nil")
	}
	if page.Name != "compiled/gateway" {
		t.Errorf("page.Name = %q, want %q", page.Name, "compiled/gateway")
	}
}

// TestResolvePromotePage_CaseInsensitive verifies tier 2: a case-insensitive
// name match resolves when the exact name misses.
func TestResolvePromotePage_CaseInsensitive(t *testing.T) {
	s := newTestStore(t)
	insertPromotePage(t, s, "compiled/Gateway", "compiled", "Gateway", "draft")

	page, err := resolvePromotePage(s, "compiled/gateway")
	if err != nil {
		t.Fatalf("resolvePromotePage: %v", err)
	}
	if page == nil {
		t.Fatal("expected page, got nil")
	}
	if page.Name != "compiled/Gateway" {
		t.Errorf("page.Name = %q, want %q", page.Name, "compiled/Gateway")
	}
}

// TestResolvePromotePage_CaseAmbiguous verifies that two pages differing
// only in case make the tier-2 lookup ambiguous rather than picking one.
func TestResolvePromotePage_CaseAmbiguous(t *testing.T) {
	s := newTestStore(t)
	insertPromotePage(t, s, "compiled/Gateway", "compiled", "Gateway", "draft")
	insertPromotePage(t, s, "compiled/gateway", "compiled", "gateway", "draft")

	// The input matches neither name exactly, so tier 1 (binary comparison)
	// misses while tier 2 (fold-case) finds both — the case-variant pair is
	// ambiguous rather than resolved to either one.
	page, err := resolvePromotePage(s, "COMPILED/GATEWAY")
	if err == nil {
		t.Fatalf("expected ambiguity error, got page %v", page)
	}
	if page != nil {
		t.Errorf("page = %v, want nil alongside the error", page)
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error = %v, want ambiguity", err)
	}
	for _, name := range []string{"compiled/Gateway", "compiled/gateway"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error = %v, should list %q", err, name)
		}
	}
}

// TestResolvePromotePage_BareTagResolvesCompiled verifies tier 3: a bare
// tag resolves to the compiled namespace page.
func TestResolvePromotePage_BareTagResolvesCompiled(t *testing.T) {
	s := newTestStore(t)
	insertPromotePage(t, s, "compiled/gateway", "compiled", "gateway", "draft")

	page, err := resolvePromotePage(s, "gateway")
	if err != nil {
		t.Fatalf("resolvePromotePage: %v", err)
	}
	if page == nil {
		t.Fatal("expected page, got nil")
	}
	if page.Name != "compiled/gateway" {
		t.Errorf("page.Name = %q, want %q", page.Name, "compiled/gateway")
	}
}

// TestResolvePromotePage_BareTagResolvesLearning verifies tier 3: a bare
// tag resolves to the learning namespace page when that is the only match.
func TestResolvePromotePage_BareTagResolvesLearning(t *testing.T) {
	s := newTestStore(t)
	insertPromotePage(t, s, "learning/gateway", "learning", "gateway", "draft")

	page, err := resolvePromotePage(s, "gateway")
	if err != nil {
		t.Fatalf("resolvePromotePage: %v", err)
	}
	if page == nil {
		t.Fatal("expected page, got nil")
	}
	if page.Name != "learning/gateway" {
		t.Errorf("page.Name = %q, want %q", page.Name, "learning/gateway")
	}
}

// TestResolvePromotePage_BareTagResolvesKnowledgeSuffix verifies tier 3:
// a bare tag resolves to a knowledge-store page ending in /{tag}.
func TestResolvePromotePage_BareTagResolvesKnowledgeSuffix(t *testing.T) {
	s := newTestStore(t)
	insertPromotePage(t, s, "knowledge/team/authentication", "knowledge-team", "authentication", "draft")

	page, err := resolvePromotePage(s, "authentication")
	if err != nil {
		t.Fatalf("resolvePromotePage: %v", err)
	}
	if page == nil {
		t.Fatal("expected page, got nil")
	}
	if page.Name != "knowledge/team/authentication" {
		t.Errorf("page.Name = %q, want %q", page.Name, "knowledge/team/authentication")
	}
}

// TestResolvePromotePage_MixedCaseKnowledgeSuffix verifies the knowledge
// range query is case-insensitive end to end: a mixed-case knowledge page
// still resolves for a lowercase bare tag.
func TestResolvePromotePage_MixedCaseKnowledgeSuffix(t *testing.T) {
	s := newTestStore(t)
	insertPromotePage(t, s, "Knowledge/Team/Authentication", "knowledge-team", "authentication", "draft")

	page, err := resolvePromotePage(s, "authentication")
	if err != nil {
		t.Fatalf("resolvePromotePage: %v", err)
	}
	if page == nil {
		t.Fatal("expected page, got nil")
	}
	if page.Name != "Knowledge/Team/Authentication" {
		t.Errorf("page.Name = %q, want %q", page.Name, "Knowledge/Team/Authentication")
	}
}

// TestResolvePromotePage_SourceDocIDMatch verifies tier 4 for slashed
// inputs: "{source-id}/{doc-id}" resolves through the source doc lookup.
func TestResolvePromotePage_SourceDocIDMatch(t *testing.T) {
	s := newTestStore(t)
	insertPromotePage(t, s, "team/wiki-home", "team-wiki", "home", "draft")

	page, err := resolvePromotePage(s, "team-wiki/home")
	if err != nil {
		t.Fatalf("resolvePromotePage: %v", err)
	}
	if page == nil {
		t.Fatal("expected page, got nil")
	}
	if page.Name != "team/wiki-home" {
		t.Errorf("page.Name = %q, want %q", page.Name, "team/wiki-home")
	}
}

// TestResolvePromotePage_SourceDocIDBareMatch verifies tier 4 for bare
// tags: when no namespace candidate exists, a page whose source_doc_id in
// the compiled or learning source equals the bare input still resolves.
// The seeded page name deliberately matches no namespace pattern so only
// the source-doc lookup can find it.
func TestResolvePromotePage_SourceDocIDBareMatch(t *testing.T) {
	s := newTestStore(t)
	insertPromotePage(t, s, "archive/gateway-old", "compiled", "gateway", "draft")

	page, err := resolvePromotePage(s, "gateway")
	if err != nil {
		t.Fatalf("resolvePromotePage: %v", err)
	}
	if page == nil {
		t.Fatal("expected page, got nil")
	}
	if page.Name != "archive/gateway-old" {
		t.Errorf("page.Name = %q, want %q", page.Name, "archive/gateway-old")
	}
}

// TestResolvePromotePage_ExactBeatsNamespace verifies tier precedence: an
// exact-name page wins over a namespace candidate for the same bare tag.
func TestResolvePromotePage_ExactBeatsNamespace(t *testing.T) {
	s := newTestStore(t)
	insertPromotePage(t, s, "gateway", "disk-local", "gateway", "draft")
	insertPromotePage(t, s, "compiled/gateway", "compiled", "gateway", "draft")

	page, err := resolvePromotePage(s, "gateway")
	if err != nil {
		t.Fatalf("resolvePromotePage: %v", err)
	}
	if page == nil {
		t.Fatal("expected page, got nil")
	}
	if page.Name != "gateway" {
		t.Errorf("page.Name = %q, want exact match %q", page.Name, "gateway")
	}
}

// TestResolvePromotePage_NamespaceBeatsSourceDoc verifies tier precedence:
// a tier-3 namespace candidate wins over a tier-4 source-doc candidate.
func TestResolvePromotePage_NamespaceBeatsSourceDoc(t *testing.T) {
	s := newTestStore(t)
	insertPromotePage(t, s, "compiled/gateway", "compiled", "gateway-ns", "draft")
	// A tier-4-only candidate: its name matches no namespace pattern, and
	// its source doc id equals the bare tag under the learning source.
	insertPromotePage(t, s, "Article Gateway", "learning", "gateway", "draft")

	page, err := resolvePromotePage(s, "gateway")
	if err != nil {
		t.Fatalf("resolvePromotePage: %v", err)
	}
	if page == nil {
		t.Fatal("expected page, got nil")
	}
	if page.Name != "compiled/gateway" {
		t.Errorf("page.Name = %q, want namespace candidate %q", page.Name, "compiled/gateway")
	}
}

// TestResolvePromotePage_Ambiguous verifies that a bare tag matching both
// the learning and compiled namespaces is ambiguous, with both candidates
// listed.
func TestResolvePromotePage_Ambiguous(t *testing.T) {
	s := newTestStore(t)
	insertPromotePage(t, s, "learning/gateway", "learning", "gateway", "draft")
	insertPromotePage(t, s, "compiled/gateway", "compiled", "gateway", "draft")

	page, err := resolvePromotePage(s, "gateway")
	if err == nil {
		t.Fatalf("expected ambiguity error, got page %v", page)
	}
	if page != nil {
		t.Errorf("page = %v, want nil alongside the error", page)
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error = %v, want ambiguity", err)
	}
	for _, name := range []string{"compiled/gateway", "learning/gateway"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error = %v, should list %q", err, name)
		}
	}
}

// TestResolvePromotePage_AmbiguityCap verifies the ambiguity error lists
// at most ten candidate names, summarizing the rest with "and N more".
func TestResolvePromotePage_AmbiguityCap(t *testing.T) {
	s := newTestStore(t)
	for i := 1; i <= 12; i++ {
		name := fmt.Sprintf("knowledge/t%02d/x", i)
		insertPromotePage(t, s, name, "knowledge-team", fmt.Sprintf("x-%02d", i), "draft")
	}

	page, err := resolvePromotePage(s, "x")
	if err == nil {
		t.Fatalf("expected ambiguity error, got page %v", page)
	}
	if page != nil {
		t.Errorf("page = %v, want nil alongside the error", page)
	}
	if !strings.Contains(err.Error(), "and 2 more") {
		t.Errorf("error = %v, want capped match list ending in 'and 2 more'", err)
	}
}

// TestResolvePromotePage_Missing verifies that an unmatched name resolves
// to (nil, nil) so the handler can distinguish not-found from failure.
func TestResolvePromotePage_Missing(t *testing.T) {
	s := newTestStore(t)

	page, err := resolvePromotePage(s, "no-such-page")
	if err != nil {
		t.Fatalf("resolvePromotePage: %v", err)
	}
	if page != nil {
		t.Errorf("page = %v, want nil for missing page", page)
	}
}

// TestResolvePromotePage_EmptyName verifies that a whitespace-only name
// is rejected before any store query runs.
func TestResolvePromotePage_EmptyName(t *testing.T) {
	s := newTestStore(t)

	page, err := resolvePromotePage(s, "   ")
	if err == nil {
		t.Fatal("expected error for whitespace-only name")
	}
	if page != nil {
		t.Errorf("page = %v, want nil alongside the error", page)
	}
	if !strings.Contains(err.Error(), "required") {
		t.Errorf("error = %v, should mention 'required'", err)
	}
}

// TestResolvePromotePage_StoreError verifies that a failing store query
// surfaces as an error rather than being masked as not-found.
func TestResolvePromotePage_StoreError(t *testing.T) {
	s := newTestStore(t)
	if err := s.Close(); err != nil {
		t.Fatalf("Close store: %v", err)
	}

	page, err := resolvePromotePage(s, "compiled/gateway")
	if err == nil {
		t.Fatal("expected error from closed store, got nil")
	}
	if page != nil {
		t.Errorf("page = %v, want nil alongside the error", page)
	}
}

// TestResolvePromotePage_UnderscoreIsLiteral verifies that underscores in
// a bare tag are matched literally — a hyphenated page must not resolve —
// guarding against wildcard or separator reinterpretation.
func TestResolvePromotePage_UnderscoreIsLiteral(t *testing.T) {
	s := newTestStore(t)
	insertPromotePage(t, s, "compiled/auth-flow", "compiled", "auth-flow", "draft")

	page, err := resolvePromotePage(s, "auth_flow")
	if err != nil {
		t.Fatalf("resolvePromotePage: %v", err)
	}
	if page != nil {
		t.Errorf("page = %q, want nil: underscore must be matched literally", page.Name)
	}
}
