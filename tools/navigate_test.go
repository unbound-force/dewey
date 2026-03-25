package tools

// Note: The health MCP tool is defined inline in server.go (main package),
// not in tools/navigate.go. The Dewey-specific health fields are tested
// via the server integration in server_test.go. This file exists per the
// task specification (T042B) but the actual health tool test is in the
// main package where the tool is defined.
//
// This file tests the Navigate tool helpers that are used by the health tool
// and the decomposed ListPages helper functions.

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/unbound-force/dewey/types"
)

// TestNavigateNewNavigate verifies Navigate constructor.
func TestNavigateNewNavigate(t *testing.T) {
	nav := NewNavigate(nil)
	if nav == nil {
		t.Fatal("NewNavigate(nil) returned nil")
	}
}

// --- filterNonEmptyPages tests ---

func TestFilterNonEmptyPages_RemovesEmpty(t *testing.T) {
	pages := []types.PageEntity{
		{Name: "alpha"},
		{Name: ""},
		{Name: "beta"},
		{Name: ""},
	}

	got := filterNonEmptyPages(pages)

	if len(got) != 2 {
		t.Fatalf("filterNonEmptyPages() returned %d pages, want 2", len(got))
	}
	if got[0].Name != "alpha" {
		t.Errorf("got[0].Name = %q, want %q", got[0].Name, "alpha")
	}
	if got[1].Name != "beta" {
		t.Errorf("got[1].Name = %q, want %q", got[1].Name, "beta")
	}
}

func TestFilterNonEmptyPages_AllEmpty(t *testing.T) {
	pages := []types.PageEntity{
		{Name: ""},
		{Name: ""},
	}

	got := filterNonEmptyPages(pages)

	if len(got) != 0 {
		t.Errorf("filterNonEmptyPages() returned %d pages, want 0", len(got))
	}
}

func TestFilterNonEmptyPages_NoneEmpty(t *testing.T) {
	pages := []types.PageEntity{
		{Name: "one"},
		{Name: "two"},
	}

	got := filterNonEmptyPages(pages)

	if len(got) != 2 {
		t.Errorf("filterNonEmptyPages() returned %d pages, want 2", len(got))
	}
}

func TestFilterNonEmptyPages_NilInput(t *testing.T) {
	got := filterNonEmptyPages(nil)

	if len(got) != 0 {
		t.Errorf("filterNonEmptyPages(nil) returned %d pages, want 0", len(got))
	}
}

// --- filterPagesByNamespace tests ---

func TestFilterPagesByNamespace_MatchesPrefix(t *testing.T) {
	pages := []types.PageEntity{
		{Name: "projects/alpha"},
		{Name: "projects/beta"},
		{Name: "journal/2024-01-01"},
		{Name: "random"},
	}

	got := filterPagesByNamespace(pages, "projects/")

	if len(got) != 2 {
		t.Fatalf("filterPagesByNamespace() returned %d pages, want 2", len(got))
	}
	if got[0].Name != "projects/alpha" {
		t.Errorf("got[0].Name = %q, want %q", got[0].Name, "projects/alpha")
	}
	if got[1].Name != "projects/beta" {
		t.Errorf("got[1].Name = %q, want %q", got[1].Name, "projects/beta")
	}
}

func TestFilterPagesByNamespace_CaseInsensitive(t *testing.T) {
	pages := []types.PageEntity{
		{Name: "Projects/Alpha"},
		{Name: "PROJECTS/Beta"},
		{Name: "other"},
	}

	got := filterPagesByNamespace(pages, "projects/")

	if len(got) != 2 {
		t.Fatalf("filterPagesByNamespace() case-insensitive returned %d pages, want 2", len(got))
	}
}

func TestFilterPagesByNamespace_NoMatches(t *testing.T) {
	pages := []types.PageEntity{
		{Name: "alpha"},
		{Name: "beta"},
	}

	got := filterPagesByNamespace(pages, "projects/")

	if len(got) != 0 {
		t.Errorf("filterPagesByNamespace() returned %d pages, want 0", len(got))
	}
}

func TestFilterPagesByNamespace_EmptyInput(t *testing.T) {
	got := filterPagesByNamespace(nil, "projects/")

	if len(got) != 0 {
		t.Errorf("filterPagesByNamespace(nil) returned %d pages, want 0", len(got))
	}
}

// --- filterPagesByProperty tests ---

func TestFilterPagesByProperty_MatchesKey(t *testing.T) {
	pages := []types.PageEntity{
		{Name: "with-status", Properties: map[string]any{"status": "active"}},
		{Name: "with-tags", Properties: map[string]any{"tags": []string{"go"}}},
		{Name: "no-props"},
		{Name: "nil-props", Properties: nil},
	}

	got := filterPagesByProperty(pages, "status")

	if len(got) != 1 {
		t.Fatalf("filterPagesByProperty() returned %d pages, want 1", len(got))
	}
	if got[0].Name != "with-status" {
		t.Errorf("got[0].Name = %q, want %q", got[0].Name, "with-status")
	}
}

func TestFilterPagesByProperty_NoMatches(t *testing.T) {
	pages := []types.PageEntity{
		{Name: "a", Properties: map[string]any{"tags": "go"}},
		{Name: "b", Properties: map[string]any{"priority": "high"}},
	}

	got := filterPagesByProperty(pages, "nonexistent")

	if len(got) != 0 {
		t.Errorf("filterPagesByProperty() returned %d pages, want 0", len(got))
	}
}

func TestFilterPagesByProperty_NilProperties(t *testing.T) {
	pages := []types.PageEntity{
		{Name: "no-props"},
		{Name: "nil-props", Properties: nil},
	}

	got := filterPagesByProperty(pages, "anything")

	if len(got) != 0 {
		t.Errorf("filterPagesByProperty() with nil properties returned %d pages, want 0", len(got))
	}
}

func TestFilterPagesByProperty_MultipleMatches(t *testing.T) {
	pages := []types.PageEntity{
		{Name: "a", Properties: map[string]any{"status": "active"}},
		{Name: "b", Properties: map[string]any{"status": "done"}},
		{Name: "c", Properties: map[string]any{"other": "val"}},
	}

	got := filterPagesByProperty(pages, "status")

	if len(got) != 2 {
		t.Fatalf("filterPagesByProperty() returned %d pages, want 2", len(got))
	}
	if got[0].Name != "a" {
		t.Errorf("got[0].Name = %q, want %q", got[0].Name, "a")
	}
	if got[1].Name != "b" {
		t.Errorf("got[1].Name = %q, want %q", got[1].Name, "b")
	}
}

// --- filterPagesByTag tests ---

func TestFilterPagesByTag_MatchesTag(t *testing.T) {
	mb := newMockBackend()
	// Page with blocks containing a #project tag.
	mb.addPage(types.PageEntity{Name: "tagged-page"}, types.BlockEntity{
		UUID:    "b-1",
		Content: "Some content #project",
	})
	// Page with blocks that do NOT contain the tag.
	mb.addPage(types.PageEntity{Name: "untagged-page"}, types.BlockEntity{
		UUID:    "b-2",
		Content: "No tags here",
	})

	pages := []types.PageEntity{
		{Name: "tagged-page"},
		{Name: "untagged-page"},
	}

	got := filterPagesByTag(context.Background(), mb, pages, "project")

	if len(got) != 1 {
		t.Fatalf("filterPagesByTag() returned %d pages, want 1", len(got))
	}
	if got[0].Name != "tagged-page" {
		t.Errorf("got[0].Name = %q, want %q", got[0].Name, "tagged-page")
	}
}

func TestFilterPagesByTag_CaseInsensitive(t *testing.T) {
	mb := newMockBackend()
	mb.addPage(types.PageEntity{Name: "page-a"}, types.BlockEntity{
		UUID:    "b-1",
		Content: "Has #GoLang tag",
	})

	pages := []types.PageEntity{{Name: "page-a"}}

	got := filterPagesByTag(context.Background(), mb, pages, "golang")

	if len(got) != 1 {
		t.Fatalf("filterPagesByTag() case-insensitive returned %d pages, want 1", len(got))
	}
}

func TestFilterPagesByTag_NoMatches(t *testing.T) {
	mb := newMockBackend()
	mb.addPage(types.PageEntity{Name: "page-a"}, types.BlockEntity{
		UUID:    "b-1",
		Content: "No matching tags #other",
	})

	pages := []types.PageEntity{{Name: "page-a"}}

	got := filterPagesByTag(context.Background(), mb, pages, "nonexistent")

	if len(got) != 0 {
		t.Errorf("filterPagesByTag() returned %d pages, want 0", len(got))
	}
}

func TestFilterPagesByTag_EmptyPages(t *testing.T) {
	mb := newMockBackend()

	got := filterPagesByTag(context.Background(), mb, nil, "tag")

	if len(got) != 0 {
		t.Errorf("filterPagesByTag(nil) returned %d pages, want 0", len(got))
	}
}

func TestFilterPagesByTag_BlockTreeError(t *testing.T) {
	mb := newMockBackend()
	// Page exists but has no blocks registered → GetPageBlocksTree returns nil, nil.
	// The function should skip it (no error, but no tag match).
	pages := []types.PageEntity{{Name: "no-blocks"}}

	got := filterPagesByTag(context.Background(), mb, pages, "tag")

	if len(got) != 0 {
		t.Errorf("filterPagesByTag() with no blocks returned %d pages, want 0", len(got))
	}
}

func TestFilterPagesByTag_NestedBlocks(t *testing.T) {
	mb := newMockBackend()
	// Tag in a child block should still be found.
	mb.addPage(types.PageEntity{Name: "nested-page"}, types.BlockEntity{
		UUID:    "parent",
		Content: "Parent content",
		Children: []types.BlockEntity{
			{UUID: "child", Content: "Has #deep tag"},
		},
	})

	pages := []types.PageEntity{{Name: "nested-page"}}

	got := filterPagesByTag(context.Background(), mb, pages, "deep")

	if len(got) != 1 {
		t.Fatalf("filterPagesByTag() with nested tag returned %d pages, want 1", len(got))
	}
}

// --- sortAndPaginatePages tests ---

func TestSortAndPaginatePages_SortByName(t *testing.T) {
	pages := []types.PageEntity{
		{Name: "charlie"},
		{Name: "alpha"},
		{Name: "bravo"},
	}

	got := sortAndPaginatePages(pages, "", 0)

	if len(got) != 3 {
		t.Fatalf("sortAndPaginatePages() returned %d pages, want 3", len(got))
	}
	want := []string{"alpha", "bravo", "charlie"}
	for i, w := range want {
		if got[i].Name != w {
			t.Errorf("got[%d].Name = %q, want %q", i, got[i].Name, w)
		}
	}
}

func TestSortAndPaginatePages_SortByModified(t *testing.T) {
	pages := []types.PageEntity{
		{Name: "oldest", UpdatedAt: 100},
		{Name: "newest", UpdatedAt: 300},
		{Name: "middle", UpdatedAt: 200},
	}

	got := sortAndPaginatePages(pages, "modified", 0)

	// Modified sorts descending (most recent first).
	if got[0].Name != "newest" {
		t.Errorf("got[0].Name = %q, want %q (most recent)", got[0].Name, "newest")
	}
	if got[1].Name != "middle" {
		t.Errorf("got[1].Name = %q, want %q", got[1].Name, "middle")
	}
	if got[2].Name != "oldest" {
		t.Errorf("got[2].Name = %q, want %q (least recent)", got[2].Name, "oldest")
	}
}

func TestSortAndPaginatePages_SortByCreated(t *testing.T) {
	pages := []types.PageEntity{
		{Name: "first", CreatedAt: 100},
		{Name: "third", CreatedAt: 300},
		{Name: "second", CreatedAt: 200},
	}

	got := sortAndPaginatePages(pages, "created", 0)

	// Created sorts descending (most recent first).
	if got[0].Name != "third" {
		t.Errorf("got[0].Name = %q, want %q (most recent)", got[0].Name, "third")
	}
	if got[2].Name != "first" {
		t.Errorf("got[2].Name = %q, want %q (least recent)", got[2].Name, "first")
	}
}

func TestSortAndPaginatePages_DefaultLimit50(t *testing.T) {
	pages := make([]types.PageEntity, 60)
	for i := range pages {
		pages[i] = types.PageEntity{Name: "page"}
	}

	got := sortAndPaginatePages(pages, "name", 0)

	if len(got) != 50 {
		t.Errorf("sortAndPaginatePages() with limit=0 returned %d pages, want 50 (default)", len(got))
	}
}

func TestSortAndPaginatePages_CustomLimit(t *testing.T) {
	pages := make([]types.PageEntity, 10)
	for i := range pages {
		pages[i] = types.PageEntity{Name: "page"}
	}

	got := sortAndPaginatePages(pages, "name", 3)

	if len(got) != 3 {
		t.Errorf("sortAndPaginatePages() with limit=3 returned %d pages, want 3", len(got))
	}
}

func TestSortAndPaginatePages_LimitLargerThanInput(t *testing.T) {
	pages := []types.PageEntity{
		{Name: "alpha"},
		{Name: "bravo"},
	}

	got := sortAndPaginatePages(pages, "name", 100)

	if len(got) != 2 {
		t.Errorf("sortAndPaginatePages() with limit > len returned %d pages, want 2", len(got))
	}
}

func TestSortAndPaginatePages_NegativeLimit(t *testing.T) {
	pages := make([]types.PageEntity, 60)
	for i := range pages {
		pages[i] = types.PageEntity{Name: "page"}
	}

	got := sortAndPaginatePages(pages, "name", -1)

	// Negative limit should default to 50 (same as 0).
	if len(got) != 50 {
		t.Errorf("sortAndPaginatePages() with limit=-1 returned %d pages, want 50 (default)", len(got))
	}
}

func TestSortAndPaginatePages_EmptySlice(t *testing.T) {
	got := sortAndPaginatePages(nil, "name", 10)

	if len(got) != 0 {
		t.Errorf("sortAndPaginatePages(nil) returned %d pages, want 0", len(got))
	}
}

// --- buildPageSummaries tests ---

func TestBuildPageSummaries_CorrectStructure(t *testing.T) {
	pages := []types.PageEntity{
		{
			Name:         "test-page",
			OriginalName: "Test Page",
			Journal:      false,
			Properties:   map[string]any{"status": "active"},
			UpdatedAt:    1700000000,
		},
	}

	got := buildPageSummaries(pages)

	if len(got) != 1 {
		t.Fatalf("buildPageSummaries() returned %d summaries, want 1", len(got))
	}

	s := got[0]

	// name should be OriginalName.
	if s["name"] != "Test Page" {
		t.Errorf("summary[name] = %v, want %q", s["name"], "Test Page")
	}

	// properties should be present.
	props, ok := s["properties"].(map[string]any)
	if !ok {
		t.Fatalf("summary[properties] type = %T, want map[string]any", s["properties"])
	}
	if props["status"] != "active" {
		t.Errorf("summary[properties][status] = %v, want %q", props["status"], "active")
	}

	// journal should be present.
	if s["journal"] != false {
		t.Errorf("summary[journal] = %v, want false", s["journal"])
	}

	// updatedAt should be present when > 0.
	if s["updatedAt"] != int64(1700000000) {
		t.Errorf("summary[updatedAt] = %v, want %d", s["updatedAt"], 1700000000)
	}
}

func TestBuildPageSummaries_OmitsZeroUpdatedAt(t *testing.T) {
	pages := []types.PageEntity{
		{Name: "no-update", OriginalName: "No Update", UpdatedAt: 0},
	}

	got := buildPageSummaries(pages)

	if len(got) != 1 {
		t.Fatalf("buildPageSummaries() returned %d summaries, want 1", len(got))
	}
	if _, exists := got[0]["updatedAt"]; exists {
		t.Errorf("summary should not contain updatedAt when UpdatedAt is 0")
	}
}

func TestBuildPageSummaries_MultiplePages(t *testing.T) {
	pages := []types.PageEntity{
		{Name: "a", OriginalName: "Page A"},
		{Name: "b", OriginalName: "Page B"},
		{Name: "c", OriginalName: "Page C"},
	}

	got := buildPageSummaries(pages)

	if len(got) != 3 {
		t.Fatalf("buildPageSummaries() returned %d summaries, want 3", len(got))
	}
	for i, name := range []string{"Page A", "Page B", "Page C"} {
		if got[i]["name"] != name {
			t.Errorf("got[%d][name] = %v, want %q", i, got[i]["name"], name)
		}
	}
}

func TestBuildPageSummaries_EmptyInput(t *testing.T) {
	got := buildPageSummaries(nil)

	// Should return an empty (not nil) slice since make([]..., 0) is used.
	if got == nil {
		t.Error("buildPageSummaries(nil) returned nil, want empty slice")
	}
	if len(got) != 0 {
		t.Errorf("buildPageSummaries(nil) returned %d summaries, want 0", len(got))
	}
}

func TestBuildPageSummaries_JournalPage(t *testing.T) {
	pages := []types.PageEntity{
		{Name: "jan 1st, 2024", OriginalName: "Jan 1st, 2024", Journal: true},
	}

	got := buildPageSummaries(pages)

	if len(got) != 1 {
		t.Fatalf("buildPageSummaries() returned %d summaries, want 1", len(got))
	}
	if got[0]["journal"] != true {
		t.Errorf("summary[journal] = %v, want true", got[0]["journal"])
	}
}

func TestBuildPageSummaries_NilProperties(t *testing.T) {
	pages := []types.PageEntity{
		{Name: "no-props", OriginalName: "No Props", Properties: nil},
	}

	got := buildPageSummaries(pages)

	if len(got) != 1 {
		t.Fatalf("buildPageSummaries() returned %d summaries, want 1", len(got))
	}
	// properties key should exist but be nil (the map stores it).
	if _, exists := got[0]["properties"]; !exists {
		t.Error("summary should contain properties key even when nil")
	}
}

// --- getBacklinks tests (T021) ---

func TestGetBacklinks_ValidReferences(t *testing.T) {
	mb := newMockBackend()
	// Simulate linked references JSON: array of [pageEntity, [blocks...]]
	mb.linkedRefs["my-page"] = json.RawMessage(`[
		[
			{"name": "source-page", "originalName": "Source Page"},
			[{"uuid": "block-1", "content": "Links to [[my-page]]"}]
		],
		[
			{"name": "other-page", "originalName": "Other Page"},
			[
				{"uuid": "block-2", "content": "Also refs [[my-page]]"},
				{"uuid": "block-3", "content": "Another ref"}
			]
		]
	]`)

	nav := NewNavigate(mb)
	backlinks := nav.getBacklinks(context.Background(), "my-page")

	if len(backlinks) != 2 {
		t.Fatalf("getBacklinks() returned %d backlinks, want 2", len(backlinks))
	}

	// First backlink should use OriginalName when available.
	if backlinks[0].PageName != "Source Page" {
		t.Errorf("backlinks[0].PageName = %q, want %q", backlinks[0].PageName, "Source Page")
	}
	if len(backlinks[0].Blocks) != 1 {
		t.Fatalf("backlinks[0] has %d blocks, want 1", len(backlinks[0].Blocks))
	}
	if backlinks[0].Blocks[0].UUID != "block-1" {
		t.Errorf("backlinks[0].Blocks[0].UUID = %q, want %q", backlinks[0].Blocks[0].UUID, "block-1")
	}
	if backlinks[0].Blocks[0].Content != "Links to [[my-page]]" {
		t.Errorf("backlinks[0].Blocks[0].Content = %q, want %q", backlinks[0].Blocks[0].Content, "Links to [[my-page]]")
	}

	// Second backlink should have two blocks.
	if backlinks[1].PageName != "Other Page" {
		t.Errorf("backlinks[1].PageName = %q, want %q", backlinks[1].PageName, "Other Page")
	}
	if len(backlinks[1].Blocks) != 2 {
		t.Errorf("backlinks[1] has %d blocks, want 2", len(backlinks[1].Blocks))
	}
}

func TestGetBacklinks_FallbackToName(t *testing.T) {
	mb := newMockBackend()
	// Page with Name but no OriginalName — getBacklinks should fall back to Name.
	mb.linkedRefs["target"] = json.RawMessage(`[
		[
			{"name": "fallback-page"},
			[{"uuid": "b-1", "content": "ref"}]
		]
	]`)

	nav := NewNavigate(mb)
	backlinks := nav.getBacklinks(context.Background(), "target")

	if len(backlinks) != 1 {
		t.Fatalf("getBacklinks() returned %d backlinks, want 1", len(backlinks))
	}
	if backlinks[0].PageName != "fallback-page" {
		t.Errorf("PageName = %q, want %q (should fall back to Name)", backlinks[0].PageName, "fallback-page")
	}
}

func TestGetBacklinks_BackendError(t *testing.T) {
	mb := newMockBackend()
	mb.getLinkedRefsErr = fmt.Errorf("backend unavailable")

	nav := NewNavigate(mb)
	backlinks := nav.getBacklinks(context.Background(), "any-page")

	if backlinks != nil {
		t.Errorf("getBacklinks() = %v, want nil on backend error", backlinks)
	}
}

func TestGetBacklinks_InvalidJSON(t *testing.T) {
	mb := newMockBackend()
	mb.linkedRefs["bad-json"] = json.RawMessage(`not valid json`)

	nav := NewNavigate(mb)
	backlinks := nav.getBacklinks(context.Background(), "bad-json")

	if backlinks != nil {
		t.Errorf("getBacklinks() = %v, want nil on invalid JSON", backlinks)
	}
}

func TestGetBacklinks_EmptyReferences(t *testing.T) {
	mb := newMockBackend()
	mb.linkedRefs["lonely-page"] = json.RawMessage(`[]`)

	nav := NewNavigate(mb)
	backlinks := nav.getBacklinks(context.Background(), "lonely-page")

	if len(backlinks) != 0 {
		t.Errorf("getBacklinks() returned %d backlinks, want 0", len(backlinks))
	}
}

func TestGetBacklinks_MalformedRefEntry(t *testing.T) {
	mb := newMockBackend()
	// Array entries with fewer than 2 elements should be skipped.
	mb.linkedRefs["partial"] = json.RawMessage(`[
		[{"name": "only-page"}],
		[
			{"name": "good-page", "originalName": "Good Page"},
			[{"uuid": "b-1", "content": "valid"}]
		]
	]`)

	nav := NewNavigate(mb)
	backlinks := nav.getBacklinks(context.Background(), "partial")

	if len(backlinks) != 1 {
		t.Fatalf("getBacklinks() returned %d backlinks, want 1 (skip malformed)", len(backlinks))
	}
	if backlinks[0].PageName != "Good Page" {
		t.Errorf("PageName = %q, want %q", backlinks[0].PageName, "Good Page")
	}
}

// --- truncateEnrichedChildren tests (T021) ---

func TestTruncateEnrichedChildren_LimitExceeded(t *testing.T) {
	blocks := []types.BlockEntity{
		{UUID: "a", Content: "block-a"},
		{UUID: "b", Content: "block-b"},
		{UUID: "c", Content: "block-c"},
	}
	remaining := 2

	result := truncateEnrichedChildren(blocks, &remaining)

	if len(result) != 2 {
		t.Fatalf("truncateEnrichedChildren() returned %d blocks, want 2", len(result))
	}
	if remaining != 0 {
		t.Errorf("remaining = %d, want 0", remaining)
	}
	if result[0].UUID != "a" {
		t.Errorf("result[0].UUID = %q, want %q", result[0].UUID, "a")
	}
	if result[1].UUID != "b" {
		t.Errorf("result[1].UUID = %q, want %q", result[1].UUID, "b")
	}
}

func TestTruncateEnrichedChildren_ZeroRemaining(t *testing.T) {
	blocks := []types.BlockEntity{
		{UUID: "a", Content: "block-a"},
	}
	remaining := 0

	result := truncateEnrichedChildren(blocks, &remaining)

	if len(result) != 0 {
		t.Errorf("truncateEnrichedChildren() returned %d blocks, want 0 when remaining=0", len(result))
	}
}

func TestTruncateEnrichedChildren_WithNestedChildren(t *testing.T) {
	blocks := []types.BlockEntity{
		{
			UUID:    "parent",
			Content: "parent block",
			Children: []types.BlockEntity{
				{UUID: "child-1", Content: "child 1"},
				{UUID: "child-2", Content: "child 2"},
				{UUID: "child-3", Content: "child 3"},
			},
		},
		{UUID: "sibling", Content: "sibling block"},
	}
	// Allow 3 total: parent (1) + 2 children = 3, so sibling excluded, child-3 excluded.
	remaining := 3

	result := truncateEnrichedChildren(blocks, &remaining)

	if len(result) != 1 {
		t.Fatalf("truncateEnrichedChildren() returned %d top-level blocks, want 1", len(result))
	}
	if result[0].UUID != "parent" {
		t.Errorf("result[0].UUID = %q, want %q", result[0].UUID, "parent")
	}
	// Parent uses 1 slot, leaving 2 for children.
	if len(result[0].Children) != 2 {
		t.Errorf("result[0] has %d children, want 2", len(result[0].Children))
	}
	if remaining != 0 {
		t.Errorf("remaining = %d, want 0", remaining)
	}
}

func TestTruncateEnrichedChildren_AllFit(t *testing.T) {
	blocks := []types.BlockEntity{
		{UUID: "a", Content: "block-a"},
		{UUID: "b", Content: "block-b"},
	}
	remaining := 10

	result := truncateEnrichedChildren(blocks, &remaining)

	if len(result) != 2 {
		t.Errorf("truncateEnrichedChildren() returned %d blocks, want 2", len(result))
	}
	if remaining != 8 {
		t.Errorf("remaining = %d, want 8", remaining)
	}
}

func TestTruncateEnrichedChildren_NilsChildrenWhenNoRemaining(t *testing.T) {
	blocks := []types.BlockEntity{
		{
			UUID:    "parent",
			Content: "parent block",
			Children: []types.BlockEntity{
				{UUID: "child-1", Content: "child 1"},
			},
		},
	}
	// Only 1 remaining: parent gets in, but no budget left for children.
	remaining := 1

	result := truncateEnrichedChildren(blocks, &remaining)

	if len(result) != 1 {
		t.Fatalf("truncateEnrichedChildren() returned %d blocks, want 1", len(result))
	}
	if result[0].Children != nil {
		t.Errorf("expected Children to be nil when no remaining budget, got %v", result[0].Children)
	}
}

func TestTruncateEnrichedChildren_DeeplyNested(t *testing.T) {
	blocks := []types.BlockEntity{
		{
			UUID:    "level-0",
			Content: "level 0",
			Children: []types.BlockEntity{
				{
					UUID:    "level-1",
					Content: "level 1",
					Children: []types.BlockEntity{
						{UUID: "level-2", Content: "level 2"},
					},
				},
			},
		},
	}
	// Budget: 2 — level-0 (1) + level-1 (1) = 2, level-2 should be excluded.
	remaining := 2

	result := truncateEnrichedChildren(blocks, &remaining)

	if len(result) != 1 {
		t.Fatalf("truncateEnrichedChildren() returned %d top-level blocks, want 1", len(result))
	}
	if len(result[0].Children) != 1 {
		t.Fatalf("level-0 has %d children, want 1", len(result[0].Children))
	}
	// level-1's children should be nil (no remaining after level-1 consumed its slot).
	if result[0].Children[0].Children != nil {
		t.Errorf("level-1 children should be nil, got %v", result[0].Children[0].Children)
	}
	if remaining != 0 {
		t.Errorf("remaining = %d, want 0", remaining)
	}
}

// --- sortByField tests (T021) ---

func TestSortByField_SortByUpdatedAtDescending(t *testing.T) {
	pages := []types.PageEntity{
		{Name: "old", UpdatedAt: 100},
		{Name: "newest", UpdatedAt: 300},
		{Name: "middle", UpdatedAt: 200},
	}

	// Sort by -UpdatedAt (descending, like sortPages "modified").
	sortByField(pages, func(p types.PageEntity) int64 { return -p.UpdatedAt })

	want := []string{"newest", "middle", "old"}
	for i, name := range want {
		if pages[i].Name != name {
			t.Errorf("pages[%d].Name = %q, want %q", i, pages[i].Name, name)
		}
	}
}

func TestSortByField_SortByCreatedAtDescending(t *testing.T) {
	pages := []types.PageEntity{
		{Name: "first", CreatedAt: 10},
		{Name: "third", CreatedAt: 30},
		{Name: "second", CreatedAt: 20},
	}

	sortByField(pages, func(p types.PageEntity) int64 { return -p.CreatedAt })

	want := []string{"third", "second", "first"}
	for i, name := range want {
		if pages[i].Name != name {
			t.Errorf("pages[%d].Name = %q, want %q", i, pages[i].Name, name)
		}
	}
}

func TestSortByField_SingleElement(t *testing.T) {
	pages := []types.PageEntity{
		{Name: "only", UpdatedAt: 100},
	}

	sortByField(pages, func(p types.PageEntity) int64 { return p.UpdatedAt })

	if pages[0].Name != "only" {
		t.Errorf("pages[0].Name = %q, want %q", pages[0].Name, "only")
	}
}

func TestSortByField_EmptySlice(t *testing.T) {
	var pages []types.PageEntity

	// Should not panic on empty slice.
	sortByField(pages, func(p types.PageEntity) int64 { return p.UpdatedAt })

	if len(pages) != 0 {
		t.Errorf("expected empty slice, got %d elements", len(pages))
	}
}

func TestSortByField_AlreadySorted(t *testing.T) {
	pages := []types.PageEntity{
		{Name: "a", UpdatedAt: 1},
		{Name: "b", UpdatedAt: 2},
		{Name: "c", UpdatedAt: 3},
	}

	sortByField(pages, func(p types.PageEntity) int64 { return p.UpdatedAt })

	want := []string{"a", "b", "c"}
	for i, name := range want {
		if pages[i].Name != name {
			t.Errorf("pages[%d].Name = %q, want %q", i, pages[i].Name, name)
		}
	}
}

func TestSortByField_StableForEqualValues(t *testing.T) {
	pages := []types.PageEntity{
		{Name: "alpha", UpdatedAt: 100},
		{Name: "beta", UpdatedAt: 100},
		{Name: "gamma", UpdatedAt: 100},
	}

	// Insertion sort is stable, so equal elements should preserve order.
	sortByField(pages, func(p types.PageEntity) int64 { return p.UpdatedAt })

	want := []string{"alpha", "beta", "gamma"}
	for i, name := range want {
		if pages[i].Name != name {
			t.Errorf("pages[%d].Name = %q, want %q (stable sort)", i, pages[i].Name, name)
		}
	}
}

func TestSortByField_AscendingOrder(t *testing.T) {
	pages := []types.PageEntity{
		{Name: "c", UpdatedAt: 300},
		{Name: "a", UpdatedAt: 100},
		{Name: "b", UpdatedAt: 200},
	}

	// Positive key = ascending order.
	sortByField(pages, func(p types.PageEntity) int64 { return p.UpdatedAt })

	want := []string{"a", "b", "c"}
	for i, name := range want {
		if pages[i].Name != name {
			t.Errorf("pages[%d].Name = %q, want %q", i, pages[i].Name, name)
		}
	}
}

// --- pageHasTag tests (T021) ---

func TestPageHasTag_TagPresent(t *testing.T) {
	blocks := []types.BlockEntity{
		{UUID: "b-1", Content: "Some content #project"},
	}

	if !pageHasTag(blocks, "project") {
		t.Error("pageHasTag() = false, want true when tag is present")
	}
}

func TestPageHasTag_TagAbsent(t *testing.T) {
	blocks := []types.BlockEntity{
		{UUID: "b-1", Content: "Some content without tags"},
	}

	if pageHasTag(blocks, "project") {
		t.Error("pageHasTag() = true, want false when tag is absent")
	}
}

func TestPageHasTag_CaseInsensitive(t *testing.T) {
	// Parser extracts "Project" (preserving case), pageHasTag compares with ToLower.
	blocks := []types.BlockEntity{
		{UUID: "b-1", Content: "Content with #Project tag"},
	}

	if !pageHasTag(blocks, "project") {
		t.Error("pageHasTag() = false, want true for case-insensitive match")
	}
}

func TestPageHasTag_TagInNestedChildren(t *testing.T) {
	blocks := []types.BlockEntity{
		{
			UUID:    "parent",
			Content: "Parent block",
			Children: []types.BlockEntity{
				{
					UUID:    "child",
					Content: "Child block",
					Children: []types.BlockEntity{
						{UUID: "grandchild", Content: "Deep block #hidden"},
					},
				},
			},
		},
	}

	if !pageHasTag(blocks, "hidden") {
		t.Error("pageHasTag() = false, want true when tag is in deeply nested child")
	}
}

func TestPageHasTag_EmptyBlocks(t *testing.T) {
	var blocks []types.BlockEntity

	if pageHasTag(blocks, "anything") {
		t.Error("pageHasTag() = true, want false for empty blocks")
	}
}

func TestPageHasTag_MultipleBlocks(t *testing.T) {
	blocks := []types.BlockEntity{
		{UUID: "b-1", Content: "No tags here"},
		{UUID: "b-2", Content: "Also no tags"},
		{UUID: "b-3", Content: "Found #target here"},
	}

	if !pageHasTag(blocks, "target") {
		t.Error("pageHasTag() = false, want true when tag is in later block")
	}
}

func TestPageHasTag_BracketTag(t *testing.T) {
	// #[[multi word tag]] syntax — parser extracts "multi word tag".
	blocks := []types.BlockEntity{
		{UUID: "b-1", Content: "Content with #[[Important Topic]]"},
	}

	if !pageHasTag(blocks, "important topic") {
		t.Error("pageHasTag() = false, want true for bracket tag #[[...]]")
	}
}

func TestPageHasTag_DoesNotMatchPartialTag(t *testing.T) {
	blocks := []types.BlockEntity{
		{UUID: "b-1", Content: "Content with #project-alpha"},
	}

	// "project" should NOT match "project-alpha" because the parser extracts
	// the full tag "project-alpha" and case-insensitive comparison is exact.
	if pageHasTag(blocks, "project") {
		t.Error("pageHasTag() = true, want false — should not match partial tag name")
	}
}

func TestPageHasTag_TagOnlyInChildNotParent(t *testing.T) {
	blocks := []types.BlockEntity{
		{
			UUID:    "parent",
			Content: "No tags in parent",
			Children: []types.BlockEntity{
				{UUID: "child", Content: "#found here"},
			},
		},
	}

	if !pageHasTag(blocks, "found") {
		t.Error("pageHasTag() = false, want true when tag is only in child block")
	}
}

// --- sortPages integration tests (T021 — exercises sortByField via sortPages) ---

func TestSortPages_ModifiedField(t *testing.T) {
	pages := []types.PageEntity{
		{Name: "old", UpdatedAt: 100},
		{Name: "newest", UpdatedAt: 300},
		{Name: "middle", UpdatedAt: 200},
	}

	sortPages(pages, "modified")

	// "modified" sorts by -UpdatedAt (descending).
	want := []string{"newest", "middle", "old"}
	for i, name := range want {
		if pages[i].Name != name {
			t.Errorf("sortPages(modified): pages[%d].Name = %q, want %q", i, pages[i].Name, name)
		}
	}
}

func TestSortPages_CreatedField(t *testing.T) {
	pages := []types.PageEntity{
		{Name: "first", CreatedAt: 10},
		{Name: "third", CreatedAt: 30},
		{Name: "second", CreatedAt: 20},
	}

	sortPages(pages, "created")

	// "created" sorts by -CreatedAt (descending).
	want := []string{"third", "second", "first"}
	for i, name := range want {
		if pages[i].Name != name {
			t.Errorf("sortPages(created): pages[%d].Name = %q, want %q", i, pages[i].Name, name)
		}
	}
}

func TestSortPages_DefaultSortByName(t *testing.T) {
	pages := []types.PageEntity{
		{Name: "Zebra"},
		{Name: "Apple"},
		{Name: "Mango"},
	}

	sortPages(pages, "name")

	want := []string{"Apple", "Mango", "Zebra"}
	for i, name := range want {
		if pages[i].Name != name {
			t.Errorf("sortPages(name): pages[%d].Name = %q, want %q", i, pages[i].Name, name)
		}
	}
}

func TestSortPages_UnknownFieldDefaultsToName(t *testing.T) {
	pages := []types.PageEntity{
		{Name: "Zebra"},
		{Name: "Apple"},
		{Name: "Mango"},
	}

	sortPages(pages, "unknown_field")

	// Unknown sortBy should default to sorting by name.
	want := []string{"Apple", "Mango", "Zebra"}
	for i, name := range want {
		if pages[i].Name != name {
			t.Errorf("sortPages(unknown): pages[%d].Name = %q, want %q", i, pages[i].Name, name)
		}
	}
}
