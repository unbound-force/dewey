package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/unbound-force/dewey/types"
)

func TestGetPage_Success(t *testing.T) {
	mb := newMockBackend()
	mb.addPage(types.PageEntity{
		Name:         "test-page",
		OriginalName: "Test Page",
		UUID:         "page-uuid",
	}, types.BlockEntity{
		UUID:    "block-1",
		Content: "Hello [[World]]",
	}, types.BlockEntity{
		UUID:    "block-2",
		Content: "Another block",
	})
	nav := NewNavigate(mb)

	result, _, err := nav.GetPage(context.Background(), nil, types.GetPageInput{
		Name: "test-page",
	})
	if err != nil {
		t.Fatalf("GetPage() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("GetPage() returned error result")
	}

	var parsed map[string]any
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	// Should have page data.
	if parsed["page"] == nil {
		t.Error("expected page in result")
	}

	// Should report block count.
	if parsed["blockCount"] == nil {
		t.Error("expected blockCount in result")
	}

	// Should collect outgoing links.
	links, ok := parsed["outgoingLinks"].([]any)
	if !ok {
		t.Fatal("expected outgoingLinks to be an array")
	}
	if len(links) != 1 || links[0] != "World" {
		t.Errorf("outgoingLinks = %v, want [World]", links)
	}
}

func TestGetPage_NotFound(t *testing.T) {
	mb := newMockBackend()
	nav := NewNavigate(mb)

	result, _, err := nav.GetPage(context.Background(), nil, types.GetPageInput{
		Name: "nonexistent",
	})
	if err != nil {
		t.Fatalf("GetPage() error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for nonexistent page")
	}
}

func TestGetPage_Compact(t *testing.T) {
	mb := newMockBackend()
	mb.addPage(types.PageEntity{
		Name: "compact-page",
	}, types.BlockEntity{
		UUID:    "b-1",
		Content: "Block one",
	}, types.BlockEntity{
		UUID:    "b-2",
		Content: "Block two",
	})
	nav := NewNavigate(mb)

	result, _, err := nav.GetPage(context.Background(), nil, types.GetPageInput{
		Name:    "compact-page",
		Compact: true,
	})
	if err != nil {
		t.Fatalf("GetPage() compact error: %v", err)
	}
	if result.IsError {
		t.Fatalf("GetPage() compact returned error result")
	}

	var parsed map[string]any
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	// Compact mode: blocks should be map[string]string with uuid+content.
	blocks, ok := parsed["blocks"].([]any)
	if !ok {
		t.Fatal("expected blocks array in compact mode")
	}
	if len(blocks) != 2 {
		t.Errorf("expected 2 blocks, got %d", len(blocks))
	}
}

func TestGetPage_MaxBlocks(t *testing.T) {
	mb := newMockBackend()
	blocks := make([]types.BlockEntity, 10)
	for i := range blocks {
		blocks[i] = types.BlockEntity{UUID: "b-" + string(rune('a'+i)), Content: "Block"}
	}
	mb.addPage(types.PageEntity{Name: "big-page"}, blocks...)
	nav := NewNavigate(mb)

	result, _, err := nav.GetPage(context.Background(), nil, types.GetPageInput{
		Name:      "big-page",
		MaxBlocks: 3,
	})
	if err != nil {
		t.Fatalf("GetPage() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("GetPage() returned error result")
	}

	var parsed map[string]any
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if parsed["truncated"] != true {
		t.Error("expected truncated = true when maxBlocks exceeded")
	}
}

func TestGetBlock_Success(t *testing.T) {
	mb := newMockBackend()
	mb.addBlock(types.BlockEntity{
		UUID:    "block-uuid-1",
		Content: "Test block [[SomeLink]]",
		Page:    &types.PageRef{Name: "test-page"},
	})
	// Set up a query result for the ancestor lookup.
	mb.queryResults[`[:find (pull ?parent [:block/uuid :block/content])
		:where
		[?b :block/uuid #uuid "block-uuid-1"]
		[?b :block/parent ?parent]]`] = json.RawMessage(`[[{"uuid":"parent-uuid","content":"parent block"}]]`)

	nav := NewNavigate(mb)

	result, _, err := nav.GetBlock(context.Background(), nil, types.GetBlockInput{
		UUID:             "block-uuid-1",
		IncludeAncestors: true,
	})
	if err != nil {
		t.Fatalf("GetBlock() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("GetBlock() returned error result")
	}

	var parsed map[string]any
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if parsed["content"] != "Test block [[SomeLink]]" {
		t.Errorf("content = %v, want %q", parsed["content"], "Test block [[SomeLink]]")
	}
}

func TestGetBlock_NotFound(t *testing.T) {
	mb := newMockBackend()
	nav := NewNavigate(mb)

	result, _, err := nav.GetBlock(context.Background(), nil, types.GetBlockInput{
		UUID: "nonexistent",
	})
	if err != nil {
		t.Fatalf("GetBlock() error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for nonexistent block")
	}
}

func TestGetLinks_Both(t *testing.T) {
	mb := newMockBackend()
	mb.addPage(types.PageEntity{Name: "link-page"}, types.BlockEntity{
		UUID:    "b-1",
		Content: "Links to [[Target1]] and [[Target2]]",
	})
	nav := NewNavigate(mb)

	result, _, err := nav.GetLinks(context.Background(), nil, types.GetLinksInput{
		Name: "link-page",
	})
	if err != nil {
		t.Fatalf("GetLinks() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("GetLinks() returned error result")
	}

	var parsed map[string]any
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if parsed["page"] != "link-page" {
		t.Errorf("page = %v, want %q", parsed["page"], "link-page")
	}

	outgoing, ok := parsed["outgoingLinks"].([]any)
	if !ok {
		t.Fatal("expected outgoingLinks array")
	}
	if len(outgoing) != 2 {
		t.Errorf("expected 2 outgoing links, got %d", len(outgoing))
	}
}

func TestGetLinks_ForwardOnly(t *testing.T) {
	mb := newMockBackend()
	mb.addPage(types.PageEntity{Name: "fwd-page"}, types.BlockEntity{
		UUID:    "b-1",
		Content: "Links to [[Other]]",
	})
	nav := NewNavigate(mb)

	result, _, err := nav.GetLinks(context.Background(), nil, types.GetLinksInput{
		Name:      "fwd-page",
		Direction: "forward",
	})
	if err != nil {
		t.Fatalf("GetLinks() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("GetLinks() returned error result")
	}

	var parsed map[string]any
	text := extractText(t, result)
	_ = json.Unmarshal([]byte(text), &parsed)

	if parsed["backlinks"] != nil {
		t.Error("expected no backlinks for forward-only direction")
	}
}

func TestGetReferences_Success(t *testing.T) {
	mb := newMockBackend()
	// Set up query result for references.
	query := `[:find (pull ?b [:block/uuid :block/content {:block/page [:block/name]}])
		:where
		[?b :block/refs ?ref]
		[?ref :block/uuid #uuid "target-uuid"]]`
	mb.queryResults[query] = json.RawMessage(`[[{"uuid":"ref-1","content":"references ((target-uuid))","page":{"name":"ref-page"}}]]`)

	nav := NewNavigate(mb)

	result, _, err := nav.GetReferences(context.Background(), nil, types.GetReferencesInput{
		UUID: "target-uuid",
	})
	if err != nil {
		t.Fatalf("GetReferences() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("GetReferences() returned error result")
	}
}

func TestGetReferences_QueryError(t *testing.T) {
	mb := newMockBackend()
	mb.queryErr = json.Unmarshal([]byte(`invalid`), &struct{}{})
	nav := NewNavigate(mb)

	result, _, err := nav.GetReferences(context.Background(), nil, types.GetReferencesInput{
		UUID: "target-uuid",
	})
	if err != nil {
		t.Fatalf("GetReferences() error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when query fails")
	}
}

func TestTraverse_PathFound(t *testing.T) {
	mb := newMockBackend()
	// from-page → [[middle]] → [[to-page]]
	mb.addPage(types.PageEntity{Name: "from-page"}, types.BlockEntity{
		UUID:    "b-1",
		Content: "Link to [[middle]]",
	})
	mb.addPage(types.PageEntity{Name: "middle"}, types.BlockEntity{
		UUID:    "b-2",
		Content: "Link to [[to-page]]",
	})
	mb.addPage(types.PageEntity{Name: "to-page"})
	nav := NewNavigate(mb)

	result, _, err := nav.Traverse(context.Background(), nil, types.TraverseInput{
		From:    "from-page",
		To:      "to-page",
		MaxHops: 4,
	})
	if err != nil {
		t.Fatalf("Traverse() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Traverse() returned error result")
	}

	var parsed map[string]any
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if parsed["pathsFound"] == nil {
		t.Fatal("expected pathsFound in result")
	}
	pathCount, _ := parsed["pathsFound"].(float64)
	if pathCount < 1 {
		t.Errorf("pathsFound = %v, want >= 1", pathCount)
	}
}

func TestTraverse_NoPath(t *testing.T) {
	mb := newMockBackend()
	mb.addPage(types.PageEntity{Name: "island-a"})
	mb.addPage(types.PageEntity{Name: "island-b"})
	nav := NewNavigate(mb)

	result, _, err := nav.Traverse(context.Background(), nil, types.TraverseInput{
		From: "island-a",
		To:   "island-b",
	})
	if err != nil {
		t.Fatalf("Traverse() error: %v", err)
	}
	// No error result, just a text message about no path found.
	if result.IsError {
		t.Fatal("no path is not an error, just a text response")
	}
}

func TestListPages_DefaultParams(t *testing.T) {
	mb := newMockBackend()
	mb.addPage(types.PageEntity{Name: "alpha", OriginalName: "Alpha"})
	mb.addPage(types.PageEntity{Name: "beta", OriginalName: "Beta"})
	mb.addPage(types.PageEntity{Name: "gamma", OriginalName: "Gamma"})
	nav := NewNavigate(mb)

	result, _, err := nav.ListPages(context.Background(), nil, types.ListPagesInput{})
	if err != nil {
		t.Fatalf("ListPages() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("ListPages() returned error result")
	}

	var parsed []map[string]any
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	// Should return all 3 pages sorted by name.
	if len(parsed) != 3 {
		t.Fatalf("expected 3 pages, got %d", len(parsed))
	}
	if parsed[0]["name"] != "Alpha" {
		t.Errorf("first page name = %v, want %q", parsed[0]["name"], "Alpha")
	}
	if parsed[1]["name"] != "Beta" {
		t.Errorf("second page name = %v, want %q", parsed[1]["name"], "Beta")
	}
	if parsed[2]["name"] != "Gamma" {
		t.Errorf("third page name = %v, want %q", parsed[2]["name"], "Gamma")
	}
}

func TestListPages_WithTagFilter(t *testing.T) {
	mb := newMockBackend()
	mb.addPage(types.PageEntity{Name: "tagged-page", OriginalName: "Tagged Page"},
		types.BlockEntity{UUID: "b1", Content: "Some content #project"},
	)
	mb.addPage(types.PageEntity{Name: "untagged-page", OriginalName: "Untagged Page"},
		types.BlockEntity{UUID: "b2", Content: "No relevant tags here"},
	)
	nav := NewNavigate(mb)

	result, _, err := nav.ListPages(context.Background(), nil, types.ListPagesInput{
		HasTag: "project",
	})
	if err != nil {
		t.Fatalf("ListPages() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("ListPages() returned error result")
	}

	var parsed []map[string]any
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	// Only the tagged page should be returned.
	if len(parsed) != 1 {
		t.Fatalf("expected 1 tagged page, got %d", len(parsed))
	}
	if parsed[0]["name"] != "Tagged Page" {
		t.Errorf("page name = %v, want %q", parsed[0]["name"], "Tagged Page")
	}
}

func TestListPages_WithLimit(t *testing.T) {
	mb := newMockBackend()
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("page-%02d", i)
		mb.addPage(types.PageEntity{Name: name, OriginalName: name})
	}
	nav := NewNavigate(mb)

	result, _, err := nav.ListPages(context.Background(), nil, types.ListPagesInput{
		Limit: 3,
	})
	if err != nil {
		t.Fatalf("ListPages() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("ListPages() returned error result")
	}

	var parsed []map[string]any
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if len(parsed) != 3 {
		t.Errorf("expected 3 pages with limit=3, got %d", len(parsed))
	}
}

func TestListPages_WithNamespace(t *testing.T) {
	mb := newMockBackend()
	mb.addPage(types.PageEntity{Name: "projects/alpha", OriginalName: "projects/alpha"})
	mb.addPage(types.PageEntity{Name: "projects/beta", OriginalName: "projects/beta"})
	mb.addPage(types.PageEntity{Name: "notes/gamma", OriginalName: "notes/gamma"})
	nav := NewNavigate(mb)

	result, _, err := nav.ListPages(context.Background(), nil, types.ListPagesInput{
		Namespace: "projects/",
	})
	if err != nil {
		t.Fatalf("ListPages() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("ListPages() returned error result")
	}

	var parsed []map[string]any
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if len(parsed) != 2 {
		t.Fatalf("expected 2 pages in projects/ namespace, got %d", len(parsed))
	}
}

func TestListPages_WithPropertyFilter(t *testing.T) {
	mb := newMockBackend()
	mb.addPage(types.PageEntity{
		Name:         "typed-page",
		OriginalName: "typed-page",
		Properties:   map[string]any{"type": "analysis"},
	})
	mb.addPage(types.PageEntity{
		Name:         "untyped-page",
		OriginalName: "untyped-page",
	})
	nav := NewNavigate(mb)

	result, _, err := nav.ListPages(context.Background(), nil, types.ListPagesInput{
		HasProperty: "type",
	})
	if err != nil {
		t.Fatalf("ListPages() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("ListPages() returned error result")
	}

	var parsed []map[string]any
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if len(parsed) != 1 {
		t.Fatalf("expected 1 page with 'type' property, got %d", len(parsed))
	}
	if parsed[0]["name"] != "typed-page" {
		t.Errorf("page name = %v, want %q", parsed[0]["name"], "typed-page")
	}
}

func TestListPages_FiltersEmptyNames(t *testing.T) {
	mb := newMockBackend()
	mb.addPage(types.PageEntity{Name: "valid-page", OriginalName: "Valid Page"})
	mb.addPage(types.PageEntity{Name: "", OriginalName: ""}) // invalid entry
	nav := NewNavigate(mb)

	result, _, err := nav.ListPages(context.Background(), nil, types.ListPagesInput{})
	if err != nil {
		t.Fatalf("ListPages() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("ListPages() returned error result")
	}

	var parsed []map[string]any
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	// Empty-name pages should be filtered out.
	if len(parsed) != 1 {
		t.Fatalf("expected 1 page (empty name filtered), got %d", len(parsed))
	}
}

func TestListPages_GetAllPagesError(t *testing.T) {
	mb := newMockBackend()
	mb.getAllPagesErr = fmt.Errorf("backend unavailable")
	nav := NewNavigate(mb)

	result, _, err := nav.ListPages(context.Background(), nil, types.ListPagesInput{})
	if err != nil {
		t.Fatalf("ListPages() error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when GetAllPages fails")
	}
}

func TestListPages_IncludesUpdatedAt(t *testing.T) {
	mb := newMockBackend()
	mb.addPage(types.PageEntity{
		Name:         "recent-page",
		OriginalName: "Recent Page",
		UpdatedAt:    1700000000,
	})
	nav := NewNavigate(mb)

	result, _, err := nav.ListPages(context.Background(), nil, types.ListPagesInput{})
	if err != nil {
		t.Fatalf("ListPages() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("ListPages() returned error result")
	}

	var parsed []map[string]any
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if len(parsed) != 1 {
		t.Fatalf("expected 1 page, got %d", len(parsed))
	}
	if parsed[0]["updatedAt"] != float64(1700000000) {
		t.Errorf("updatedAt = %v, want %v", parsed[0]["updatedAt"], float64(1700000000))
	}
}
