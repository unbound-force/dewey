package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/unbound-force/dewey/types"
)

func TestGetWhiteboard_Success(t *testing.T) {
	mb := newMockBackend()
	mb.addPage(types.PageEntity{Name: "my-whiteboard"},
		types.BlockEntity{
			UUID:    "wb-1",
			Content: "Text element [[PageRef]]",
			Properties: map[string]any{
				"ls-type": "rectangle",
			},
		},
		types.BlockEntity{
			UUID:    "wb-2",
			Content: "Another element",
			Properties: map[string]any{
				"ls-type":              "line",
				"logseq.tldraw.source": "wb-1",
				"logseq.tldraw.target": "wb-3",
			},
		},
		types.BlockEntity{
			UUID:    "wb-3",
			Content: "",
			Properties: map[string]any{
				"ls-type":            "rectangle",
				"logseq.tldraw.page": "embedded-page",
			},
		},
	)

	w := NewWhiteboard(mb)

	result, _, err := w.GetWhiteboard(context.Background(), nil, types.GetWhiteboardInput{
		Name: "my-whiteboard",
	})
	if err != nil {
		t.Fatalf("GetWhiteboard() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("GetWhiteboard() returned error result")
	}

	var parsed map[string]any
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	// --- Assert: name field ---
	if parsed["name"] != "my-whiteboard" {
		t.Errorf("parsed[\"name\"] = %v, want %q", parsed["name"], "my-whiteboard")
	}

	// --- Assert: elementCount field ---
	elementCount, ok := parsed["elementCount"].(float64)
	if !ok {
		t.Fatalf("parsed[\"elementCount\"] missing or not a number, got %T: %v", parsed["elementCount"], parsed["elementCount"])
	}
	if elementCount != 3 {
		t.Errorf("parsed[\"elementCount\"] = %v, want 3", elementCount)
	}

	// --- Assert: elements field ---
	elements, ok := parsed["elements"].([]any)
	if !ok {
		t.Fatalf("parsed[\"elements\"] missing or not a []any, got %T", parsed["elements"])
	}
	if len(elements) != 3 {
		t.Fatalf("len(parsed[\"elements\"]) = %d, want 3", len(elements))
	}

	// elementCount must match len(elements) — cross-field consistency.
	if int(elementCount) != len(elements) {
		t.Errorf("parsed[\"elementCount\"] = %v, but len(parsed[\"elements\"]) = %d — mismatch", elementCount, len(elements))
	}

	// --- Assert: elements[0] — rectangle with content link ---
	elem0, ok := elements[0].(map[string]any)
	if !ok {
		t.Fatalf("elements[0] not a map[string]any, got %T", elements[0])
	}
	if elem0["uuid"] != "wb-1" {
		t.Errorf("elements[0][\"uuid\"] = %v, want %q", elem0["uuid"], "wb-1")
	}
	if elem0["content"] != "Text element [[PageRef]]" {
		t.Errorf("elements[0][\"content\"] = %v, want %q", elem0["content"], "Text element [[PageRef]]")
	}
	if elem0["shapeType"] != "rectangle" {
		t.Errorf("elements[0][\"shapeType\"] = %v, want %q", elem0["shapeType"], "rectangle")
	}
	if _, hasProps0 := elem0["properties"]; !hasProps0 {
		t.Errorf("elements[0][\"properties\"] missing — expected properties map")
	}

	// Assert: elements[0].links contains "PageRef" via direct index access.
	links0, ok := elem0["links"].([]any)
	if !ok {
		t.Fatalf("elements[0][\"links\"] missing or not a []any, got %T", elem0["links"])
	}
	if len(links0) < 1 {
		t.Fatalf("len(elements[0][\"links\"]) = 0, want at least 1")
	}
	if links0[0] != "PageRef" {
		t.Errorf("elements[0][\"links\"][0] = %v, want %q", links0[0], "PageRef")
	}

	// --- Assert: elements[1] — line connector ---
	elem1, ok := elements[1].(map[string]any)
	if !ok {
		t.Fatalf("elements[1] not a map[string]any, got %T", elements[1])
	}
	if elem1["uuid"] != "wb-2" {
		t.Errorf("elements[1][\"uuid\"] = %v, want %q", elem1["uuid"], "wb-2")
	}
	if elem1["content"] != "Another element" {
		t.Errorf("elements[1][\"content\"] = %v, want %q", elem1["content"], "Another element")
	}
	if elem1["shapeType"] != "line" {
		t.Errorf("elements[1][\"shapeType\"] = %v, want %q", elem1["shapeType"], "line")
	}
	if _, hasProps1 := elem1["properties"]; !hasProps1 {
		t.Errorf("elements[1][\"properties\"] missing — expected properties map")
	}

	// --- Assert: elements[2] — rectangle with embedded page ---
	elem2, ok := elements[2].(map[string]any)
	if !ok {
		t.Fatalf("elements[2] not a map[string]any, got %T", elements[2])
	}
	if elem2["uuid"] != "wb-3" {
		t.Errorf("elements[2][\"uuid\"] = %v, want %q", elem2["uuid"], "wb-3")
	}
	if elem2["shapeType"] != "rectangle" {
		t.Errorf("elements[2][\"shapeType\"] = %v, want %q", elem2["shapeType"], "rectangle")
	}
	if _, hasProps2 := elem2["properties"]; !hasProps2 {
		t.Errorf("elements[2][\"properties\"] missing — expected properties map")
	}
	if elem2["embeddedPage"] != "embedded-page" {
		t.Errorf("elements[2][\"embeddedPage\"] = %v, want %q", elem2["embeddedPage"], "embedded-page")
	}

	// --- Assert: embeddedPages field ---
	// Production code collects pages from logseq.tldraw.page properties and content [[links]].
	// Expected order: "PageRef" (from elem0 content link), "embedded-page" (from elem2 property).
	embeddedPages, ok := parsed["embeddedPages"].([]any)
	if !ok {
		t.Fatalf("parsed[\"embeddedPages\"] missing or not a []any, got %T", parsed["embeddedPages"])
	}
	if len(embeddedPages) != 2 {
		t.Fatalf("len(parsed[\"embeddedPages\"]) = %d, want 2", len(embeddedPages))
	}
	// Verify each embedded page name is a string with the expected value.
	ep0, ok := embeddedPages[0].(string)
	if !ok {
		t.Fatalf("parsed[\"embeddedPages\"][0] not a string, got %T", embeddedPages[0])
	}
	if ep0 != "PageRef" {
		t.Errorf("parsed[\"embeddedPages\"][0] = %q, want %q", ep0, "PageRef")
	}
	ep1, ok := embeddedPages[1].(string)
	if !ok {
		t.Fatalf("parsed[\"embeddedPages\"][1] not a string, got %T", embeddedPages[1])
	}
	if ep1 != "embedded-page" {
		t.Errorf("parsed[\"embeddedPages\"][1] = %q, want %q", ep1, "embedded-page")
	}

	// --- Assert: connections field ---
	// Production code extracts connections from blocks with both source and target properties.
	connections, ok := parsed["connections"].([]any)
	if !ok {
		t.Fatalf("parsed[\"connections\"] missing or not a []any, got %T", parsed["connections"])
	}
	if len(connections) != 1 {
		t.Fatalf("len(parsed[\"connections\"]) = %d, want 1", len(connections))
	}
	conn0, ok := connections[0].(map[string]any)
	if !ok {
		t.Fatalf("parsed[\"connections\"][0] not a map[string]any, got %T", connections[0])
	}
	conn0Source, ok := conn0["source"].(string)
	if !ok {
		t.Fatalf("parsed[\"connections\"][0][\"source\"] not a string, got %T", conn0["source"])
	}
	if conn0Source != "wb-1" {
		t.Errorf("parsed[\"connections\"][0][\"source\"] = %q, want %q", conn0Source, "wb-1")
	}
	conn0Target, ok := conn0["target"].(string)
	if !ok {
		t.Fatalf("parsed[\"connections\"][0][\"target\"] not a string, got %T", conn0["target"])
	}
	if conn0Target != "wb-3" {
		t.Errorf("parsed[\"connections\"][0][\"target\"] = %q, want %q", conn0Target, "wb-3")
	}
}

func TestGetWhiteboard_NotFound(t *testing.T) {
	mb := newMockBackend()
	mb.getBlocksErr = fmt.Errorf("page not found")
	w := NewWhiteboard(mb)

	result, _, err := w.GetWhiteboard(context.Background(), nil, types.GetWhiteboardInput{
		Name: "nonexistent",
	})
	if err != nil {
		t.Fatalf("GetWhiteboard() error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for nonexistent whiteboard")
	}

	// Verify error message mentions the whiteboard name.
	text := extractText(t, result)
	if text == "" {
		t.Error("error result text should not be empty")
	}
	if !strings.Contains(text, "nonexistent") {
		t.Errorf("error message = %q, should mention whiteboard name 'nonexistent'", text)
	}
	if !strings.Contains(text, "not found") {
		t.Errorf("error message = %q, should mention 'not found'", text)
	}
}

func TestGetWhiteboard_EmptyBoard(t *testing.T) {
	mb := newMockBackend()
	mb.addPage(types.PageEntity{Name: "empty-wb"})
	w := NewWhiteboard(mb)

	result, _, err := w.GetWhiteboard(context.Background(), nil, types.GetWhiteboardInput{
		Name: "empty-wb",
	})
	if err != nil {
		t.Fatalf("GetWhiteboard() error: %v", err)
	}
	if result.IsError {
		t.Fatal("empty whiteboard should not be an error")
	}

	var parsed map[string]any
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	// Verify structural fields exist even for empty whiteboard.
	if parsed["name"] != "empty-wb" {
		t.Errorf("name = %v, want %q", parsed["name"], "empty-wb")
	}
	if parsed["elementCount"] != float64(0) {
		t.Errorf("elementCount = %v, want 0", parsed["elementCount"])
	}

	// Elements should be null/nil (no blocks), but the key should exist.
	if _, exists := parsed["elements"]; !exists {
		t.Error("elements key should exist in response even for empty whiteboard")
	}

	// Connections should be null/nil for empty whiteboard.
	if _, exists := parsed["connections"]; !exists {
		t.Error("connections key should exist in response even for empty whiteboard")
	}

	// EmbeddedPages should be null/nil for empty whiteboard.
	if _, exists := parsed["embeddedPages"]; !exists {
		t.Error("embeddedPages key should exist in response even for empty whiteboard")
	}
}

func TestListWhiteboards_ViaDataScript(t *testing.T) {
	mb := newMockBackend()
	query := `[:find (pull ?p [:block/uuid :block/name :block/original-name
	                           :block/created-at :block/updated-at])
		:where
		[?p :block/name]
		[?p :block/type "whiteboard"]]`
	mb.queryResults[query] = json.RawMessage(`[[{"uuid":"wb-uuid","name":"my-board","original-name":"My Board","created-at":1000,"updated-at":2000}]]`)

	w := NewWhiteboard(mb)

	result, _, err := w.ListWhiteboards(context.Background(), nil, types.ListWhiteboardsInput{})
	if err != nil {
		t.Fatalf("ListWhiteboards() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("ListWhiteboards() returned error result")
	}

	var parsed map[string]any
	text := extractText(t, result)
	_ = json.Unmarshal([]byte(text), &parsed)

	if parsed["count"] != float64(1) {
		t.Errorf("count = %v, want 1", parsed["count"])
	}
}

func TestListWhiteboards_Fallback(t *testing.T) {
	mb := newMockBackend()
	mb.queryErr = fmt.Errorf("query not supported")
	// Fallback: scan pages by file path.
	mb.addPage(types.PageEntity{
		Name:         "wb-page",
		OriginalName: "WB Page",
		UUID:         "wb-uuid",
		File: &types.FileInfo{
			Path: "whiteboards/wb-page.edn",
		},
	})
	mb.addPage(types.PageEntity{
		Name:         "normal-page",
		OriginalName: "Normal Page",
	})

	w := NewWhiteboard(mb)

	result, _, err := w.ListWhiteboards(context.Background(), nil, types.ListWhiteboardsInput{})
	if err != nil {
		t.Fatalf("ListWhiteboards() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("ListWhiteboards() fallback returned error result")
	}

	var parsed map[string]any
	text := extractText(t, result)
	_ = json.Unmarshal([]byte(text), &parsed)

	if parsed["count"] != float64(1) {
		t.Errorf("count = %v, want 1 (only whiteboards/ path)", parsed["count"])
	}
}

func TestListWhiteboards_NoWhiteboards(t *testing.T) {
	mb := newMockBackend()
	// DataScript returns empty, fallback also returns empty.
	query := `[:find (pull ?p [:block/uuid :block/name :block/original-name
	                           :block/created-at :block/updated-at])
		:where
		[?p :block/name]
		[?p :block/type "whiteboard"]]`
	mb.queryResults[query] = json.RawMessage(`[]`)
	// Fallback: no pages match whiteboards/ path.
	mb.addPage(types.PageEntity{Name: "regular-page", OriginalName: "Regular"})

	w := NewWhiteboard(mb)

	result, _, err := w.ListWhiteboards(context.Background(), nil, types.ListWhiteboardsInput{})
	if err != nil {
		t.Fatalf("ListWhiteboards() error: %v", err)
	}
	// Not an error, just a text message.
	if result.IsError {
		t.Fatal("no whiteboards is not an error")
	}
}
