package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/unbound-force/dewey/embed"
	"github.com/unbound-force/dewey/store"
	"github.com/unbound-force/dewey/types"
)

// resultText extracts the text content from an MCP result, handling the type assertion safely.
func resultText(result *mcp.CallToolResult) string {
	tc, _ := result.Content[0].(*mcp.TextContent)
	return tc.Text
}

// mockEmbedder is a test double for embed.Embedder.
// Returns pre-configured vectors for testing.
type mockEmbedder struct {
	available bool
	model     string
	vectors   map[string][]float32 // text → vector
}

func newMockEmbedder(available bool) *mockEmbedder {
	return &mockEmbedder{
		available: available,
		model:     "test-model",
		vectors:   make(map[string][]float32),
	}
}

func (m *mockEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	if !m.available {
		return nil, fmt.Errorf("model not available")
	}
	if vec, ok := m.vectors[text]; ok {
		return vec, nil
	}
	// Default: return a simple hash-based vector for any text.
	return []float32{0.5, 0.5, 0.5}, nil
}

func (m *mockEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	if !m.available {
		return nil, fmt.Errorf("model not available")
	}
	result := make([][]float32, len(texts))
	for i, t := range texts {
		vec, err := m.Embed(context.Background(), t)
		if err != nil {
			return nil, err
		}
		result[i] = vec
	}
	return result, nil
}

func (m *mockEmbedder) Available() bool { return m.available }
func (m *mockEmbedder) ModelID() string { return m.model }

// Verify mockEmbedder implements embed.Embedder at compile time.
var _ embed.Embedder = (*mockEmbedder)(nil)

// newTestStoreWithData creates an in-memory store with test pages, blocks,
// and embeddings for semantic search testing.
func newTestStoreWithData(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	// Insert test pages.
	pages := []*store.Page{
		{Name: "setup", OriginalName: "setup", SourceID: "disk-local", SourceDocID: "setup.md", ContentHash: "abc", CreatedAt: 1000, UpdatedAt: 1000},
		{Name: "api-guide", OriginalName: "api-guide", SourceID: "disk-local", SourceDocID: "api-guide.md", ContentHash: "def", CreatedAt: 1000, UpdatedAt: 1000},
	}
	for _, p := range pages {
		if err := s.InsertPage(p); err != nil {
			t.Fatalf("InsertPage(%s): %v", p.Name, err)
		}
	}

	// Insert test blocks.
	blocks := []*store.Block{
		{UUID: "block-install", PageName: "setup", Content: "## Installation\nRun go install to set up.", HeadingLevel: 2, Position: 0},
		{UUID: "block-config", PageName: "setup", Content: "## Configuration\nEdit config.yaml.", HeadingLevel: 2, Position: 1},
		{UUID: "block-api", PageName: "api-guide", Content: "## API Reference\nThe REST API supports GET and POST.", HeadingLevel: 2, Position: 0},
	}
	for _, b := range blocks {
		if err := s.InsertBlock(b); err != nil {
			t.Fatalf("InsertBlock(%s): %v", b.UUID, err)
		}
	}

	// Insert test embeddings with known vectors.
	embeddings := []struct {
		uuid  string
		vec   []float32
		chunk string
	}{
		{"block-install", []float32{1, 0, 0}, "setup > Installation\n\nRun go install"},
		{"block-config", []float32{0, 1, 0}, "setup > Configuration\n\nEdit config.yaml"},
		{"block-api", []float32{0.9, 0.1, 0}, "api-guide > API Reference\n\nREST API"},
	}
	for _, e := range embeddings {
		if err := s.InsertEmbedding(e.uuid, "test-model", e.vec, e.chunk); err != nil {
			t.Fatalf("InsertEmbedding(%s): %v", e.uuid, err)
		}
	}

	return s
}

// TestSemanticSearch_Basic verifies basic semantic search returns ranked results.
func TestSemanticSearch_Basic(t *testing.T) {
	s := newTestStoreWithData(t)
	e := newMockEmbedder(true)
	// Mock: query "how to install" returns vector close to block-install.
	e.vectors["how to install"] = []float32{0.95, 0.05, 0}

	sem := NewSemantic(e, s)

	result, _, err := sem.SemanticSearch(context.Background(), nil, types.SemanticSearchInput{
		Query: "how to install",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("SemanticSearch error: %v", err)
	}
	if result.IsError {
		t.Fatalf("SemanticSearch returned error: %s", resultText(result))
	}

	// Parse results.
	var results []types.SemanticSearchResult
	text := resultText(result)
	if err := json.Unmarshal([]byte(text), &results); err != nil {
		t.Fatalf("unmarshal results: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected results, got none")
	}

	// First result should be block-install (closest vector).
	if results[0].DocumentID != "block-install" {
		t.Errorf("first result = %q, want %q", results[0].DocumentID, "block-install")
	}

	// Verify provenance metadata.
	if results[0].Page != "setup" {
		t.Errorf("page = %q, want %q", results[0].Page, "setup")
	}
	if results[0].Source != "disk" {
		t.Errorf("source = %q, want %q", results[0].Source, "disk")
	}
	if results[0].SourceID != "disk-local" {
		t.Errorf("source_id = %q, want %q", results[0].SourceID, "disk-local")
	}
	if results[0].IndexedAt == "" {
		t.Error("indexed_at should not be empty")
	}
}

// TestSemanticSearch_EmptyIndex verifies behavior with no embeddings.
func TestSemanticSearch_EmptyIndex(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	e := newMockEmbedder(true)
	sem := NewSemantic(e, s)

	result, _, err := sem.SemanticSearch(context.Background(), nil, types.SemanticSearchInput{
		Query: "anything",
	})
	if err != nil {
		t.Fatalf("SemanticSearch error: %v", err)
	}
	if result.IsError {
		t.Fatalf("empty index should not be an error: %s", resultText(result))
	}

	// Should return empty array, not error.
	var results []types.SemanticSearchResult
	text := resultText(result)
	_ = json.Unmarshal([]byte(text), &results)
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty index, got %d", len(results))
	}
}

// TestSemanticSearch_EmbedderUnavailable verifies graceful degradation.
func TestSemanticSearch_EmbedderUnavailable(t *testing.T) {
	s := newTestStoreWithData(t)
	e := newMockEmbedder(false) // unavailable
	sem := NewSemantic(e, s)

	result, _, err := sem.SemanticSearch(context.Background(), nil, types.SemanticSearchInput{
		Query: "test",
	})
	if err != nil {
		t.Fatalf("SemanticSearch error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when embedder unavailable")
	}

	text := resultText(result)
	if !strings.Contains(text, "embedding model not loaded") {
		t.Errorf("error message = %q, should mention embedding model", text)
	}
}

// TestSemanticSearch_NilEmbedder verifies graceful degradation with nil embedder.
func TestSemanticSearch_NilEmbedder(t *testing.T) {
	s := newTestStoreWithData(t)
	sem := NewSemantic(nil, s)

	result, _, err := sem.SemanticSearch(context.Background(), nil, types.SemanticSearchInput{
		Query: "test",
	})
	if err != nil {
		t.Fatalf("SemanticSearch error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when embedder is nil")
	}
}

// TestSimilar_ByUUID verifies finding similar documents by block UUID.
func TestSimilar_ByUUID(t *testing.T) {
	s := newTestStoreWithData(t)
	e := newMockEmbedder(true)
	sem := NewSemantic(e, s)

	result, _, err := sem.Similar(context.Background(), nil, types.SimilarInput{
		UUID:  "block-install",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Similar error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Similar returned error: %s", resultText(result))
	}

	var results []types.SemanticSearchResult
	text := resultText(result)
	_ = json.Unmarshal([]byte(text), &results)

	// Should not include the query document itself.
	for _, r := range results {
		if r.DocumentID == "block-install" {
			t.Error("similar results should not include the query document")
		}
	}

	// block-api (0.9, 0.1, 0) should be the most similar to block-install (1, 0, 0).
	if len(results) > 0 && results[0].DocumentID != "block-api" {
		t.Errorf("most similar = %q, want %q", results[0].DocumentID, "block-api")
	}
}

// TestSimilar_ByPage verifies finding similar documents by page name.
func TestSimilar_ByPage(t *testing.T) {
	s := newTestStoreWithData(t)
	e := newMockEmbedder(true)
	sem := NewSemantic(e, s)

	result, _, err := sem.Similar(context.Background(), nil, types.SimilarInput{
		Page:  "setup",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Similar error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Similar returned error: %s", resultText(result))
	}
}

// TestSimilar_NeitherPageNorUUID verifies the "neither provided" error.
func TestSimilar_NeitherPageNorUUID(t *testing.T) {
	s := newTestStoreWithData(t)
	e := newMockEmbedder(true)
	sem := NewSemantic(e, s)

	result, _, err := sem.Similar(context.Background(), nil, types.SimilarInput{})
	if err != nil {
		t.Fatalf("Similar error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error when neither page nor uuid provided")
	}

	text := resultText(result)
	if !strings.Contains(text, "At least one of 'page' or 'uuid' must be provided") {
		t.Errorf("error message = %q, want 'At least one of...'", text)
	}
}

// TestSimilar_NoEmbeddingFound verifies error when block has no embedding.
func TestSimilar_NoEmbeddingFound(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	// Create page and block but no embedding.
	_ = s.InsertPage(&store.Page{Name: "test", OriginalName: "test", SourceID: "disk-local", SourceDocID: "test.md", ContentHash: "x", CreatedAt: 1, UpdatedAt: 1})
	_ = s.InsertBlock(&store.Block{UUID: "block-no-embed", PageName: "test", Content: "content", Position: 0})

	// Insert one embedding so the "no embeddings in index" check passes.
	_ = s.InsertBlock(&store.Block{UUID: "block-with-embed", PageName: "test", Content: "other", Position: 1})
	_ = s.InsertEmbedding("block-with-embed", "test-model", []float32{1, 0}, "chunk")

	e := newMockEmbedder(true)
	sem := NewSemantic(e, s)

	result, _, _ := sem.Similar(context.Background(), nil, types.SimilarInput{
		UUID: "block-no-embed",
	})
	if !result.IsError {
		t.Fatal("expected error for block with no embedding")
	}

	text := resultText(result)
	if !strings.Contains(text, "No embedding found") {
		t.Errorf("error message = %q, want 'No embedding found...'", text)
	}
}

// TestSimilar_NoEmbeddingsInIndex verifies error when index is completely empty.
func TestSimilar_NoEmbeddingsInIndex(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	e := newMockEmbedder(true)
	sem := NewSemantic(e, s)

	result, _, _ := sem.Similar(context.Background(), nil, types.SimilarInput{
		UUID: "any-uuid",
	})
	if !result.IsError {
		t.Fatal("expected error when no embeddings in index")
	}

	text := resultText(result)
	if !strings.Contains(text, "No embeddings in index") {
		t.Errorf("error message = %q, want 'No embeddings in index...'", text)
	}
}

// TestSemanticSearchFiltered_Basic verifies filtered semantic search.
func TestSemanticSearchFiltered_Basic(t *testing.T) {
	s := newTestStoreWithData(t)
	e := newMockEmbedder(true)
	e.vectors["search query"] = []float32{0.9, 0.1, 0}

	sem := NewSemantic(e, s)

	result, _, err := sem.SemanticSearchFiltered(context.Background(), nil, types.SemanticSearchFilteredInput{
		Query:    "search query",
		SourceID: "disk-local",
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("SemanticSearchFiltered error: %v", err)
	}
	if result.IsError {
		t.Fatalf("SemanticSearchFiltered returned error: %s", resultText(result))
	}

	var results []types.SemanticSearchResult
	text := resultText(result)
	_ = json.Unmarshal([]byte(text), &results)

	// All results should have source_id = "disk-local".
	for _, r := range results {
		if r.SourceID != "disk-local" {
			t.Errorf("filtered result source_id = %q, want %q", r.SourceID, "disk-local")
		}
	}
}

// TestSemanticSearchFiltered_EmbedderUnavailable verifies degradation.
func TestSemanticSearchFiltered_EmbedderUnavailable(t *testing.T) {
	s := newTestStoreWithData(t)
	e := newMockEmbedder(false)
	sem := NewSemantic(e, s)

	result, _, _ := sem.SemanticSearchFiltered(context.Background(), nil, types.SemanticSearchFilteredInput{
		Query: "test",
	})
	if !result.IsError {
		t.Fatal("expected error when embedder unavailable")
	}
}

// TestSemanticSearch_DefaultThreshold verifies default threshold is applied.
func TestSemanticSearch_DefaultThreshold(t *testing.T) {
	s := newTestStoreWithData(t)
	e := newMockEmbedder(true)
	// Query vector orthogonal to all embeddings.
	e.vectors["orthogonal query"] = []float32{0, 0, 1}

	sem := NewSemantic(e, s)

	result, _, err := sem.SemanticSearch(context.Background(), nil, types.SemanticSearchInput{
		Query: "orthogonal query",
		// No threshold specified — default 0.3 should filter out orthogonal results.
	})
	if err != nil {
		t.Fatalf("SemanticSearch error: %v", err)
	}

	var results []types.SemanticSearchResult
	text := resultText(result)
	_ = json.Unmarshal([]byte(text), &results)

	// All embeddings are orthogonal to [0,0,1], so similarity = 0.
	// Default threshold 0.3 should filter them all out.
	if len(results) != 0 {
		t.Errorf("expected 0 results with default threshold, got %d", len(results))
	}
}

// TestSemanticSearch_ProvenanceMetadata verifies all provenance fields are populated.
func TestSemanticSearch_ProvenanceMetadata(t *testing.T) {
	s := newTestStoreWithData(t)
	e := newMockEmbedder(true)
	e.vectors["install query"] = []float32{1, 0, 0}

	sem := NewSemantic(e, s)

	result, _, _ := sem.SemanticSearch(context.Background(), nil, types.SemanticSearchInput{
		Query:     "install query",
		Limit:     1,
		Threshold: 0.0,
	})

	var results []types.SemanticSearchResult
	text := resultText(result)
	_ = json.Unmarshal([]byte(text), &results)

	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}

	r := results[0]
	if r.DocumentID == "" {
		t.Error("document_id should not be empty")
	}
	if r.Page == "" {
		t.Error("page should not be empty")
	}
	if r.Content == "" {
		t.Error("content should not be empty")
	}
	if r.Similarity <= 0 {
		t.Error("similarity should be positive")
	}
	if r.Source == "" {
		t.Error("source should not be empty")
	}
	if r.SourceID == "" {
		t.Error("source_id should not be empty")
	}
	if r.IndexedAt == "" {
		t.Error("indexed_at should not be empty")
	}
}
