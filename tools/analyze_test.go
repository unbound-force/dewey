package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/unbound-force/dewey/types"
)

func TestGraphOverview_Success(t *testing.T) {
	mb := newMockBackend()
	mb.addPage(types.PageEntity{Name: "page-a", OriginalName: "Page A"},
		types.BlockEntity{UUID: "b1", Content: "Link to [[page-b]]"},
	)
	mb.addPage(types.PageEntity{Name: "page-b", OriginalName: "Page B"},
		types.BlockEntity{UUID: "b2", Content: "Some content"},
	)
	a := NewAnalyze(mb)

	result, _, err := a.GraphOverview(context.Background(), nil, types.GraphOverviewInput{})
	if err != nil {
		t.Fatalf("GraphOverview() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("GraphOverview() returned error result")
	}

	var parsed map[string]any
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	// Should have totalPages.
	if parsed["totalPages"] == nil {
		t.Error("expected totalPages in overview")
	}
	totalPages, _ := parsed["totalPages"].(float64)
	if totalPages != 2 {
		t.Errorf("totalPages = %v, want 2", totalPages)
	}
}

func TestGraphOverview_Error(t *testing.T) {
	mb := newMockBackend()
	mb.getAllPagesErr = fmt.Errorf("backend down")
	a := NewAnalyze(mb)

	result, _, err := a.GraphOverview(context.Background(), nil, types.GraphOverviewInput{})
	if err != nil {
		t.Fatalf("GraphOverview() error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when backend fails")
	}
}

func TestFindConnections_Connected(t *testing.T) {
	mb := newMockBackend()
	mb.addPage(types.PageEntity{Name: "start", OriginalName: "Start"},
		types.BlockEntity{UUID: "b1", Content: "Links to [[end]]"},
	)
	mb.addPage(types.PageEntity{Name: "end", OriginalName: "End"},
		types.BlockEntity{UUID: "b2", Content: "end content"},
	)
	a := NewAnalyze(mb)

	result, _, err := a.FindConnections(context.Background(), nil, types.FindConnectionsInput{
		From: "start",
		To:   "end",
	})
	if err != nil {
		t.Fatalf("FindConnections() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("FindConnections() returned error result")
	}
}

func TestFindConnections_NotConnected(t *testing.T) {
	mb := newMockBackend()
	mb.addPage(types.PageEntity{Name: "island-a"},
		types.BlockEntity{UUID: "b1", Content: "no links"},
	)
	mb.addPage(types.PageEntity{Name: "island-b"},
		types.BlockEntity{UUID: "b2", Content: "also no links"},
	)
	a := NewAnalyze(mb)

	result, _, err := a.FindConnections(context.Background(), nil, types.FindConnectionsInput{
		From: "island-a",
		To:   "island-b",
	})
	if err != nil {
		t.Fatalf("FindConnections() error: %v", err)
	}
	// Not connected produces a text result, not an error.
	if result.IsError {
		t.Fatal("no connection is not an error, just a text result")
	}
}

func TestFindConnections_Error(t *testing.T) {
	mb := newMockBackend()
	mb.getAllPagesErr = fmt.Errorf("backend down")
	a := NewAnalyze(mb)

	result, _, err := a.FindConnections(context.Background(), nil, types.FindConnectionsInput{
		From: "a",
		To:   "b",
	})
	if err != nil {
		t.Fatalf("FindConnections() error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when backend fails")
	}
}

func TestKnowledgeGaps_Success(t *testing.T) {
	mb := newMockBackend()
	// Create an orphan page (no links in or out).
	mb.addPage(types.PageEntity{Name: "orphan", OriginalName: "Orphan"},
		types.BlockEntity{UUID: "b-orphan", Content: "no links here"},
	)
	// Create connected pages.
	mb.addPage(types.PageEntity{Name: "connected-a", OriginalName: "Connected A"},
		types.BlockEntity{UUID: "b-ca", Content: "Links to [[connected-b]]"},
	)
	mb.addPage(types.PageEntity{Name: "connected-b", OriginalName: "Connected B"},
		types.BlockEntity{UUID: "b-cb", Content: "Links to [[connected-a]]"},
	)
	a := NewAnalyze(mb)

	result, _, err := a.KnowledgeGaps(context.Background(), nil, types.KnowledgeGapsInput{})
	if err != nil {
		t.Fatalf("KnowledgeGaps() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("KnowledgeGaps() returned error result")
	}
}

func TestKnowledgeGaps_WithFilters(t *testing.T) {
	mb := newMockBackend()
	mb.addPage(types.PageEntity{Name: "real-orphan"},
		types.BlockEntity{UUID: "b1", Content: "has content"},
		types.BlockEntity{UUID: "b2", Content: "more content"},
	)
	mb.addPage(types.PageEntity{Name: "12345"},
		types.BlockEntity{UUID: "b3", Content: "numeric junk"},
	)
	a := NewAnalyze(mb)

	result, _, err := a.KnowledgeGaps(context.Background(), nil, types.KnowledgeGapsInput{
		MinBlockCount:  2,
		ExcludeNumeric: true,
	})
	if err != nil {
		t.Fatalf("KnowledgeGaps() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("KnowledgeGaps() returned error result")
	}
}

func TestListOrphans_Success(t *testing.T) {
	mb := newMockBackend()
	mb.addPage(types.PageEntity{Name: "orphan-page"},
		types.BlockEntity{UUID: "b1", Content: "content without links"},
	)
	mb.addPage(types.PageEntity{Name: "linked-page"},
		types.BlockEntity{UUID: "b2", Content: "Links to [[other]]"},
	)
	a := NewAnalyze(mb)

	result, _, err := a.ListOrphans(context.Background(), nil, types.ListOrphansInput{})
	if err != nil {
		t.Fatalf("ListOrphans() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("ListOrphans() returned error result")
	}

	var parsed map[string]any
	text := extractText(t, result)
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if parsed["total"] == nil {
		t.Error("expected total in result")
	}
	if parsed["orphans"] == nil {
		t.Error("expected orphans array in result")
	}
}

func TestListOrphans_WithLimit(t *testing.T) {
	mb := newMockBackend()
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("orphan-%d", i)
		mb.addPage(types.PageEntity{Name: name},
			types.BlockEntity{UUID: fmt.Sprintf("b-%d", i), Content: "no links"},
		)
	}
	a := NewAnalyze(mb)

	result, _, err := a.ListOrphans(context.Background(), nil, types.ListOrphansInput{
		Limit: 3,
	})
	if err != nil {
		t.Fatalf("ListOrphans() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("ListOrphans() returned error result")
	}

	var parsed map[string]any
	text := extractText(t, result)
	_ = json.Unmarshal([]byte(text), &parsed)

	returned, _ := parsed["returned"].(float64)
	if returned > 3 {
		t.Errorf("returned = %v, want <= 3", returned)
	}
}

func TestListOrphans_Error(t *testing.T) {
	mb := newMockBackend()
	mb.getAllPagesErr = fmt.Errorf("backend down")
	a := NewAnalyze(mb)

	result, _, err := a.ListOrphans(context.Background(), nil, types.ListOrphansInput{})
	if err != nil {
		t.Fatalf("ListOrphans() error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when backend fails")
	}
}

func TestTopicClusters_Success(t *testing.T) {
	mb := newMockBackend()
	// Create a connected cluster.
	mb.addPage(types.PageEntity{Name: "cluster-a"},
		types.BlockEntity{UUID: "b-ca", Content: "Links to [[cluster-b]]"},
	)
	mb.addPage(types.PageEntity{Name: "cluster-b"},
		types.BlockEntity{UUID: "b-cb", Content: "Links to [[cluster-a]]"},
	)
	a := NewAnalyze(mb)

	result, _, err := a.TopicClusters(context.Background(), nil, types.TopicClustersInput{})
	if err != nil {
		t.Fatalf("TopicClusters() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("TopicClusters() returned error result")
	}
}

func TestTopicClusters_Empty(t *testing.T) {
	mb := newMockBackend()
	a := NewAnalyze(mb)

	result, _, err := a.TopicClusters(context.Background(), nil, types.TopicClustersInput{})
	if err != nil {
		t.Fatalf("TopicClusters() error: %v", err)
	}
	// Empty graph: no clusters, but not an error.
	if result.IsError {
		t.Fatal("empty graph should not return an error")
	}
}

func TestTopicClusters_Error(t *testing.T) {
	mb := newMockBackend()
	mb.getAllPagesErr = fmt.Errorf("backend down")
	a := NewAnalyze(mb)

	result, _, err := a.TopicClusters(context.Background(), nil, types.TopicClustersInput{})
	if err != nil {
		t.Fatalf("TopicClusters() error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when backend fails")
	}
}

func TestIsNumericPageName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"12345", true},
		{"12345)", true},
		{"12345`", true},
		{"actual-page", false},
		{"page123", false},
		{"", true},
		{"  ", true},
		{"42.0", false}, // "." is not a digit
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isNumericPageName(tt.name)
			if got != tt.want {
				t.Errorf("isNumericPageName(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}
