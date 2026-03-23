package tools

import (
	"context"
	"encoding/json"
	"fmt"
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

	if parsed["name"] != "my-whiteboard" {
		t.Errorf("name = %v, want %q", parsed["name"], "my-whiteboard")
	}

	elementCount := parsed["elementCount"].(float64)
	if elementCount != 3 {
		t.Errorf("elementCount = %v, want 3", elementCount)
	}

	// Should detect embedded page.
	embeddedPages := parsed["embeddedPages"].([]any)
	if len(embeddedPages) < 1 {
		t.Error("expected at least 1 embedded page")
	}

	// Should detect connection.
	connections := parsed["connections"].([]any)
	if len(connections) != 1 {
		t.Errorf("expected 1 connection, got %d", len(connections))
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
	json.Unmarshal([]byte(text), &parsed)

	if parsed["elementCount"] != float64(0) {
		t.Errorf("elementCount = %v, want 0", parsed["elementCount"])
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
	json.Unmarshal([]byte(text), &parsed)

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
	json.Unmarshal([]byte(text), &parsed)

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
