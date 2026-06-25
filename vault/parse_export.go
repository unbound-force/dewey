package vault

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/unbound-force/dewey/v3/embed"
	"github.com/unbound-force/dewey/v3/parser"
	"github.com/unbound-force/dewey/v3/store"
	"github.com/unbound-force/dewey/v3/types"
)

// ParseDocument parses a markdown document's content into frontmatter properties
// and a hierarchical block tree. This is the exported entry point for external
// callers (e.g., `dewey index`) that need to parse document content without
// requiring a vault.Client instance or filesystem access.
//
// The docID parameter is used as a seed for deterministic block UUID generation.
//
// Design decision: Wraps the unexported parseFrontmatter() and parseMarkdownBlocks()
// rather than exporting them directly, to provide a clean API boundary and avoid
// coupling callers to internal parsing details (per research R2).
func ParseDocument(docID, content string) (props map[string]any, blocks []types.BlockEntity) {
	props, body := parseFrontmatter(content)
	blocks = parseMarkdownBlocks(docID, body)
	return props, blocks
}

// PersistBlocks recursively inserts blocks into the store.
// This is the shared implementation used by both VaultStore.persistBlocks()
// and the CLI indexing pipeline, eliminating duplication (Architect DRY finding).
func PersistBlocks(s *store.Store, pageName string, blocks []types.BlockEntity, parentUUID sql.NullString, startPos int) error {
	for i, b := range blocks {
		hl := HeadingLevelFromContent(b.Content)
		logger.Debug("inserting block", "page", pageName, "uuid", b.UUID, "headingLevel", hl, "position", startPos+i)
		sb := &store.Block{
			UUID:         b.UUID,
			PageName:     pageName,
			ParentUUID:   parentUUID,
			Content:      b.Content,
			HeadingLevel: hl,
			Position:     startPos + i,
		}
		if err := s.InsertBlock(sb); err != nil {
			return fmt.Errorf("insert block %q: %w", b.UUID, err)
		}

		if len(b.Children) > 0 {
			childParent := sql.NullString{String: b.UUID, Valid: true}
			if err := PersistBlocks(s, pageName, b.Children, childParent, 0); err != nil {
				return err
			}
		}
	}
	return nil
}

// PersistLinks extracts wikilinks from blocks and persists them to the store.
// This is the shared implementation used by both VaultStore.persistLinks()
// and the CLI indexing pipeline, eliminating duplication (Architect DRY finding).
func PersistLinks(s *store.Store, pageName string, blocks []types.BlockEntity) error {
	for _, b := range blocks {
		parsed := parser.Parse(b.Content)
		for _, link := range parsed.Links {
			sl := &store.Link{
				FromPage:  pageName,
				ToPage:    link,
				BlockUUID: b.UUID,
			}
			if err := s.InsertLink(sl); err != nil {
				return fmt.Errorf("insert link %q -> %q: %w", pageName, link, err)
			}
		}

		if len(b.Children) > 0 {
			if err := PersistLinks(s, pageName, b.Children); err != nil {
				return err
			}
		}
	}
	return nil
}

// HeadingLevelFromContent returns the markdown heading level (1-6) for a block's
// content, or 0 if the content does not start with a heading. Examines only the
// first line of multi-line content.
func HeadingLevelFromContent(content string) int {
	firstLine := content
	if idx := strings.IndexByte(content, '\n'); idx >= 0 {
		firstLine = content[:idx]
	}
	return headingLevel(firstLine)
}

// ExtractHeadingFromContent returns the heading text (without # prefix) from a
// block's content, or empty string if not a heading. Examines only the first line.
// This is the single implementation of heading extraction — used by both
// VaultStore.generateEmbeddings() and the CLI indexing pipeline.
func ExtractHeadingFromContent(content string) string {
	firstLine := content
	if idx := strings.IndexByte(content, '\n'); idx >= 0 {
		firstLine = content[:idx]
	}
	firstLine = strings.TrimSpace(firstLine)
	if !strings.HasPrefix(firstLine, "#") {
		return ""
	}
	trimmed := strings.TrimLeft(firstLine, "#")
	return strings.TrimSpace(trimmed)
}

// embeddingBatchSize is the number of chunks to batch per EmbedBatch() call.
// Balances memory usage (32 chunks * up to maxChunkChars each) against HTTP round-trip
// reduction. Ollama's /api/embed endpoint accepts arrays natively (D1).
const embeddingBatchSize = 32

// blockChunk associates a block UUID with its prepared chunk text and heading
// path for batch embedding. Collected during the flatten pass and consumed
// during the batch-embed pass (D2).
type blockChunk struct {
	blockUUID   string
	chunk       string
	headingPath []string
}

// GenerateEmbeddings creates vector embeddings for blocks and persists them
// to the store. This is the shared implementation used by both VaultStore
// (serve-time persistence) and the CLI indexing pipeline, eliminating
// duplication (same pattern as PersistBlocks/PersistLinks).
//
// Uses a flatten-then-batch approach (D2): collects all non-empty blocks
// into (blockUUID, chunk, headingPath) tuples, then batch-embeds using
// EmbedBatch() with a batch size of 32 (D1). On batch failure, falls back
// to individual Embed() calls with existing truncation retry logic.
//
// The maxChunkChars parameter controls the maximum character length per
// embedding chunk. Use embed.DefaultMaxChunkChars when no config-derived
// value is available.
//
// Returns the number of embeddings generated. Embedding failures are logged
// but don't block indexing (graceful degradation).
func GenerateEmbeddings(s *store.Store, embedder embed.Embedder, pageName string, blocks []types.BlockEntity, headingPath []string, maxChunkChars int) int {
	// Pass 1: Flatten block tree into chunk tuples.
	var chunks []blockChunk
	flattenBlocks(blocks, headingPath, pageName, &chunks, maxChunkChars)

	if len(chunks) == 0 {
		return 0
	}

	// Pass 2: Batch-embed collected chunks.
	count := 0
	ctx := context.Background()

	for i := 0; i < len(chunks); i += embeddingBatchSize {
		end := i + embeddingBatchSize
		if end > len(chunks) {
			end = len(chunks)
		}
		batch := chunks[i:end]

		// Collect chunk texts for the batch call.
		texts := make([]string, len(batch))
		for j, bc := range batch {
			texts[j] = bc.chunk
		}

		vectors, err := embedder.EmbedBatch(ctx, texts)
		if err != nil {
			// Batch failed — fall back to individual Embed() for each chunk
			// in this batch, preserving existing truncation retry logic (D1).
			logger.Warn("batch embedding failed, falling back to individual embedding",
				"page", pageName, "batchSize", len(batch), "err", err)
			for _, bc := range batch {
				count += embedSingleChunk(ctx, s, embedder, pageName, bc.blockUUID, bc.chunk)
			}
			continue
		}

		// Batch succeeded — persist each embedding.
		for j, vec := range vectors {
			bc := batch[j]
			if err := s.InsertEmbedding(bc.blockUUID, embedder.ModelID(), vec, bc.chunk); err != nil {
				logger.Warn("failed to persist embedding",
					"page", pageName, "block", bc.blockUUID, "err", err)
				continue
			}
			count++
		}
	}

	return count
}

// flattenBlocks recursively collects all non-empty blocks into a flat slice
// of blockChunk tuples, preserving heading path context for each block.
// The maxChunkChars parameter is forwarded to embed.PrepareChunk to control
// chunk truncation.
func flattenBlocks(blocks []types.BlockEntity, headingPath []string, pageName string, out *[]blockChunk, maxChunkChars int) {
	for _, b := range blocks {
		if strings.TrimSpace(b.Content) == "" {
			continue
		}

		currentPath := headingPath
		heading := ExtractHeadingFromContent(b.Content)
		if heading != "" {
			currentPath = append(append([]string{}, headingPath...), heading)
		}

		chunk := embed.PrepareChunk(pageName, currentPath, b.Content, maxChunkChars)
		*out = append(*out, blockChunk{
			blockUUID:   b.UUID,
			chunk:       chunk,
			headingPath: currentPath,
		})

		if len(b.Children) > 0 {
			flattenBlocks(b.Children, currentPath, pageName, out, maxChunkChars)
		}
	}
}

// embedSingleChunk embeds a single chunk with the existing truncation retry
// logic for context-length errors. Returns 1 on success, 0 on failure.
func embedSingleChunk(ctx context.Context, s *store.Store, embedder embed.Embedder, pageName, blockUUID, chunk string) int {
	vec, err := embedder.Embed(ctx, chunk)
	if err != nil {
		// Check for context-length overflow and retry with truncated chunk.
		if strings.Contains(err.Error(), "context length") {
			runes := []rune(chunk)
			truncated := string(runes[:len(runes)/2])
			logger.Debug("retrying embedding with truncated chunk",
				"page", pageName, "block", blockUUID,
				"originalLen", len(runes), "truncatedLen", len(runes)/2)
			vec, err = embedder.Embed(ctx, truncated)
			if err == nil {
				if storeErr := s.InsertEmbedding(blockUUID, embedder.ModelID(), vec, truncated); storeErr != nil {
					logger.Warn("failed to persist embedding after retry",
						"page", pageName, "block", blockUUID, "err", storeErr)
					return 0
				}
				return 1
			}
		}
		logger.Warn("failed to generate embedding",
			"page", pageName, "block", blockUUID, "chunkLen", len([]rune(chunk)), "err", err)
		return 0
	}

	if err := s.InsertEmbedding(blockUUID, embedder.ModelID(), vec, chunk); err != nil {
		logger.Warn("failed to persist embedding",
			"page", pageName, "block", blockUUID, "err", err)
		return 0
	}
	return 1
}
