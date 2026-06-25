package vault

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/unbound-force/dewey/v3/embed"
	"github.com/unbound-force/dewey/v3/store"
	"github.com/unbound-force/dewey/v3/types"
)

func TestParseDocument_WithHeadingsAndFrontmatter(t *testing.T) {
	content := `---
title: Test Doc
tags:
  - go
---
# Introduction

Welcome to the test document.

## Details

Some details here.
`
	props, blocks := ParseDocument("test-doc", content)

	// Verify frontmatter was parsed.
	if props == nil {
		t.Fatal("expected non-nil properties")
	}
	if title, ok := props["title"]; !ok || title != "Test Doc" {
		t.Errorf("props[title] = %v, want 'Test Doc'", props["title"])
	}

	// Verify blocks were created from headings.
	if len(blocks) < 1 {
		t.Fatalf("expected at least 1 root block, got %d", len(blocks))
	}

	// First root block should contain "Introduction".
	if !strings.Contains(blocks[0].Content, "Introduction") {
		t.Errorf("first block content = %q, want to contain 'Introduction'", blocks[0].Content)
	}
}

func TestParseDocument_PlainTextNoHeadings(t *testing.T) {
	content := "Just some plain text without any headings.\nAnother line."
	props, blocks := ParseDocument("plain-doc", content)

	// No frontmatter → nil props.
	if props != nil {
		t.Errorf("expected nil properties for plain text, got %v", props)
	}

	// Should still produce blocks (one root block with the content).
	if len(blocks) == 0 {
		t.Fatal("expected at least one block for plain text")
	}

	// Block should contain the input text.
	if !strings.Contains(blocks[0].Content, "Just some plain text") {
		t.Errorf("block content = %q, want to contain input text", blocks[0].Content)
	}
}

func TestParseDocument_EmptyContent(t *testing.T) {
	props, blocks := ParseDocument("empty-doc", "")

	if props != nil {
		t.Errorf("expected nil properties for empty content, got %v", props)
	}
	if len(blocks) != 0 {
		t.Errorf("expected no blocks for empty content, got %d", len(blocks))
	}
}

func TestParseDocument_FrontmatterOnly(t *testing.T) {
	content := `---
title: Metadata Only
---
`
	props, blocks := ParseDocument("meta-doc", content)

	if props == nil {
		t.Fatal("expected non-nil properties")
	}
	if title, ok := props["title"]; !ok || title != "Metadata Only" {
		t.Errorf("props[title] = %v, want 'Metadata Only'", props["title"])
	}

	// Body after frontmatter is empty/whitespace → no blocks.
	if len(blocks) != 0 {
		t.Errorf("expected no blocks for frontmatter-only content, got %d", len(blocks))
	}
}

func TestParseDocument_NestedHeadings(t *testing.T) {
	content := `# Top Level

## Sub Section

### Sub Sub Section

Content here.
`
	_, blocks := ParseDocument("nested-doc", content)

	if len(blocks) != 1 {
		t.Fatalf("expected 1 root block (H1), got %d", len(blocks))
	}

	// H1 should contain "Top Level".
	if !strings.Contains(blocks[0].Content, "Top Level") {
		t.Errorf("root block content = %q, want to contain 'Top Level'", blocks[0].Content)
	}

	// H1 should have H2 as child.
	if len(blocks[0].Children) != 1 {
		t.Fatalf("expected 1 child of H1 (H2), got %d", len(blocks[0].Children))
	}

	// H2 should contain "Sub Section".
	if !strings.Contains(blocks[0].Children[0].Content, "Sub Section") {
		t.Errorf("H2 block content = %q, want to contain 'Sub Section'", blocks[0].Children[0].Content)
	}

	// H2 should have H3 as child.
	if len(blocks[0].Children[0].Children) != 1 {
		t.Fatalf("expected 1 child of H2 (H3), got %d", len(blocks[0].Children[0].Children))
	}

	// H3 should contain "Sub Sub Section".
	if !strings.Contains(blocks[0].Children[0].Children[0].Content, "Sub Sub Section") {
		t.Errorf("H3 block content = %q, want to contain 'Sub Sub Section'", blocks[0].Children[0].Children[0].Content)
	}
}

// --- Mock embedder for GenerateEmbeddings tests ---

// testEmbedder implements embed.Embedder for testing batch embedding behavior.
// Tracks call counts for Embed and EmbedBatch to verify batching strategy.
type testEmbedder struct {
	embedCalls      atomic.Int64
	embedBatchCalls atomic.Int64
	batchErr        error // If set, EmbedBatch returns this error (triggers fallback).
	embedErr        error // If set, Embed returns this error.
}

func (e *testEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	e.embedCalls.Add(1)
	if e.embedErr != nil {
		return nil, e.embedErr
	}
	return []float32{0.1, 0.2, 0.3}, nil
}

func (e *testEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	e.embedBatchCalls.Add(1)
	if e.batchErr != nil {
		return nil, e.batchErr
	}
	result := make([][]float32, len(texts))
	for i := range texts {
		result[i] = []float32{0.1, 0.2, 0.3}
	}
	return result, nil
}

func (e *testEmbedder) Available() bool { return true }
func (e *testEmbedder) ModelID() string { return "test-model" }

var _ embed.Embedder = (*testEmbedder)(nil)

// newTestStore creates an in-memory store for testing. Calls t.Fatal on error.
func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New(:memory:): %v", err)
	}
	// Insert a page so InsertEmbedding foreign key (if any) is satisfied.
	if err := s.InsertPage(&store.Page{Name: "test-page"}); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}
	return s
}

// makeBlocks creates n blocks with unique UUIDs and non-empty content.
// Blocks are flat (no children) unless withChildren is called separately.
func makeBlocks(n int) []types.BlockEntity {
	blocks := make([]types.BlockEntity, n)
	for i := range blocks {
		blocks[i] = types.BlockEntity{
			UUID:    fmt.Sprintf("block-%03d", i),
			Content: fmt.Sprintf("Content for block %d with some text.", i),
		}
	}
	return blocks
}

// TestGenerateEmbeddings_BatchReducesRoundTrips verifies that EmbedBatch()
// is called instead of per-block Embed() calls (FR-100 scenario 1).
func TestGenerateEmbeddings_BatchReducesRoundTrips(t *testing.T) {
	s := newTestStore(t)
	e := &testEmbedder{}

	// 64 blocks should produce 2 batches of 32 (not 64 individual Embed calls).
	blocks := makeBlocks(64)
	// Insert blocks into the store so embedding persistence works.
	for _, b := range blocks {
		if err := s.InsertBlock(&store.Block{
			UUID:     b.UUID,
			PageName: "test-page",
			Content:  b.Content,
		}); err != nil {
			t.Fatalf("InsertBlock: %v", err)
		}
	}

	count := GenerateEmbeddings(s, e, "test-page", blocks, nil, embed.DefaultMaxChunkChars)

	if count != 64 {
		t.Errorf("GenerateEmbeddings returned %d, want 64", count)
	}
	if got := e.embedBatchCalls.Load(); got != 2 {
		t.Errorf("EmbedBatch called %d times, want 2", got)
	}
	if got := e.embedCalls.Load(); got != 0 {
		t.Errorf("Embed called %d times, want 0 (should use batch)", got)
	}
}

// TestGenerateEmbeddings_BatchFallback verifies that when EmbedBatch() fails,
// individual Embed() calls are used as fallback (FR-100 scenario 2).
func TestGenerateEmbeddings_BatchFallback(t *testing.T) {
	s := newTestStore(t)
	e := &testEmbedder{
		batchErr: fmt.Errorf("simulated context length overflow"),
	}

	blocks := makeBlocks(5)
	for _, b := range blocks {
		if err := s.InsertBlock(&store.Block{
			UUID:     b.UUID,
			PageName: "test-page",
			Content:  b.Content,
		}); err != nil {
			t.Fatalf("InsertBlock: %v", err)
		}
	}

	count := GenerateEmbeddings(s, e, "test-page", blocks, nil, embed.DefaultMaxChunkChars)

	if count != 5 {
		t.Errorf("GenerateEmbeddings returned %d, want 5", count)
	}
	// Batch was attempted once and failed.
	if got := e.embedBatchCalls.Load(); got != 1 {
		t.Errorf("EmbedBatch called %d times, want 1", got)
	}
	// Fallback: each block embedded individually.
	if got := e.embedCalls.Load(); got != 5 {
		t.Errorf("Embed called %d times, want 5 (fallback)", got)
	}
}

// TestGenerateEmbeddings_EmptyBlocksSkipped verifies that empty/whitespace
// blocks are not sent for embedding (FR-100 scenario 3).
func TestGenerateEmbeddings_EmptyBlocksSkipped(t *testing.T) {
	s := newTestStore(t)
	e := &testEmbedder{}

	blocks := []types.BlockEntity{
		{UUID: "b1", Content: "Real content here."},
		{UUID: "b2", Content: ""},
		{UUID: "b3", Content: "   "},
		{UUID: "b4", Content: "\t\n"},
		{UUID: "b5", Content: "Another real block."},
		{UUID: "b6", Content: "Third real block."},
		{UUID: "b7", Content: "   \n   "},
		{UUID: "b8", Content: "Fourth real block."},
		{UUID: "b9", Content: "Fifth real block."},
		{UUID: "b10", Content: "Sixth real block."},
	}

	// Only insert non-empty blocks (the ones that will actually be embedded).
	for _, b := range blocks {
		if strings.TrimSpace(b.Content) == "" {
			continue
		}
		if err := s.InsertBlock(&store.Block{
			UUID:     b.UUID,
			PageName: "test-page",
			Content:  b.Content,
		}); err != nil {
			t.Fatalf("InsertBlock: %v", err)
		}
	}

	count := GenerateEmbeddings(s, e, "test-page", blocks, nil, embed.DefaultMaxChunkChars)

	// 10 blocks, 4 empty → 6 should be embedded.
	if count != 6 {
		t.Errorf("GenerateEmbeddings returned %d, want 6 (4 empty skipped)", count)
	}
}

// TestGenerateEmbeddings_CorrectCount verifies the total embedding count
// matches the number of non-empty blocks including children.
func TestGenerateEmbeddings_CorrectCount(t *testing.T) {
	s := newTestStore(t)
	e := &testEmbedder{}

	// Tree: 2 root blocks, first has 3 children, second has 1 child.
	// Total non-empty: 6.
	blocks := []types.BlockEntity{
		{
			UUID:    "root1",
			Content: "# Heading One",
			Children: []types.BlockEntity{
				{UUID: "child1a", Content: "Child 1a content."},
				{UUID: "child1b", Content: "Child 1b content."},
				{UUID: "child1c", Content: "Child 1c content."},
			},
		},
		{
			UUID:    "root2",
			Content: "## Heading Two",
			Children: []types.BlockEntity{
				{UUID: "child2a", Content: "Child 2a content."},
			},
		},
	}

	// Insert all blocks into store.
	allBlocks := []types.BlockEntity{
		blocks[0], blocks[0].Children[0], blocks[0].Children[1], blocks[0].Children[2],
		blocks[1], blocks[1].Children[0],
	}
	for _, b := range allBlocks {
		if err := s.InsertBlock(&store.Block{
			UUID:     b.UUID,
			PageName: "test-page",
			Content:  b.Content,
		}); err != nil {
			t.Fatalf("InsertBlock: %v", err)
		}
	}

	count := GenerateEmbeddings(s, e, "test-page", blocks, nil, embed.DefaultMaxChunkChars)

	if count != 6 {
		t.Errorf("GenerateEmbeddings returned %d, want 6", count)
	}
}

// TestFlattenBlocks_CollectsAllNonEmpty verifies the flatten pass collects
// blocks from all tree levels and skips empty ones.
func TestFlattenBlocks_CollectsAllNonEmpty(t *testing.T) {
	blocks := []types.BlockEntity{
		{
			UUID:    "r1",
			Content: "# Top",
			Children: []types.BlockEntity{
				{UUID: "c1", Content: "Child content."},
				{UUID: "c2", Content: ""},
				{UUID: "c3", Content: "Another child."},
			},
		},
		{UUID: "r2", Content: "   "},
		{UUID: "r3", Content: "Root three."},
	}

	var chunks []blockChunk
	flattenBlocks(blocks, nil, "page", &chunks, embed.DefaultMaxChunkChars)

	// r1 (Top), c1, c3, r3 = 4 non-empty. r2 and c2 are empty/whitespace.
	if len(chunks) != 4 {
		t.Errorf("flattenBlocks collected %d chunks, want 4", len(chunks))
	}
}
