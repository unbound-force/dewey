package store

import (
	"fmt"
	"math"
	"testing"
)

// TestInsertAndGetEmbedding verifies basic insert/get round-trip.
func TestInsertAndGetEmbedding(t *testing.T) {
	s := newTestStore(t)

	// Insert a page and block first (FK constraint).
	if err := s.InsertPage(testPage("test-page")); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}
	if err := s.InsertBlock(testBlock("block-1", "test-page")); err != nil {
		t.Fatalf("InsertBlock: %v", err)
	}

	vec := []float32{0.1, 0.2, 0.3, 0.4}
	if err := s.InsertEmbedding("block-1", "granite-embedding:30m", vec, "test chunk text"); err != nil {
		t.Fatalf("InsertEmbedding: %v", err)
	}

	got, err := s.GetEmbedding("block-1", "granite-embedding:30m")
	if err != nil {
		t.Fatalf("GetEmbedding: %v", err)
	}
	if got == nil {
		t.Fatal("GetEmbedding returned nil")
	}

	if got.BlockUUID != "block-1" {
		t.Errorf("BlockUUID = %q, want %q", got.BlockUUID, "block-1")
	}
	if got.ModelID != "granite-embedding:30m" {
		t.Errorf("ModelID = %q, want %q", got.ModelID, "granite-embedding:30m")
	}
	if got.ChunkText != "test chunk text" {
		t.Errorf("ChunkText = %q, want %q", got.ChunkText, "test chunk text")
	}
	if len(got.Vector) != 4 {
		t.Fatalf("Vector length = %d, want 4", len(got.Vector))
	}

	// Verify vector values survive serialization round-trip.
	for i, v := range vec {
		if got.Vector[i] != v {
			t.Errorf("Vector[%d] = %f, want %f", i, got.Vector[i], v)
		}
	}
}

// TestGetEmbedding_NotFound verifies nil is returned for missing embeddings.
func TestGetEmbedding_NotFound(t *testing.T) {
	s := newTestStore(t)

	got, err := s.GetEmbedding("nonexistent", "model")
	if err != nil {
		t.Fatalf("GetEmbedding: %v", err)
	}
	if got != nil {
		t.Errorf("GetEmbedding(nonexistent) = %v, want nil", got)
	}
}

// TestInsertEmbedding_Upsert verifies that inserting an embedding for the
// same block+model replaces the existing one.
func TestInsertEmbedding_Upsert(t *testing.T) {
	s := newTestStore(t)
	if err := s.InsertPage(testPage("test-page")); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}
	if err := s.InsertBlock(testBlock("block-1", "test-page")); err != nil {
		t.Fatalf("InsertBlock: %v", err)
	}

	// Insert first embedding.
	vec1 := []float32{1.0, 2.0}
	if err := s.InsertEmbedding("block-1", "model-a", vec1, "chunk v1"); err != nil {
		t.Fatalf("InsertEmbedding v1: %v", err)
	}

	// Upsert with new vector.
	vec2 := []float32{3.0, 4.0}
	if err := s.InsertEmbedding("block-1", "model-a", vec2, "chunk v2"); err != nil {
		t.Fatalf("InsertEmbedding v2: %v", err)
	}

	got, err := s.GetEmbedding("block-1", "model-a")
	if err != nil {
		t.Fatalf("GetEmbedding: %v", err)
	}
	if got.ChunkText != "chunk v2" {
		t.Errorf("ChunkText = %q, want %q (upsert should replace)", got.ChunkText, "chunk v2")
	}
	if got.Vector[0] != 3.0 {
		t.Errorf("Vector[0] = %f, want 3.0 (upsert should replace)", got.Vector[0])
	}
}

// TestGetAllEmbeddings verifies listing all embeddings for a model.
func TestGetAllEmbeddings(t *testing.T) {
	s := newTestStore(t)
	if err := s.InsertPage(testPage("test-page")); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}
	if err := s.InsertBlock(testBlock("block-1", "test-page")); err != nil {
		t.Fatalf("InsertBlock: %v", err)
	}
	if err := s.InsertBlock(testBlock("block-2", "test-page")); err != nil {
		t.Fatalf("InsertBlock: %v", err)
	}

	s.InsertEmbedding("block-1", "model-a", []float32{1, 2}, "chunk 1")
	s.InsertEmbedding("block-2", "model-a", []float32{3, 4}, "chunk 2")
	s.InsertEmbedding("block-1", "model-b", []float32{5, 6}, "chunk 1 model b")

	embeddings, err := s.GetAllEmbeddings("model-a")
	if err != nil {
		t.Fatalf("GetAllEmbeddings: %v", err)
	}
	if len(embeddings) != 2 {
		t.Fatalf("GetAllEmbeddings(model-a) returned %d, want 2", len(embeddings))
	}

	// model-b should have only 1.
	embeddingsB, err := s.GetAllEmbeddings("model-b")
	if err != nil {
		t.Fatalf("GetAllEmbeddings(model-b): %v", err)
	}
	if len(embeddingsB) != 1 {
		t.Fatalf("GetAllEmbeddings(model-b) returned %d, want 1", len(embeddingsB))
	}
}

// TestDeleteEmbeddingsByBlock verifies deletion by block UUID.
func TestDeleteEmbeddingsByBlock(t *testing.T) {
	s := newTestStore(t)
	if err := s.InsertPage(testPage("test-page")); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}
	if err := s.InsertBlock(testBlock("block-1", "test-page")); err != nil {
		t.Fatalf("InsertBlock: %v", err)
	}

	s.InsertEmbedding("block-1", "model-a", []float32{1, 2}, "chunk")
	s.InsertEmbedding("block-1", "model-b", []float32{3, 4}, "chunk")

	if err := s.DeleteEmbeddingsByBlock("block-1"); err != nil {
		t.Fatalf("DeleteEmbeddingsByBlock: %v", err)
	}

	// Both model embeddings should be gone.
	got, _ := s.GetEmbedding("block-1", "model-a")
	if got != nil {
		t.Error("embedding for model-a should be deleted")
	}
	got, _ = s.GetEmbedding("block-1", "model-b")
	if got != nil {
		t.Error("embedding for model-b should be deleted")
	}
}

// TestDeleteEmbeddingsByModel verifies deletion by model ID.
func TestDeleteEmbeddingsByModel(t *testing.T) {
	s := newTestStore(t)
	if err := s.InsertPage(testPage("test-page")); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}
	if err := s.InsertBlock(testBlock("block-1", "test-page")); err != nil {
		t.Fatalf("InsertBlock: %v", err)
	}
	if err := s.InsertBlock(testBlock("block-2", "test-page")); err != nil {
		t.Fatalf("InsertBlock: %v", err)
	}

	s.InsertEmbedding("block-1", "model-a", []float32{1, 2}, "chunk 1")
	s.InsertEmbedding("block-2", "model-a", []float32{3, 4}, "chunk 2")
	s.InsertEmbedding("block-1", "model-b", []float32{5, 6}, "chunk 1b")

	if err := s.DeleteEmbeddingsByModel("model-a"); err != nil {
		t.Fatalf("DeleteEmbeddingsByModel: %v", err)
	}

	// model-a embeddings should be gone.
	all, _ := s.GetAllEmbeddings("model-a")
	if len(all) != 0 {
		t.Errorf("model-a embeddings remaining: %d, want 0", len(all))
	}

	// model-b should still exist.
	got, _ := s.GetEmbedding("block-1", "model-b")
	if got == nil {
		t.Error("model-b embedding should still exist")
	}
}

// TestCountEmbeddings verifies the embedding count.
func TestCountEmbeddings(t *testing.T) {
	s := newTestStore(t)
	if err := s.InsertPage(testPage("test-page")); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}
	if err := s.InsertBlock(testBlock("block-1", "test-page")); err != nil {
		t.Fatalf("InsertBlock: %v", err)
	}
	if err := s.InsertBlock(testBlock("block-2", "test-page")); err != nil {
		t.Fatalf("InsertBlock: %v", err)
	}

	count, err := s.CountEmbeddings()
	if err != nil {
		t.Fatalf("CountEmbeddings: %v", err)
	}
	if count != 0 {
		t.Errorf("CountEmbeddings() = %d, want 0", count)
	}

	s.InsertEmbedding("block-1", "model-a", []float32{1, 2}, "chunk 1")
	s.InsertEmbedding("block-2", "model-a", []float32{3, 4}, "chunk 2")

	count, err = s.CountEmbeddings()
	if err != nil {
		t.Fatalf("CountEmbeddings: %v", err)
	}
	if count != 2 {
		t.Errorf("CountEmbeddings() = %d, want 2", count)
	}
}

// TestCosineSimilarity verifies cosine similarity computation with known vectors.
func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name string
		a, b []float32
		want float64
	}{
		{
			name: "identical vectors",
			a:    []float32{1, 0, 0},
			b:    []float32{1, 0, 0},
			want: 1.0,
		},
		{
			name: "orthogonal vectors",
			a:    []float32{1, 0, 0},
			b:    []float32{0, 1, 0},
			want: 0.0,
		},
		{
			name: "opposite vectors",
			a:    []float32{1, 0, 0},
			b:    []float32{-1, 0, 0},
			want: -1.0,
		},
		{
			name: "45 degree angle",
			a:    []float32{1, 0},
			b:    []float32{1, 1},
			want: 1.0 / math.Sqrt(2),
		},
		{
			name: "zero vector a",
			a:    []float32{0, 0, 0},
			b:    []float32{1, 2, 3},
			want: 0.0,
		},
		{
			name: "zero vector b",
			a:    []float32{1, 2, 3},
			b:    []float32{0, 0, 0},
			want: 0.0,
		},
		{
			name: "both zero",
			a:    []float32{0, 0},
			b:    []float32{0, 0},
			want: 0.0,
		},
		{
			name: "different lengths",
			a:    []float32{1, 2},
			b:    []float32{1, 2, 3},
			want: 0.0,
		},
		{
			name: "empty vectors",
			a:    []float32{},
			b:    []float32{},
			want: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cosineSimilarity(tt.a, tt.b)
			if math.Abs(got-tt.want) > 1e-6 {
				t.Errorf("cosineSimilarity(%v, %v) = %f, want %f", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// TestSearchSimilar verifies brute-force cosine search returns correctly
// ranked results.
func TestSearchSimilar(t *testing.T) {
	s := newTestStore(t)
	if err := s.InsertPage(testPage("test-page")); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	// Insert blocks with known vectors.
	blocks := []struct {
		uuid string
		vec  []float32
		text string
	}{
		{"block-a", []float32{1, 0, 0}, "about apples"},
		{"block-b", []float32{0, 1, 0}, "about bananas"},
		{"block-c", []float32{0.9, 0.1, 0}, "mostly about apples"},
		{"block-d", []float32{0.1, 0.9, 0}, "mostly about bananas"},
	}

	for _, b := range blocks {
		if err := s.InsertBlock(testBlock(b.uuid, "test-page")); err != nil {
			t.Fatalf("InsertBlock(%s): %v", b.uuid, err)
		}
		if err := s.InsertEmbedding(b.uuid, "model-a", b.vec, b.text); err != nil {
			t.Fatalf("InsertEmbedding(%s): %v", b.uuid, err)
		}
	}

	// Search for something similar to [1, 0, 0] (apples).
	query := []float32{1, 0, 0}
	results, err := s.SearchSimilar("model-a", query, 10, 0.0)
	if err != nil {
		t.Fatalf("SearchSimilar: %v", err)
	}

	if len(results) != 4 {
		t.Fatalf("SearchSimilar returned %d results, want 4", len(results))
	}

	// First result should be block-a (identical vector, similarity = 1.0).
	if results[0].BlockUUID != "block-a" {
		t.Errorf("results[0].BlockUUID = %q, want %q", results[0].BlockUUID, "block-a")
	}
	if math.Abs(results[0].Similarity-1.0) > 1e-6 {
		t.Errorf("results[0].Similarity = %f, want 1.0", results[0].Similarity)
	}

	// Second should be block-c (0.9, 0.1, 0).
	if results[1].BlockUUID != "block-c" {
		t.Errorf("results[1].BlockUUID = %q, want %q", results[1].BlockUUID, "block-c")
	}

	// Last should be block-b (orthogonal, similarity = 0.0).
	if results[3].BlockUUID != "block-b" {
		t.Errorf("results[3].BlockUUID = %q, want %q", results[3].BlockUUID, "block-b")
	}
}

// TestSearchSimilar_Threshold verifies the threshold filter.
func TestSearchSimilar_Threshold(t *testing.T) {
	s := newTestStore(t)
	if err := s.InsertPage(testPage("test-page")); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	blocks := []struct {
		uuid string
		vec  []float32
	}{
		{"block-a", []float32{1, 0, 0}},
		{"block-b", []float32{0, 1, 0}},
		{"block-c", []float32{0.9, 0.1, 0}},
	}

	for _, b := range blocks {
		s.InsertBlock(testBlock(b.uuid, "test-page"))
		s.InsertEmbedding(b.uuid, "model-a", b.vec, "text")
	}

	// Search with high threshold — should only return very similar results.
	results, err := s.SearchSimilar("model-a", []float32{1, 0, 0}, 10, 0.9)
	if err != nil {
		t.Fatalf("SearchSimilar: %v", err)
	}

	// block-a (1.0) and block-c (~0.994) should pass, block-b (0.0) should not.
	if len(results) != 2 {
		t.Errorf("SearchSimilar with threshold 0.9 returned %d results, want 2", len(results))
	}
}

// TestSearchSimilar_Limit verifies the limit parameter.
func TestSearchSimilar_Limit(t *testing.T) {
	s := newTestStore(t)
	if err := s.InsertPage(testPage("test-page")); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	for i := 0; i < 5; i++ {
		uuid := fmt.Sprintf("block-%d", i)
		s.InsertBlock(testBlock(uuid, "test-page"))
		vec := make([]float32, 3)
		vec[0] = float32(i) * 0.2
		s.InsertEmbedding(uuid, "model-a", vec, "text")
	}

	results, err := s.SearchSimilar("model-a", []float32{1, 0, 0}, 2, 0.0)
	if err != nil {
		t.Fatalf("SearchSimilar: %v", err)
	}

	if len(results) > 2 {
		t.Errorf("SearchSimilar with limit 2 returned %d results, want <= 2", len(results))
	}
}

// TestSearchSimilar_EmptyIndex verifies behavior with no embeddings.
func TestSearchSimilar_EmptyIndex(t *testing.T) {
	s := newTestStore(t)

	results, err := s.SearchSimilar("model-a", []float32{1, 0, 0}, 10, 0.0)
	if err != nil {
		t.Fatalf("SearchSimilar: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("SearchSimilar on empty index returned %d results, want 0", len(results))
	}
}

// TestSearchSimilarFiltered verifies filtered search with metadata.
func TestSearchSimilarFiltered(t *testing.T) {
	s := newTestStore(t)

	// Create two pages with different source IDs.
	page1 := testPage("page-disk")
	page1.SourceID = "disk-local"
	s.InsertPage(page1)

	page2 := testPage("page-github")
	page2.Name = "page-github"
	page2.OriginalName = "page-github"
	page2.SourceID = "github-gaze"
	page2.SourceDocID = "page-github.md"
	s.InsertPage(page2)

	s.InsertBlock(testBlock("block-disk", "page-disk"))
	s.InsertBlock(testBlock("block-github", "page-github"))

	s.InsertEmbedding("block-disk", "model-a", []float32{1, 0, 0}, "disk content")
	s.InsertEmbedding("block-github", "model-a", []float32{0.9, 0.1, 0}, "github content")

	// Filter by source_id = "disk-local".
	filters := SearchFilters{SourceID: "disk-local"}
	results, err := s.SearchSimilarFiltered("model-a", []float32{1, 0, 0}, filters, 10, 0.0)
	if err != nil {
		t.Fatalf("SearchSimilarFiltered: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("filtered search returned %d results, want 1", len(results))
	}
	if results[0].BlockUUID != "block-disk" {
		t.Errorf("filtered result = %q, want %q", results[0].BlockUUID, "block-disk")
	}
}

// TestSerializeDeserializeVector verifies vector serialization round-trip.
func TestSerializeDeserializeVector(t *testing.T) {
	original := []float32{0.1, -0.2, 0.3, 1.0, -1.0, 0.0}
	blob := serializeVector(original)
	restored := deserializeVector(blob)

	if len(restored) != len(original) {
		t.Fatalf("restored length = %d, want %d", len(restored), len(original))
	}

	for i, v := range original {
		if restored[i] != v {
			t.Errorf("restored[%d] = %f, want %f", i, restored[i], v)
		}
	}
}

// TestDeserializeVector_InvalidLength verifies handling of invalid blob length.
func TestDeserializeVector_InvalidLength(t *testing.T) {
	// 5 bytes is not divisible by 4.
	result := deserializeVector([]byte{1, 2, 3, 4, 5})
	if result != nil {
		t.Errorf("deserializeVector(5 bytes) = %v, want nil", result)
	}
}

// TestInferSourceType verifies source type extraction from source ID.
func TestInferSourceType(t *testing.T) {
	tests := []struct {
		sourceID string
		want     string
	}{
		{"disk-local", "disk"},
		{"github-gaze", "github"},
		{"web-docs", "web"},
		{"standalone", "standalone"},
	}

	for _, tt := range tests {
		got := inferSourceType(tt.sourceID)
		if got != tt.want {
			t.Errorf("inferSourceType(%q) = %q, want %q", tt.sourceID, got, tt.want)
		}
	}
}

// TestCascadeDeleteEmbeddings verifies that deleting a page cascades to
// delete its blocks' embeddings.
func TestCascadeDeleteEmbeddings(t *testing.T) {
	s := newTestStore(t)
	if err := s.InsertPage(testPage("test-page")); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}
	if err := s.InsertBlock(testBlock("block-1", "test-page")); err != nil {
		t.Fatalf("InsertBlock: %v", err)
	}
	s.InsertEmbedding("block-1", "model-a", []float32{1, 2}, "chunk")

	// Delete the page — should cascade to blocks and embeddings.
	if err := s.DeletePage("test-page"); err != nil {
		t.Fatalf("DeletePage: %v", err)
	}

	got, _ := s.GetEmbedding("block-1", "model-a")
	if got != nil {
		t.Error("embedding should be cascade-deleted with page")
	}
}

// --- Test helpers ---

// testBlock returns a Block with sensible defaults for testing.
func testBlock(uuid, pageName string) *Block {
	return &Block{
		UUID:         uuid,
		PageName:     pageName,
		Content:      "test content for " + uuid,
		HeadingLevel: 0,
		Position:     0,
	}
}
