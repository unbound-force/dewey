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

	// Verify all result fields are populated (complete result structure).
	for i, r := range results {
		if r.DocumentID == "" {
			t.Errorf("results[%d].DocumentID should not be empty", i)
		}
		if r.Page == "" {
			t.Errorf("results[%d].Page should not be empty", i)
		}
		if r.Content == "" {
			t.Errorf("results[%d].Content should not be empty", i)
		}
		if r.Similarity <= 0 || r.Similarity > 1 {
			t.Errorf("results[%d].Similarity = %f, want (0, 1]", i, r.Similarity)
		}
		if r.Source == "" {
			t.Errorf("results[%d].Source should not be empty", i)
		}
		if r.SourceID == "" {
			t.Errorf("results[%d].SourceID should not be empty", i)
		}
	}

	// Verify results are ordered by descending similarity score.
	for i := 1; i < len(results); i++ {
		if results[i].Similarity > results[i-1].Similarity {
			t.Errorf("results not ordered: [%d].Similarity=%f > [%d].Similarity=%f",
				i, results[i].Similarity, i-1, results[i-1].Similarity)
		}
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

	text := resultText(result)
	if !strings.Contains(text, "embedding model not loaded") {
		t.Errorf("error message = %q, should mention embedding model", text)
	}
}

// TestSemanticSearch_NilStore verifies graceful degradation with nil store.
func TestSemanticSearch_NilStore(t *testing.T) {
	e := newMockEmbedder(true)
	sem := NewSemantic(e, nil)

	result, _, err := sem.SemanticSearch(context.Background(), nil, types.SemanticSearchInput{
		Query: "test",
	})
	if err != nil {
		t.Fatalf("SemanticSearch error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when store is nil")
	}

	text := resultText(result)
	if !strings.Contains(text, "no persistent store") {
		t.Errorf("error message = %q, should mention persistent store", text)
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

	// Verify similarity scores are between 0 and 1.
	for i, r := range results {
		if r.Similarity < 0 || r.Similarity > 1 {
			t.Errorf("results[%d].Similarity = %f, want between 0 and 1", i, r.Similarity)
		}
	}

	// Verify results are ordered by descending similarity score.
	for i := 1; i < len(results); i++ {
		if results[i].Similarity > results[i-1].Similarity {
			t.Errorf("results not ordered by descending score: results[%d].Similarity=%f > results[%d].Similarity=%f",
				i, results[i].Similarity, i-1, results[i-1].Similarity)
		}
	}

	// Verify provenance metadata is populated on all results.
	for i, r := range results {
		if r.Page == "" {
			t.Errorf("results[%d].Page should not be empty", i)
		}
		if r.Content == "" {
			t.Errorf("results[%d].Content should not be empty", i)
		}
		if r.Source == "" {
			t.Errorf("results[%d].Source should not be empty", i)
		}
		if r.SourceID == "" {
			t.Errorf("results[%d].SourceID should not be empty", i)
		}
		if r.DocumentID == "" {
			t.Errorf("results[%d].DocumentID should not be empty", i)
		}
		if r.IndexedAt == "" {
			t.Errorf("results[%d].IndexedAt should not be empty", i)
		}
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

	var results []types.SemanticSearchResult
	text := resultText(result)
	if err := json.Unmarshal([]byte(text), &results); err != nil {
		t.Fatalf("unmarshal results: %v", err)
	}

	// Page lookup uses the first block's embedding (block-install: [1,0,0]).
	// Should return similar results based on that vector.
	if len(results) == 0 {
		t.Fatal("expected at least 1 similar result for page 'setup'")
	}

	// Verify similarity scores are valid (0-1 range, descending order).
	for i, r := range results {
		if r.Similarity < 0 || r.Similarity > 1 {
			t.Errorf("results[%d].Similarity = %f, want between 0 and 1", i, r.Similarity)
		}
		if r.DocumentID == "" {
			t.Errorf("results[%d].DocumentID should not be empty", i)
		}
	}
	for i := 1; i < len(results); i++ {
		if results[i].Similarity > results[i-1].Similarity {
			t.Errorf("results not ordered by descending score: [%d]=%f > [%d]=%f",
				i, results[i].Similarity, i-1, results[i-1].Similarity)
		}
	}
}

// TestSimilar_EmbedderUnavailable verifies graceful degradation for Similar.
func TestSimilar_EmbedderUnavailable(t *testing.T) {
	s := newTestStoreWithData(t)
	e := newMockEmbedder(false)
	sem := NewSemantic(e, s)

	result, _, err := sem.Similar(context.Background(), nil, types.SimilarInput{
		UUID: "block-install",
	})
	if err != nil {
		t.Fatalf("Similar error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when embedder unavailable")
	}

	text := resultText(result)
	if !strings.Contains(text, "embedding model not loaded") {
		t.Errorf("error message = %q, should mention embedding model", text)
	}
}

// TestSimilar_NilStore verifies graceful degradation for Similar with nil store.
func TestSimilar_NilStore(t *testing.T) {
	e := newMockEmbedder(true)
	sem := NewSemantic(e, nil)

	result, _, err := sem.Similar(context.Background(), nil, types.SimilarInput{
		UUID: "block-install",
	})
	if err != nil {
		t.Fatalf("Similar error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when store is nil")
	}

	text := resultText(result)
	if !strings.Contains(text, "no persistent store") {
		t.Errorf("error message = %q, should mention persistent store", text)
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

// TestSimilar_ByUUID_ResultFields verifies that Similar results have all fields
// correctly populated with expected types and values.
func TestSimilar_ByUUID_ResultFields(t *testing.T) {
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
	if err := json.Unmarshal([]byte(text), &results); err != nil {
		t.Fatalf("unmarshal results: %v", err)
	}

	// Verify we got results.
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}

	// Verify exact result count: with block-install excluded, we expect 2 results
	// (block-config and block-api).
	if len(results) != 2 {
		t.Errorf("expected 2 results (excluding query doc), got %d", len(results))
	}

	// Verify each result has all required fields with valid values.
	for i, r := range results {
		// Text/Content must be non-empty.
		if r.Content == "" {
			t.Errorf("results[%d].Content is empty", i)
		}

		// DocumentID must be non-empty and not the query UUID.
		if r.DocumentID == "" {
			t.Errorf("results[%d].DocumentID is empty", i)
		}
		if r.DocumentID == "block-install" {
			t.Errorf("results[%d].DocumentID = query doc 'block-install', should be excluded", i)
		}

		// Similarity must be a valid float64 in range [0, 1].
		if r.Similarity < 0 || r.Similarity > 1.0 {
			t.Errorf("results[%d].Similarity = %f, want in range [0, 1]", i, r.Similarity)
		}

		// Page must be non-empty.
		if r.Page == "" {
			t.Errorf("results[%d].Page is empty", i)
		}

		// Source must be non-empty.
		if r.Source == "" {
			t.Errorf("results[%d].Source is empty", i)
		}

		// SourceID must be non-empty.
		if r.SourceID == "" {
			t.Errorf("results[%d].SourceID is empty", i)
		}

		// IndexedAt must be a valid RFC3339 timestamp.
		if r.IndexedAt == "" {
			t.Errorf("results[%d].IndexedAt is empty", i)
		}
	}

	// Verify descending sort order by similarity score.
	for i := 1; i < len(results); i++ {
		if results[i].Similarity > results[i-1].Similarity {
			t.Errorf("results not in descending order: [%d].Similarity=%f > [%d].Similarity=%f",
				i, results[i].Similarity, i-1, results[i-1].Similarity)
		}
	}
}

// TestSimilar_ByPage_ResultFields verifies Similar by page returns results with
// all provenance fields populated and correct result count.
func TestSimilar_ByPage_ResultFields(t *testing.T) {
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

	var results []types.SemanticSearchResult
	text := resultText(result)
	if err := json.Unmarshal([]byte(text), &results); err != nil {
		t.Fatalf("unmarshal results: %v", err)
	}

	// Should return results (at least block-api which is on a different page).
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}

	// Verify each result has non-empty Text (Content) and DocumentID.
	for i, r := range results {
		if r.Content == "" {
			t.Errorf("results[%d].Content is empty", i)
		}
		if r.DocumentID == "" {
			t.Errorf("results[%d].DocumentID is empty", i)
		}

		// Similarity should be valid float64.
		if r.Similarity < 0 || r.Similarity > 1.0 {
			t.Errorf("results[%d].Similarity = %f, want in range [0, 1]", i, r.Similarity)
		}

		// Provenance fields should all be populated.
		if r.Page == "" {
			t.Errorf("results[%d].Page is empty", i)
		}
		if r.Source == "" {
			t.Errorf("results[%d].Source is empty", i)
		}
		if r.SourceID == "" {
			t.Errorf("results[%d].SourceID is empty", i)
		}
	}
}

// TestSimilar_LimitRespected verifies that the limit parameter is respected.
func TestSimilar_LimitRespected(t *testing.T) {
	s := newTestStoreWithData(t)
	e := newMockEmbedder(true)
	sem := NewSemantic(e, s)

	result, _, err := sem.Similar(context.Background(), nil, types.SimilarInput{
		UUID:  "block-install",
		Limit: 1,
	})
	if err != nil {
		t.Fatalf("Similar error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Similar returned error: %s", resultText(result))
	}

	var results []types.SemanticSearchResult
	text := resultText(result)
	if err := json.Unmarshal([]byte(text), &results); err != nil {
		t.Fatalf("unmarshal results: %v", err)
	}

	if len(results) > 1 {
		t.Errorf("expected at most 1 result with Limit=1, got %d", len(results))
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

	if len(results) == 0 {
		t.Fatal("expected at least 1 filtered result")
	}

	// All results should have source_id = "disk-local" (filter was applied).
	for i, r := range results {
		if r.SourceID != "disk-local" {
			t.Errorf("filtered results[%d].SourceID = %q, want %q", i, r.SourceID, "disk-local")
		}
		// Verify complete result structure for filtered results.
		if r.DocumentID == "" {
			t.Errorf("filtered results[%d].DocumentID should not be empty", i)
		}
		if r.Page == "" {
			t.Errorf("filtered results[%d].Page should not be empty", i)
		}
		if r.Content == "" {
			t.Errorf("filtered results[%d].Content should not be empty", i)
		}
		if r.Similarity <= 0 || r.Similarity > 1 {
			t.Errorf("filtered results[%d].Similarity = %f, want (0, 1]", i, r.Similarity)
		}
	}

	// Verify results are ordered by descending similarity score.
	for i := 1; i < len(results); i++ {
		if results[i].Similarity > results[i-1].Similarity {
			t.Errorf("filtered results not ordered: [%d].Similarity=%f > [%d].Similarity=%f",
				i, results[i].Similarity, i-1, results[i-1].Similarity)
		}
	}
}

// TestSemanticSearchFiltered_EmbedderUnavailable verifies degradation.
func TestSemanticSearchFiltered_EmbedderUnavailable(t *testing.T) {
	s := newTestStoreWithData(t)
	e := newMockEmbedder(false)
	sem := NewSemantic(e, s)

	result, _, err := sem.SemanticSearchFiltered(context.Background(), nil, types.SemanticSearchFilteredInput{
		Query: "test",
	})
	if err != nil {
		t.Fatalf("SemanticSearchFiltered error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error when embedder unavailable")
	}

	text := resultText(result)
	if !strings.Contains(text, "embedding model not loaded") {
		t.Errorf("error message = %q, should mention embedding model", text)
	}
}

// TestSemanticSearchFiltered_NilStore verifies degradation with nil store.
func TestSemanticSearchFiltered_NilStore(t *testing.T) {
	e := newMockEmbedder(true)
	sem := NewSemantic(e, nil)

	result, _, err := sem.SemanticSearchFiltered(context.Background(), nil, types.SemanticSearchFilteredInput{
		Query: "test",
	})
	if err != nil {
		t.Fatalf("SemanticSearchFiltered error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error when store is nil")
	}

	text := resultText(result)
	if !strings.Contains(text, "no persistent store") {
		t.Errorf("error message = %q, should mention persistent store", text)
	}
}

// TestSemanticSearchFiltered_EmptyIndex verifies filtered search on empty index.
func TestSemanticSearchFiltered_EmptyIndex(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = s.Close() }()

	e := newMockEmbedder(true)
	sem := NewSemantic(e, s)

	result, _, err := sem.SemanticSearchFiltered(context.Background(), nil, types.SemanticSearchFilteredInput{
		Query:    "anything",
		SourceID: "disk-local",
	})
	if err != nil {
		t.Fatalf("SemanticSearchFiltered error: %v", err)
	}
	if result.IsError {
		t.Fatalf("empty index should not be an error: %s", resultText(result))
	}

	var results []types.SemanticSearchResult
	text := resultText(result)
	if err := json.Unmarshal([]byte(text), &results); err != nil {
		t.Fatalf("unmarshal results: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty index, got %d", len(results))
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

// --- Phase 3 tests: Search metadata enrichment (013-knowledge-compile) ---

// newTestStoreWithTieredData creates an in-memory store with pages of different
// tiers and categories for testing metadata enrichment and tier filtering.
func newTestStoreWithTieredData(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	// Insert pages with different tiers and categories.
	// Each page needs a unique (source_id, source_doc_id) pair per the schema constraint.
	pages := []*store.Page{
		{
			Name: "authored-spec", OriginalName: "authored-spec",
			SourceID: "disk-local", SourceDocID: "spec.md",
			ContentHash: "aaa", CreatedAt: 1000, UpdatedAt: 2000,
			Tier: "authored",
		},
		{
			Name: "learning/auth-1", OriginalName: "learning/auth-1",
			SourceID: "learning", SourceDocID: "learning/auth-1",
			ContentHash: "bbb", CreatedAt: 3000, UpdatedAt: 4000,
			Tier: "draft", Category: "decision",
		},
		{
			Name: "learning/perf-1", OriginalName: "learning/perf-1",
			SourceID: "learning", SourceDocID: "learning/perf-1",
			ContentHash: "ccc", CreatedAt: 5000, UpdatedAt: 6000,
			Tier: "draft", Category: "pattern",
		},
		{
			Name: "validated-learning", OriginalName: "validated-learning",
			SourceID: "learning", SourceDocID: "validated-learning",
			ContentHash: "ddd", CreatedAt: 7000, UpdatedAt: 8000,
			Tier: "validated", Category: "gotcha",
		},
	}
	for _, p := range pages {
		if err := s.InsertPage(p); err != nil {
			t.Fatalf("InsertPage(%s): %v", p.Name, err)
		}
	}

	// Insert blocks for each page.
	blocks := []*store.Block{
		{UUID: "block-spec", PageName: "authored-spec", Content: "## Spec\nAuthentication spec.", HeadingLevel: 2, Position: 0},
		{UUID: "block-auth", PageName: "learning/auth-1", Content: "## Auth\nUse Option A for auth.", HeadingLevel: 2, Position: 0},
		{UUID: "block-perf", PageName: "learning/perf-1", Content: "## Perf\nCache frequently.", HeadingLevel: 2, Position: 0},
		{UUID: "block-validated", PageName: "validated-learning", Content: "## Gotcha\nWatch for race conditions.", HeadingLevel: 2, Position: 0},
	}
	for _, b := range blocks {
		if err := s.InsertBlock(b); err != nil {
			t.Fatalf("InsertBlock(%s): %v", b.UUID, err)
		}
	}

	// Insert embeddings with distinct vectors so we can control search results.
	embeddings := []struct {
		uuid  string
		vec   []float32
		chunk string
	}{
		{"block-spec", []float32{1, 0, 0, 0}, "authored-spec > Spec\n\nAuthentication spec."},
		{"block-auth", []float32{0.9, 0.1, 0, 0}, "learning/auth-1 > Auth\n\nUse Option A for auth."},
		{"block-perf", []float32{0, 1, 0, 0}, "learning/perf-1 > Perf\n\nCache frequently."},
		{"block-validated", []float32{0, 0, 1, 0}, "validated-learning > Gotcha\n\nWatch for race conditions."},
	}
	for _, e := range embeddings {
		if err := s.InsertEmbedding(e.uuid, "test-model", e.vec, e.chunk); err != nil {
			t.Fatalf("InsertEmbedding(%s): %v", e.uuid, err)
		}
	}

	return s
}

// TestSemanticSearch_MetadataEnrichment verifies that search results include
// created_at, tier, and category metadata from page data (FR-004).
func TestSemanticSearch_MetadataEnrichment(t *testing.T) {
	s := newTestStoreWithTieredData(t)
	e := newMockEmbedder(true)
	// Query vector close to block-auth (learning with category=decision, tier=draft).
	e.vectors["auth query"] = []float32{0.85, 0.15, 0, 0}

	sem := NewSemantic(e, s)

	result, _, err := sem.SemanticSearch(context.Background(), nil, types.SemanticSearchInput{
		Query:     "auth query",
		Limit:     10,
		Threshold: 0.0,
	})
	if err != nil {
		t.Fatalf("SemanticSearch error: %v", err)
	}
	if result.IsError {
		t.Fatalf("SemanticSearch returned error: %s", resultText(result))
	}

	var results []types.SemanticSearchResult
	text := resultText(result)
	if err := json.Unmarshal([]byte(text), &results); err != nil {
		t.Fatalf("unmarshal results: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected results, got none")
	}

	// Find the learning/auth-1 result (should be near the top).
	var authResult *types.SemanticSearchResult
	var specResult *types.SemanticSearchResult
	for i := range results {
		switch results[i].Page {
		case "learning/auth-1":
			authResult = &results[i]
		case "authored-spec":
			specResult = &results[i]
		}
	}

	// Verify learning result has tier, category, and created_at.
	if authResult == nil {
		t.Fatal("expected learning/auth-1 in results")
	}
	if authResult.Tier != "draft" {
		t.Errorf("learning tier = %q, want %q", authResult.Tier, "draft")
	}
	if authResult.Category != "decision" {
		t.Errorf("learning category = %q, want %q", authResult.Category, "decision")
	}
	if authResult.CreatedAt == "" {
		t.Error("learning created_at should not be empty")
	}
	// Verify created_at is a valid RFC3339 timestamp.
	if !strings.Contains(authResult.CreatedAt, "T") {
		t.Errorf("created_at = %q, want RFC3339 format", authResult.CreatedAt)
	}

	// Verify authored page has tier but no category (non-learning pages).
	if specResult == nil {
		t.Fatal("expected authored-spec in results")
	}
	if specResult.Tier != "authored" {
		t.Errorf("authored tier = %q, want %q", specResult.Tier, "authored")
	}
	if specResult.Category != "" {
		t.Errorf("authored category = %q, want empty (non-learning page)", specResult.Category)
	}
	if specResult.CreatedAt == "" {
		t.Error("authored created_at should not be empty")
	}
}

// TestSemanticSearch_TierMetadataOnAllResults verifies that all results
// include tier metadata, regardless of source type.
func TestSemanticSearch_TierMetadataOnAllResults(t *testing.T) {
	s := newTestStoreWithTieredData(t)
	e := newMockEmbedder(true)
	// Broad query that matches everything.
	e.vectors["broad query"] = []float32{0.5, 0.5, 0.5, 0.5}

	sem := NewSemantic(e, s)

	result, _, err := sem.SemanticSearch(context.Background(), nil, types.SemanticSearchInput{
		Query:     "broad query",
		Limit:     10,
		Threshold: 0.0,
	})
	if err != nil {
		t.Fatalf("SemanticSearch error: %v", err)
	}
	if result.IsError {
		t.Fatalf("SemanticSearch returned error: %s", resultText(result))
	}

	var results []types.SemanticSearchResult
	text := resultText(result)
	if err := json.Unmarshal([]byte(text), &results); err != nil {
		t.Fatalf("unmarshal results: %v", err)
	}

	if len(results) < 4 {
		t.Fatalf("expected at least 4 results, got %d", len(results))
	}

	// Every result should have a non-empty tier.
	for i, r := range results {
		if r.Tier == "" {
			t.Errorf("results[%d] (page=%s) tier should not be empty", i, r.Page)
		}
		if r.CreatedAt == "" {
			t.Errorf("results[%d] (page=%s) created_at should not be empty", i, r.Page)
		}
	}
}

// TestSemanticSearchFiltered_TierFilter verifies that tier filtering returns
// only pages matching the requested tier (FR-024).
func TestSemanticSearchFiltered_TierFilter(t *testing.T) {
	s := newTestStoreWithTieredData(t)
	e := newMockEmbedder(true)
	// Broad query to match all pages.
	e.vectors["all pages"] = []float32{0.5, 0.5, 0.5, 0.5}

	sem := NewSemantic(e, s)

	// Filter for "authored" tier — should only return authored-spec.
	result, _, err := sem.SemanticSearchFiltered(context.Background(), nil, types.SemanticSearchFilteredInput{
		Query:     "all pages",
		Tier:      "authored",
		Limit:     10,
		Threshold: 0.0,
	})
	if err != nil {
		t.Fatalf("SemanticSearchFiltered error: %v", err)
	}
	if result.IsError {
		t.Fatalf("SemanticSearchFiltered returned error: %s", resultText(result))
	}

	var results []types.SemanticSearchResult
	text := resultText(result)
	if err := json.Unmarshal([]byte(text), &results); err != nil {
		t.Fatalf("unmarshal results: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected at least 1 result for tier=authored")
	}

	// All results should have tier=authored.
	for i, r := range results {
		if r.Tier != "authored" {
			t.Errorf("results[%d] tier = %q, want %q (tier filter should exclude non-authored)", i, r.Tier, "authored")
		}
	}

	// Specifically, no learning pages should appear.
	for _, r := range results {
		if strings.HasPrefix(r.Page, "learning/") {
			t.Errorf("learning page %q should not appear with tier=authored filter", r.Page)
		}
	}
}

// TestSemanticSearchFiltered_TierFilterDraft verifies draft tier filtering
// returns only draft pages (learnings).
func TestSemanticSearchFiltered_TierFilterDraft(t *testing.T) {
	s := newTestStoreWithTieredData(t)
	e := newMockEmbedder(true)
	e.vectors["draft query"] = []float32{0.5, 0.5, 0.5, 0.5}

	sem := NewSemantic(e, s)

	result, _, err := sem.SemanticSearchFiltered(context.Background(), nil, types.SemanticSearchFilteredInput{
		Query:     "draft query",
		Tier:      "draft",
		Limit:     10,
		Threshold: 0.0,
	})
	if err != nil {
		t.Fatalf("SemanticSearchFiltered error: %v", err)
	}
	if result.IsError {
		t.Fatalf("SemanticSearchFiltered returned error: %s", resultText(result))
	}

	var results []types.SemanticSearchResult
	text := resultText(result)
	if err := json.Unmarshal([]byte(text), &results); err != nil {
		t.Fatalf("unmarshal results: %v", err)
	}

	// Should return the 2 draft pages (learning/auth-1 and learning/perf-1).
	if len(results) != 2 {
		t.Fatalf("expected 2 draft results, got %d", len(results))
	}

	for i, r := range results {
		if r.Tier != "draft" {
			t.Errorf("results[%d] tier = %q, want %q", i, r.Tier, "draft")
		}
	}
}

// TestSemanticSearchFiltered_NoTierFilter verifies that omitting the tier
// filter returns all results (backward compatibility).
func TestSemanticSearchFiltered_NoTierFilter(t *testing.T) {
	s := newTestStoreWithTieredData(t)
	e := newMockEmbedder(true)
	e.vectors["no filter query"] = []float32{0.5, 0.5, 0.5, 0.5}

	sem := NewSemantic(e, s)

	result, _, err := sem.SemanticSearchFiltered(context.Background(), nil, types.SemanticSearchFilteredInput{
		Query:     "no filter query",
		Limit:     10,
		Threshold: 0.0,
	})
	if err != nil {
		t.Fatalf("SemanticSearchFiltered error: %v", err)
	}
	if result.IsError {
		t.Fatalf("SemanticSearchFiltered returned error: %s", resultText(result))
	}

	var results []types.SemanticSearchResult
	text := resultText(result)
	if err := json.Unmarshal([]byte(text), &results); err != nil {
		t.Fatalf("unmarshal results: %v", err)
	}

	// Without tier filter, all 4 pages should appear.
	if len(results) != 4 {
		t.Fatalf("expected 4 results without tier filter, got %d", len(results))
	}
}

// TestSemanticSearchFiltered_TierFilterNoMatch verifies that filtering by
// a tier with no matching pages returns empty results.
func TestSemanticSearchFiltered_TierFilterNoMatch(t *testing.T) {
	s := newTestStoreWithTieredData(t)
	e := newMockEmbedder(true)
	e.vectors["no match query"] = []float32{0.5, 0.5, 0.5, 0.5}

	sem := NewSemantic(e, s)

	// Filter for a tier that doesn't exist in the test data.
	result, _, err := sem.SemanticSearchFiltered(context.Background(), nil, types.SemanticSearchFilteredInput{
		Query:     "no match query",
		Tier:      "nonexistent",
		Limit:     10,
		Threshold: 0.0,
	})
	if err != nil {
		t.Fatalf("SemanticSearchFiltered error: %v", err)
	}
	if result.IsError {
		t.Fatalf("SemanticSearchFiltered returned error: %s", resultText(result))
	}

	var results []types.SemanticSearchResult
	text := resultText(result)
	if err := json.Unmarshal([]byte(text), &results); err != nil {
		t.Fatalf("unmarshal results: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("expected 0 results for nonexistent tier, got %d", len(results))
	}
}

// TestSemanticSearch_NonLearningPageEmptyCategory verifies that non-learning
// pages have empty category in search results.
func TestSemanticSearch_NonLearningPageEmptyCategory(t *testing.T) {
	s := newTestStoreWithTieredData(t)
	e := newMockEmbedder(true)
	// Query vector close to block-spec (authored page, no category).
	e.vectors["spec query"] = []float32{1, 0, 0, 0}

	sem := NewSemantic(e, s)

	result, _, err := sem.SemanticSearch(context.Background(), nil, types.SemanticSearchInput{
		Query:     "spec query",
		Limit:     1,
		Threshold: 0.5,
	})
	if err != nil {
		t.Fatalf("SemanticSearch error: %v", err)
	}
	if result.IsError {
		t.Fatalf("SemanticSearch returned error: %s", resultText(result))
	}

	var results []types.SemanticSearchResult
	text := resultText(result)
	if err := json.Unmarshal([]byte(text), &results); err != nil {
		t.Fatalf("unmarshal results: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}

	// The top result should be the authored-spec page with no category.
	r := results[0]
	if r.Page != "authored-spec" {
		t.Errorf("page = %q, want %q", r.Page, "authored-spec")
	}
	if r.Category != "" {
		t.Errorf("category = %q, want empty for non-learning page", r.Category)
	}
	if r.Tier != "authored" {
		t.Errorf("tier = %q, want %q", r.Tier, "authored")
	}
}

// TestSemanticSearchFiltered_TierWithOtherFilters verifies that tier filtering
// works correctly when combined with other filters (e.g., source_id).
func TestSemanticSearchFiltered_TierWithOtherFilters(t *testing.T) {
	s := newTestStoreWithTieredData(t)
	e := newMockEmbedder(true)
	e.vectors["combined filter"] = []float32{0.5, 0.5, 0.5, 0.5}

	sem := NewSemantic(e, s)

	// Filter for tier=draft AND source_id=learning.
	result, _, err := sem.SemanticSearchFiltered(context.Background(), nil, types.SemanticSearchFilteredInput{
		Query:     "combined filter",
		Tier:      "draft",
		SourceID:  "learning",
		Limit:     10,
		Threshold: 0.0,
	})
	if err != nil {
		t.Fatalf("SemanticSearchFiltered error: %v", err)
	}
	if result.IsError {
		t.Fatalf("SemanticSearchFiltered returned error: %s", resultText(result))
	}

	var results []types.SemanticSearchResult
	text := resultText(result)
	if err := json.Unmarshal([]byte(text), &results); err != nil {
		t.Fatalf("unmarshal results: %v", err)
	}

	// Should return only draft learning pages (auth-1 and perf-1, not validated-learning).
	if len(results) != 2 {
		t.Fatalf("expected 2 results for tier=draft + source_id=learning, got %d", len(results))
	}

	for i, r := range results {
		if r.Tier != "draft" {
			t.Errorf("results[%d] tier = %q, want %q", i, r.Tier, "draft")
		}
		if r.SourceID != "learning" {
			t.Errorf("results[%d] source_id = %q, want %q", i, r.SourceID, "learning")
		}
	}
}

// --- Phase 4 tests: Curated trust tier (015-curated-knowledge-stores) ---

// newTestStoreWithCuratedData creates an in-memory store with pages at all four
// tiers (authored, curated, validated, draft) for testing curated tier filtering.
func newTestStoreWithCuratedData(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	// Insert pages with all four tiers.
	pages := []*store.Page{
		{
			Name: "human-docs", OriginalName: "human-docs",
			SourceID: "disk-local", SourceDocID: "docs.md",
			ContentHash: "h1", CreatedAt: 1000, UpdatedAt: 2000,
			Tier: "authored",
		},
		{
			Name: "curated/auth-patterns", OriginalName: "curated/auth-patterns",
			SourceID: "knowledge-team", SourceDocID: "auth-patterns-1.md",
			ContentHash: "c1", CreatedAt: 3000, UpdatedAt: 4000,
			Tier: "curated", Category: "pattern",
		},
		{
			Name: "curated/deploy-decisions", OriginalName: "curated/deploy-decisions",
			SourceID: "knowledge-team", SourceDocID: "deploy-decisions-1.md",
			ContentHash: "c2", CreatedAt: 5000, UpdatedAt: 6000,
			Tier: "curated", Category: "decision",
		},
		{
			Name: "validated-insight", OriginalName: "validated-insight",
			SourceID: "learning", SourceDocID: "validated-insight",
			ContentHash: "v1", CreatedAt: 7000, UpdatedAt: 8000,
			Tier: "validated", Category: "gotcha",
		},
		{
			Name: "learning/draft-note", OriginalName: "learning/draft-note",
			SourceID: "learning", SourceDocID: "learning/draft-note",
			ContentHash: "d1", CreatedAt: 9000, UpdatedAt: 10000,
			Tier: "draft", Category: "context",
		},
	}
	for _, p := range pages {
		if err := s.InsertPage(p); err != nil {
			t.Fatalf("InsertPage(%s): %v", p.Name, err)
		}
	}

	// Insert blocks — one per page.
	blocks := []*store.Block{
		{UUID: "block-human", PageName: "human-docs", Content: "## Setup\nHuman-written documentation.", HeadingLevel: 2, Position: 0},
		{UUID: "block-curated-auth", PageName: "curated/auth-patterns", Content: "## Auth\nUse OAuth2 for all services.", HeadingLevel: 2, Position: 0},
		{UUID: "block-curated-deploy", PageName: "curated/deploy-decisions", Content: "## Deploy\nBlue-green deployment strategy.", HeadingLevel: 2, Position: 0},
		{UUID: "block-validated", PageName: "validated-insight", Content: "## Gotcha\nWatch for connection pool exhaustion.", HeadingLevel: 2, Position: 0},
		{UUID: "block-draft", PageName: "learning/draft-note", Content: "## Note\nInitial investigation notes.", HeadingLevel: 2, Position: 0},
	}
	for _, b := range blocks {
		if err := s.InsertBlock(b); err != nil {
			t.Fatalf("InsertBlock(%s): %v", b.UUID, err)
		}
	}

	// Insert embeddings with distinct vectors for controlled search results.
	embeddings := []struct {
		uuid  string
		vec   []float32
		chunk string
	}{
		{"block-human", []float32{1, 0, 0, 0, 0}, "human-docs > Setup"},
		{"block-curated-auth", []float32{0, 1, 0, 0, 0}, "curated/auth-patterns > Auth"},
		{"block-curated-deploy", []float32{0, 0, 1, 0, 0}, "curated/deploy-decisions > Deploy"},
		{"block-validated", []float32{0, 0, 0, 1, 0}, "validated-insight > Gotcha"},
		{"block-draft", []float32{0, 0, 0, 0, 1}, "learning/draft-note > Note"},
	}
	for _, e := range embeddings {
		if err := s.InsertEmbedding(e.uuid, "test-model", e.vec, e.chunk); err != nil {
			t.Fatalf("InsertEmbedding(%s): %v", e.uuid, err)
		}
	}

	return s
}

// TestSemanticSearchFiltered_CuratedTierFilter verifies that filtering by
// tier="curated" returns only curated knowledge store pages (FR-022, FR-024).
func TestSemanticSearchFiltered_CuratedTierFilter(t *testing.T) {
	s := newTestStoreWithCuratedData(t)
	e := newMockEmbedder(true)
	// Broad query to match all pages.
	e.vectors["all content"] = []float32{0.4, 0.4, 0.4, 0.4, 0.4}

	sem := NewSemantic(e, s)

	result, _, err := sem.SemanticSearchFiltered(context.Background(), nil, types.SemanticSearchFilteredInput{
		Query:     "all content",
		Tier:      "curated",
		Limit:     10,
		Threshold: 0.0,
	})
	if err != nil {
		t.Fatalf("SemanticSearchFiltered error: %v", err)
	}
	if result.IsError {
		t.Fatalf("SemanticSearchFiltered returned error: %s", resultText(result))
	}

	var results []types.SemanticSearchResult
	text := resultText(result)
	if err := json.Unmarshal([]byte(text), &results); err != nil {
		t.Fatalf("unmarshal results: %v", err)
	}

	// Should return exactly the 2 curated pages.
	if len(results) != 2 {
		t.Fatalf("expected 2 curated results, got %d", len(results))
	}

	// All results must have tier=curated.
	for i, r := range results {
		if r.Tier != "curated" {
			t.Errorf("results[%d] tier = %q, want %q", i, r.Tier, "curated")
		}
	}

	// Verify the curated pages are the ones we expect.
	pageNames := make(map[string]bool)
	for _, r := range results {
		pageNames[r.Page] = true
	}
	if !pageNames["curated/auth-patterns"] {
		t.Error("expected curated/auth-patterns in results")
	}
	if !pageNames["curated/deploy-decisions"] {
		t.Error("expected curated/deploy-decisions in results")
	}
}

// TestSemanticSearchFiltered_AuthoredExcludesCurated verifies that filtering
// by tier="authored" does NOT return curated content (FR-024).
func TestSemanticSearchFiltered_AuthoredExcludesCurated(t *testing.T) {
	s := newTestStoreWithCuratedData(t)
	e := newMockEmbedder(true)
	e.vectors["authored only"] = []float32{0.4, 0.4, 0.4, 0.4, 0.4}

	sem := NewSemantic(e, s)

	result, _, err := sem.SemanticSearchFiltered(context.Background(), nil, types.SemanticSearchFilteredInput{
		Query:     "authored only",
		Tier:      "authored",
		Limit:     10,
		Threshold: 0.0,
	})
	if err != nil {
		t.Fatalf("SemanticSearchFiltered error: %v", err)
	}
	if result.IsError {
		t.Fatalf("SemanticSearchFiltered returned error: %s", resultText(result))
	}

	var results []types.SemanticSearchResult
	text := resultText(result)
	if err := json.Unmarshal([]byte(text), &results); err != nil {
		t.Fatalf("unmarshal results: %v", err)
	}

	// Should return only the 1 authored page.
	if len(results) != 1 {
		t.Fatalf("expected 1 authored result, got %d", len(results))
	}

	if results[0].Tier != "authored" {
		t.Errorf("result tier = %q, want %q", results[0].Tier, "authored")
	}
	if results[0].Page != "human-docs" {
		t.Errorf("result page = %q, want %q", results[0].Page, "human-docs")
	}

	// Verify no curated pages leaked through.
	for _, r := range results {
		if r.Tier == "curated" {
			t.Errorf("curated page %q should not appear with tier=authored filter", r.Page)
		}
	}
}

// TestSemanticSearchFiltered_DraftExcludesCurated verifies that filtering
// by tier="draft" does NOT return curated content.
func TestSemanticSearchFiltered_DraftExcludesCurated(t *testing.T) {
	s := newTestStoreWithCuratedData(t)
	e := newMockEmbedder(true)
	e.vectors["draft only"] = []float32{0.4, 0.4, 0.4, 0.4, 0.4}

	sem := NewSemantic(e, s)

	result, _, err := sem.SemanticSearchFiltered(context.Background(), nil, types.SemanticSearchFilteredInput{
		Query:     "draft only",
		Tier:      "draft",
		Limit:     10,
		Threshold: 0.0,
	})
	if err != nil {
		t.Fatalf("SemanticSearchFiltered error: %v", err)
	}
	if result.IsError {
		t.Fatalf("SemanticSearchFiltered returned error: %s", resultText(result))
	}

	var results []types.SemanticSearchResult
	text := resultText(result)
	if err := json.Unmarshal([]byte(text), &results); err != nil {
		t.Fatalf("unmarshal results: %v", err)
	}

	// Should return only the 1 draft page.
	if len(results) != 1 {
		t.Fatalf("expected 1 draft result, got %d", len(results))
	}

	if results[0].Tier != "draft" {
		t.Errorf("result tier = %q, want %q", results[0].Tier, "draft")
	}

	// Verify no curated pages leaked through.
	for _, r := range results {
		if r.Tier == "curated" {
			t.Errorf("curated page %q should not appear with tier=draft filter", r.Page)
		}
	}
}

// TestSemanticSearchFiltered_CuratedTierMetadata verifies that curated results
// include correct tier and category metadata in the response.
func TestSemanticSearchFiltered_CuratedTierMetadata(t *testing.T) {
	s := newTestStoreWithCuratedData(t)
	e := newMockEmbedder(true)
	e.vectors["curated metadata"] = []float32{0.4, 0.4, 0.4, 0.4, 0.4}

	sem := NewSemantic(e, s)

	result, _, err := sem.SemanticSearchFiltered(context.Background(), nil, types.SemanticSearchFilteredInput{
		Query:     "curated metadata",
		Tier:      "curated",
		Limit:     10,
		Threshold: 0.0,
	})
	if err != nil {
		t.Fatalf("SemanticSearchFiltered error: %v", err)
	}
	if result.IsError {
		t.Fatalf("SemanticSearchFiltered returned error: %s", resultText(result))
	}

	var results []types.SemanticSearchResult
	text := resultText(result)
	if err := json.Unmarshal([]byte(text), &results); err != nil {
		t.Fatalf("unmarshal results: %v", err)
	}

	// Verify each curated result has the expected metadata.
	for _, r := range results {
		if r.Tier != "curated" {
			t.Errorf("page %q tier = %q, want %q", r.Page, r.Tier, "curated")
		}
		if r.Category == "" {
			t.Errorf("page %q category should not be empty for curated content", r.Page)
		}
		if r.CreatedAt == "" {
			t.Errorf("page %q created_at should not be empty", r.Page)
		}
	}

	// Verify specific categories match what was inserted.
	catByPage := make(map[string]string)
	for _, r := range results {
		catByPage[r.Page] = r.Category
	}
	if cat, ok := catByPage["curated/auth-patterns"]; ok && cat != "pattern" {
		t.Errorf("curated/auth-patterns category = %q, want %q", cat, "pattern")
	}
	if cat, ok := catByPage["curated/deploy-decisions"]; ok && cat != "decision" {
		t.Errorf("curated/deploy-decisions category = %q, want %q", cat, "decision")
	}
}

// TestSemanticSearchFiltered_NoTierReturnsAllIncludingCurated verifies that
// omitting the tier filter returns all results including curated pages.
func TestSemanticSearchFiltered_NoTierReturnsAllIncludingCurated(t *testing.T) {
	s := newTestStoreWithCuratedData(t)
	e := newMockEmbedder(true)
	e.vectors["everything"] = []float32{0.4, 0.4, 0.4, 0.4, 0.4}

	sem := NewSemantic(e, s)

	result, _, err := sem.SemanticSearchFiltered(context.Background(), nil, types.SemanticSearchFilteredInput{
		Query:     "everything",
		Limit:     10,
		Threshold: 0.0,
	})
	if err != nil {
		t.Fatalf("SemanticSearchFiltered error: %v", err)
	}
	if result.IsError {
		t.Fatalf("SemanticSearchFiltered returned error: %s", resultText(result))
	}

	var results []types.SemanticSearchResult
	text := resultText(result)
	if err := json.Unmarshal([]byte(text), &results); err != nil {
		t.Fatalf("unmarshal results: %v", err)
	}

	// Without tier filter, all 5 pages should appear.
	if len(results) != 5 {
		t.Fatalf("expected 5 results without tier filter, got %d", len(results))
	}

	// Verify curated pages are present in unfiltered results.
	tiers := make(map[string]int)
	for _, r := range results {
		tiers[r.Tier]++
	}
	if tiers["curated"] != 2 {
		t.Errorf("expected 2 curated results in unfiltered query, got %d", tiers["curated"])
	}
	if tiers["authored"] != 1 {
		t.Errorf("expected 1 authored result, got %d", tiers["authored"])
	}
	if tiers["validated"] != 1 {
		t.Errorf("expected 1 validated result, got %d", tiers["validated"])
	}
	if tiers["draft"] != 1 {
		t.Errorf("expected 1 draft result, got %d", tiers["draft"])
	}
}
