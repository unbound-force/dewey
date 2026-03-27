package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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
		Content: "Another block with [[Second]]",
	})
	// Set up backlinks so the backlinks field is populated.
	mb.linkedRefs["test-page"] = json.RawMessage(`[
		[
			{"name": "referrer-page", "originalName": "Referrer Page"},
			[{"uuid": "ref-b1", "content": "See [[test-page]]"}]
		]
	]`)
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

	// --- Assert all required top-level keys exist ---
	requiredKeys := []string{"page", "outgoingLinks", "backlinks", "linkCount", "blocks", "blockCount"}
	for _, key := range requiredKeys {
		if _, ok := parsed[key]; !ok {
			t.Errorf("GetPage() result missing required key %q", key)
		}
	}

	// --- Assert page field contains expected data ---
	pageData, ok := parsed["page"].(map[string]any)
	if !ok {
		t.Fatalf("page is not an object, got %T", parsed["page"])
	}
	if pageData["name"] != "test-page" {
		t.Errorf("page.name = %v, want %q", pageData["name"], "test-page")
	}
	if pageData["originalName"] != "Test Page" {
		t.Errorf("page.originalName = %v, want %q", pageData["originalName"], "Test Page")
	}

	// --- Assert blockCount is present and correct ---
	blockCount, ok := parsed["blockCount"].(float64)
	if !ok {
		t.Fatalf("blockCount is not a number, got %T", parsed["blockCount"])
	}
	if int(blockCount) != 2 {
		t.Errorf("blockCount = %d, want 2", int(blockCount))
	}

	// --- Assert outgoingLinks contains expected link names from mock blocks ---
	links, ok := parsed["outgoingLinks"].([]any)
	if !ok {
		t.Fatal("expected outgoingLinks to be an array")
	}
	wantLinks := map[string]bool{"World": false, "Second": false}
	for _, link := range links {
		linkStr, ok := link.(string)
		if !ok {
			t.Errorf("outgoingLinks entry is not a string, got %T", link)
			continue
		}
		if _, expected := wantLinks[linkStr]; expected {
			wantLinks[linkStr] = true
		}
	}
	for name, found := range wantLinks {
		if !found {
			t.Errorf("outgoingLinks missing expected link %q", name)
		}
	}

	// --- Assert backlinks field exists and has correct structure ---
	backlinks, ok := parsed["backlinks"].([]any)
	if !ok {
		t.Fatalf("backlinks is not an array, got %T", parsed["backlinks"])
	}
	if len(backlinks) != 1 {
		t.Fatalf("backlinks has %d entries, want 1", len(backlinks))
	}
	bl, ok := backlinks[0].(map[string]any)
	if !ok {
		t.Fatalf("backlinks[0] is not an object, got %T", backlinks[0])
	}
	if bl["pageName"] != "Referrer Page" {
		t.Errorf("backlinks[0].pageName = %v, want %q", bl["pageName"], "Referrer Page")
	}
	blBlocks, ok := bl["blocks"].([]any)
	if !ok {
		t.Fatalf("backlinks[0].blocks is not an array, got %T", bl["blocks"])
	}
	if len(blBlocks) != 1 {
		t.Errorf("backlinks[0].blocks has %d entries, want 1", len(blBlocks))
	}

	// --- Assert linkCount = outgoing + backlinks ---
	linkCount, ok := parsed["linkCount"].(float64)
	if !ok {
		t.Fatalf("linkCount is not a number, got %T", parsed["linkCount"])
	}
	expectedLinkCount := len(links) + len(backlinks)
	if int(linkCount) != expectedLinkCount {
		t.Errorf("linkCount = %d, want %d (outgoing=%d + backlinks=%d)",
			int(linkCount), expectedLinkCount, len(links), len(backlinks))
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
	text := extractText(t, result)
	if !strings.Contains(text, "page not found") {
		t.Errorf("error message should contain 'page not found', got: %s", text)
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
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}

	// Assert blockCount matches the number of compact blocks.
	blockCount, ok := parsed["blockCount"].(float64)
	if !ok {
		t.Fatalf("blockCount is not a number, got %T", parsed["blockCount"])
	}
	if int(blockCount) != 2 {
		t.Errorf("blockCount = %d, want 2 in compact mode", int(blockCount))
	}

	// Assert each compact block entry has uuid and content fields.
	for i, b := range blocks {
		entry, ok := b.(map[string]any)
		if !ok {
			t.Fatalf("blocks[%d] is not an object, got %T", i, b)
		}
		if _, hasUUID := entry["uuid"]; !hasUUID {
			t.Errorf("compact blocks[%d] missing 'uuid' field", i)
		}
		if _, hasContent := entry["content"]; !hasContent {
			t.Errorf("compact blocks[%d] missing 'content' field", i)
		}
	}
}

func TestGetPage_MaxBlocks(t *testing.T) {
	mb := newMockBackend()
	blocks := make([]types.BlockEntity, 10)
	for i := range blocks {
		blocks[i] = types.BlockEntity{UUID: fmt.Sprintf("b-%d", i), Content: fmt.Sprintf("Block %d", i)}
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

	// Assert truncated flag is set.
	if parsed["truncated"] != true {
		t.Error("expected truncated = true when maxBlocks exceeded")
	}

	// Assert totalBlocks reports the original count before truncation.
	totalBlocks, ok := parsed["totalBlocks"].(float64)
	if !ok {
		t.Fatalf("totalBlocks is not a number, got %T", parsed["totalBlocks"])
	}
	if int(totalBlocks) != 10 {
		t.Errorf("totalBlocks = %d, want 10", int(totalBlocks))
	}

	// Assert blockCount reflects the truncated count.
	blockCount, ok := parsed["blockCount"].(float64)
	if !ok {
		t.Fatalf("blockCount is not a number, got %T", parsed["blockCount"])
	}
	if int(blockCount) > 3 {
		t.Errorf("blockCount = %d, want <= 3 (maxBlocks limit)", int(blockCount))
	}
}

func TestGetBlock_Success(t *testing.T) {
	mb := newMockBackend()
	mb.addBlock(types.BlockEntity{
		UUID:    "block-uuid-1",
		Content: "Test block [[SomeLink]]",
		Page:    &types.PageRef{Name: "test-page"},
		Children: []types.BlockEntity{
			{UUID: "child-a", Content: "Child A content"},
			{UUID: "child-b", Content: "Child B content"},
		},
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

	var data map[string]any
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	// --- Assert block content matches what was returned by mock ---
	if data["content"] != "Test block [[SomeLink]]" {
		t.Errorf("content = %v, want %q", data["content"], "Test block [[SomeLink]]")
	}

	// --- Assert uuid matches ---
	if data["uuid"] != "block-uuid-1" {
		t.Errorf("uuid = %v, want %q", data["uuid"], "block-uuid-1")
	}

	// --- Assert children field is present with correct entries ---
	children, ok := data["children"].([]any)
	if !ok {
		t.Fatalf("children is not an array, got %T (value: %v)", data["children"], data["children"])
	}
	if len(children) != 2 {
		t.Fatalf("children has %d entries, want 2", len(children))
	}
	child0, ok := children[0].(map[string]any)
	if !ok {
		t.Fatalf("children[0] is not an object, got %T", children[0])
	}
	if child0["uuid"] != "child-a" {
		t.Errorf("children[0].uuid = %v, want %q", child0["uuid"], "child-a")
	}
	if child0["content"] != "Child A content" {
		t.Errorf("children[0].content = %v, want %q", child0["content"], "Child A content")
	}
	child1, ok := children[1].(map[string]any)
	if !ok {
		t.Fatalf("children[1] is not an object, got %T", children[1])
	}
	if child1["uuid"] != "child-b" {
		t.Errorf("children[1].uuid = %v, want %q", child1["uuid"], "child-b")
	}

	// --- Assert parsed field is present with expected structure ---
	parsedField, ok := data["parsed"].(map[string]any)
	if !ok {
		t.Fatalf("parsed is not an object, got %T", data["parsed"])
	}
	if parsedField["raw"] != "Test block [[SomeLink]]" {
		t.Errorf("parsed.raw = %v, want %q", parsedField["raw"], "Test block [[SomeLink]]")
	}
	// parsed.links should contain "SomeLink" from [[SomeLink]].
	parsedLinks, ok := parsedField["links"].([]any)
	if !ok {
		t.Fatalf("parsed.links is not an array, got %T", parsedField["links"])
	}
	foundLink := false
	for _, l := range parsedLinks {
		if l == "SomeLink" {
			foundLink = true
			break
		}
	}
	if !foundLink {
		t.Errorf("parsed.links = %v, want to contain %q", parsedLinks, "SomeLink")
	}

	// --- Assert ancestors field is present when IncludeAncestors=true ---
	ancestors, ok := data["ancestors"].([]any)
	if !ok {
		t.Fatalf("ancestors is not an array, got %T", data["ancestors"])
	}
	if len(ancestors) != 1 {
		t.Fatalf("ancestors has %d entries, want 1", len(ancestors))
	}
	ancestor, ok := ancestors[0].(map[string]any)
	if !ok {
		t.Fatalf("ancestors[0] is not an object, got %T", ancestors[0])
	}
	if ancestor["uuid"] != "parent-uuid" {
		t.Errorf("ancestors[0].uuid = %v, want %q", ancestor["uuid"], "parent-uuid")
	}
	if ancestor["content"] != "parent block" {
		t.Errorf("ancestors[0].content = %v, want %q", ancestor["content"], "parent block")
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
	text := extractText(t, result)
	if !strings.Contains(text, "block not found") {
		t.Errorf("error message should contain 'block not found', got: %s", text)
	}
}

func TestGetBlock_LeafBlock_NoChildren(t *testing.T) {
	mb := newMockBackend()
	mb.addBlock(types.BlockEntity{
		UUID:    "leaf-uuid",
		Content: "A leaf block with no children",
	})

	nav := NewNavigate(mb)
	result, _, err := nav.GetBlock(context.Background(), nil, types.GetBlockInput{
		UUID: "leaf-uuid",
	})
	if err != nil {
		t.Fatalf("GetBlock() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("GetBlock() returned error result")
	}

	var data map[string]any
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	// Content should match mock.
	if data["content"] != "A leaf block with no children" {
		t.Errorf("content = %v, want %q", data["content"], "A leaf block with no children")
	}

	// Children should be absent or null for a leaf block (omitempty in JSON tags).
	if children, exists := data["children"]; exists && children != nil {
		childArr, ok := children.([]any)
		if ok && len(childArr) > 0 {
			t.Errorf("leaf block should have no children, got %d", len(childArr))
		}
	}

	// parsed field should still be present even for leaf blocks.
	if _, ok := data["parsed"].(map[string]any); !ok {
		t.Errorf("parsed field missing or not an object, got %T", data["parsed"])
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
	mb.addPage(types.PageEntity{
		Name:         "alpha",
		OriginalName: "Alpha",
		Properties:   map[string]any{"status": "active"},
		UpdatedAt:    1700000000,
	})
	mb.addPage(types.PageEntity{
		Name:         "beta",
		OriginalName: "Beta",
		Journal:      true,
		UpdatedAt:    1700000100,
	})
	mb.addPage(types.PageEntity{
		Name:         "gamma",
		OriginalName: "Gamma",
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

	// --- Assert pages count matches expected ---
	if len(parsed) != 3 {
		t.Fatalf("expected 3 pages, got %d", len(parsed))
	}

	// --- Assert each page entry has required fields with correct types ---
	for i, page := range parsed {
		// name field must be present and be a string.
		nameVal, ok := page["name"]
		if !ok {
			t.Errorf("pages[%d] missing 'name' field", i)
		} else if _, isStr := nameVal.(string); !isStr {
			t.Errorf("pages[%d].name is not a string, got %T", i, nameVal)
		}

		// journal field must be present (boolean).
		if _, ok := page["journal"]; !ok {
			t.Errorf("pages[%d] missing 'journal' field", i)
		}

		// properties field must be present (may be null).
		if _, ok := page["properties"]; !ok {
			t.Errorf("pages[%d] missing 'properties' field", i)
		}
	}

	// --- Assert pages are sorted by name (default sort) ---
	wantOrder := []string{"Alpha", "Beta", "Gamma"}
	for i, wantName := range wantOrder {
		if parsed[i]["name"] != wantName {
			t.Errorf("pages[%d].name = %v, want %q (sorted by name)", i, parsed[i]["name"], wantName)
		}
	}

	// --- Assert journal field values ---
	if parsed[0]["journal"] != false {
		t.Errorf("pages[0] (Alpha) journal = %v, want false", parsed[0]["journal"])
	}
	if parsed[1]["journal"] != true {
		t.Errorf("pages[1] (Beta) journal = %v, want true", parsed[1]["journal"])
	}

	// --- Assert properties are passed through ---
	props, ok := parsed[0]["properties"].(map[string]any)
	if !ok {
		t.Fatalf("pages[0].properties is not an object, got %T", parsed[0]["properties"])
	}
	if props["status"] != "active" {
		t.Errorf("pages[0].properties.status = %v, want %q", props["status"], "active")
	}

	// --- Assert updatedAt is present only when > 0 ---
	if _, ok := parsed[0]["updatedAt"]; !ok {
		t.Error("pages[0] (Alpha) should have 'updatedAt' since UpdatedAt > 0")
	}
	if _, ok := parsed[2]["updatedAt"]; ok {
		t.Error("pages[2] (Gamma) should not have 'updatedAt' since UpdatedAt is 0")
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
	text := extractText(t, result)
	if !strings.Contains(text, "failed to list pages") {
		t.Errorf("error message should contain 'failed to list pages', got: %s", text)
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
