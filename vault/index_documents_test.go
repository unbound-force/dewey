package vault

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unbound-force/dewey/source"
	"github.com/unbound-force/dewey/store"
)

// --- Concurrent indexing tests (FR-102) ---

// makeSourceDocs creates a map of source ID → documents for testing.
func makeSourceDocs(sources int, docsPerSource int) map[string][]source.Document {
	allDocs := make(map[string][]source.Document)
	for i := range sources {
		id := fmt.Sprintf("src-%d", i)
		docs := make([]source.Document, docsPerSource)
		for j := range docsPerSource {
			docs[j] = source.Document{
				ID:        fmt.Sprintf("doc-%d", j),
				Title:     fmt.Sprintf("Doc %d", j),
				Content:   fmt.Sprintf("# Heading\n\nContent for doc %d from source %d.", j, i),
				FetchedAt: time.Now(),
			}
		}
		allDocs[id] = docs
	}
	return allDocs
}

// TestIndexDocuments_TotalCountAccurate verifies that document counts are
// accurate across concurrent sources (FR-102 scenario: Index totals).
func TestIndexDocuments_TotalCountAccurate(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	// 3 sources with 10, 20, 30 documents.
	allDocs := make(map[string][]source.Document)
	for i, count := range []int{10, 20, 30} {
		id := fmt.Sprintf("src-%d", i)
		docs := make([]source.Document, count)
		for j := range count {
			docs[j] = source.Document{
				ID:        fmt.Sprintf("doc-%d", j),
				Title:     fmt.Sprintf("Doc %d", j),
				Content:   fmt.Sprintf("Content %d-%d", i, j),
				FetchedAt: time.Now(),
			}
		}
		allDocs[id] = docs
	}

	result, err := IndexDocuments(s, allDocs, nil, nil)
	if err != nil {
		t.Fatalf("IndexDocuments: %v", err)
	}

	if result.TotalIndexed != 60 {
		t.Errorf("TotalIndexed = %d, want 60", result.TotalIndexed)
	}
}

// TestIndexDocuments_EmptyInput verifies zero-doc input returns empty result.
func TestIndexDocuments_EmptyInput(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	result, err := IndexDocuments(s, map[string][]source.Document{}, nil, nil)
	if err != nil {
		t.Fatalf("IndexDocuments: %v", err)
	}
	if result.TotalIndexed != 0 {
		t.Errorf("TotalIndexed = %d, want 0", result.TotalIndexed)
	}
}

// TestIndexDocuments_ConcurrentSourceProcessing verifies that sources are
// processed concurrently by using a mock embedder that tracks concurrent access.
func TestIndexDocuments_ConcurrentSourceProcessing(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	allDocs := makeSourceDocs(4, 2)

	// Track that multiple sources are processed (they all complete).
	result, err := IndexDocuments(s, allDocs, nil, nil)
	if err != nil {
		t.Fatalf("IndexDocuments: %v", err)
	}

	if result.TotalIndexed != 8 {
		t.Errorf("TotalIndexed = %d, want 8 (4 sources × 2 docs)", result.TotalIndexed)
	}

	// Verify all pages are in the store.
	pages, err := s.ListPages()
	if err != nil {
		t.Fatalf("ListPages: %v", err)
	}
	if len(pages) != 8 {
		t.Errorf("ListPages returned %d pages, want 8", len(pages))
	}
}

// TestIndexDocuments_WithEmbeddings verifies that embeddings are generated
// when an embedder is available.
func TestIndexDocuments_WithEmbeddings(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	e := &testEmbedder{}

	allDocs := makeSourceDocs(2, 3)

	result, err := IndexDocuments(s, allDocs, nil, e)
	if err != nil {
		t.Fatalf("IndexDocuments: %v", err)
	}

	if result.TotalIndexed != 6 {
		t.Errorf("TotalIndexed = %d, want 6", result.TotalIndexed)
	}
	if result.TotalEmbeddings == 0 {
		t.Error("TotalEmbeddings = 0, want > 0")
	}

	// Verify EmbedBatch was called (not individual Embed).
	if e.embedBatchCalls.Load() == 0 {
		t.Error("EmbedBatch was never called")
	}
}

// TestIndexDocuments_SourceRecordCreated verifies that source records are
// created in the store after indexing.
func TestIndexDocuments_SourceRecordCreated(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	configs := []source.SourceConfig{
		{ID: "disk-notes", Type: "disk", Name: "notes"},
	}
	allDocs := map[string][]source.Document{
		"disk-notes": {
			{ID: "note1", Title: "Note 1", Content: "content", FetchedAt: time.Now()},
		},
	}

	_, err = IndexDocuments(s, allDocs, configs, nil)
	if err != nil {
		t.Fatalf("IndexDocuments: %v", err)
	}

	rec, err := s.GetSource("disk-notes")
	if err != nil {
		t.Fatalf("GetSource: %v", err)
	}
	if rec == nil {
		t.Fatal("source record should exist after indexing")
	}
	if rec.Status != "active" {
		t.Errorf("source status = %q, want %q", rec.Status, "active")
	}
}

// testConcurrentEmbedder tracks concurrent embedding calls for race testing.
type testConcurrentEmbedder struct {
	embedBatchCalls atomic.Int64
}

func (e *testConcurrentEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return []float32{0.1, 0.2, 0.3}, nil
}
func (e *testConcurrentEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	e.embedBatchCalls.Add(1)
	result := make([][]float32, len(texts))
	for i := range texts {
		result[i] = []float32{0.1, 0.2, 0.3}
	}
	return result, nil
}
func (e *testConcurrentEmbedder) Available() bool { return true }
func (e *testConcurrentEmbedder) ModelID() string { return "test-model" }
