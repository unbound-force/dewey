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

	// Verify top-level whiteboard structure fields.
	if parsed["name"] != "my-whiteboard" {
		t.Errorf("name = %v, want %q", parsed["name"], "my-whiteboard")
	}

	elementCount, ok := parsed["elementCount"].(float64)
	if !ok {
		t.Fatalf("elementCount missing or not a number, got %T: %v", parsed["elementCount"], parsed["elementCount"])
	}
	if elementCount != 3 {
		t.Errorf("elementCount = %v, want 3", elementCount)
	}

	// Verify elements array structure.
	elements, ok := parsed["elements"].([]any)
	if !ok {
		t.Fatalf("elements missing or not an array, got %T", parsed["elements"])
	}
	if len(elements) != 3 {
		t.Errorf("len(elements) = %d, want 3", len(elements))
	}

	// Verify element[0] has uuid, content, and shapeType fields.
	elem0, ok := elements[0].(map[string]any)
	if !ok {
		t.Fatalf("elements[0] not a map, got %T", elements[0])
	}
	if elem0["uuid"] != "wb-1" {
		t.Errorf("elements[0].uuid = %v, want %q", elem0["uuid"], "wb-1")
	}
	if elem0["content"] != "Text element [[PageRef]]" {
		t.Errorf("elements[0].content = %v, want %q", elem0["content"], "Text element [[PageRef]]")
	}
	if elem0["shapeType"] != "rectangle" {
		t.Errorf("elements[0].shapeType = %v, want %q", elem0["shapeType"], "rectangle")
	}

	// Verify element[0] detected the [[PageRef]] link.
	links0, ok := elem0["links"].([]any)
	if !ok {
		t.Fatalf("elements[0].links missing or not an array")
	}
	foundPageRef := false
	for _, l := range links0 {
		if l == "PageRef" {
			foundPageRef = true
			break
		}
	}
	if !foundPageRef {
		t.Errorf("elements[0].links = %v, expected to contain 'PageRef'", links0)
	}

	// Should detect embedded page — verify specific page name.
	embeddedPages, ok := parsed["embeddedPages"].([]any)
	if !ok {
		t.Fatalf("embeddedPages missing or not an array")
	}
	if len(embeddedPages) < 1 {
		t.Fatal("expected at least 1 embedded page")
	}
	// Should contain "embedded-page" (from logseq.tldraw.page) and "PageRef" (from content links).
	embeddedSet := make(map[string]bool)
	for _, ep := range embeddedPages {
		if s, ok := ep.(string); ok {
			embeddedSet[s] = true
		}
	}
	if !embeddedSet["embedded-page"] {
		t.Errorf("embeddedPages should contain 'embedded-page', got %v", embeddedPages)
	}
	if !embeddedSet["PageRef"] {
		t.Errorf("embeddedPages should contain 'PageRef', got %v", embeddedPages)
	}

	// Verify connections — source/target relationship.
	connections, ok := parsed["connections"].([]any)
	if !ok {
		t.Fatalf("connections missing or not an array")
	}
	if len(connections) != 1 {
		t.Errorf("expected 1 connection, got %d", len(connections))
	}
	if len(connections) > 0 {
		conn0, ok := connections[0].(map[string]any)
		if !ok {
			t.Fatalf("connections[0] not a map")
		}
		if conn0["source"] != "wb-1" {
			t.Errorf("connection source = %v, want %q", conn0["source"], "wb-1")
		}
		if conn0["target"] != "wb-3" {
			t.Errorf("connection target = %v, want %q", conn0["target"], "wb-3")
		}
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
