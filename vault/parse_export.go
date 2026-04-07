package vault

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/unbound-force/dewey/embed"
	"github.com/unbound-force/dewey/parser"
	"github.com/unbound-force/dewey/store"
	"github.com/unbound-force/dewey/types"
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

// GenerateEmbeddings creates vector embeddings for blocks and persists them
// to the store. This is the shared implementation used by both VaultStore
// (serve-time persistence) and the CLI indexing pipeline, eliminating
// duplication (same pattern as PersistBlocks/PersistLinks).
//
// Returns the number of embeddings generated. Embedding failures are logged
// but don't block indexing (graceful degradation).
func GenerateEmbeddings(s *store.Store, embedder embed.Embedder, pageName string, blocks []types.BlockEntity, headingPath []string) int {
	count := 0
	ctx := context.Background()

	for _, b := range blocks {
		if strings.TrimSpace(b.Content) == "" {
			continue
		}

		currentPath := headingPath
		heading := ExtractHeadingFromContent(b.Content)
		if heading != "" {
			currentPath = append(append([]string{}, headingPath...), heading)
		}

		chunk := embed.PrepareChunk(pageName, currentPath, b.Content)

		vec, err := embedder.Embed(ctx, chunk)
		if err != nil {
			// Check for context-length overflow and retry with truncated chunk.
			if strings.Contains(err.Error(), "context length") {
				runes := []rune(chunk)
				truncated := string(runes[:len(runes)/2])
				logger.Debug("retrying embedding with truncated chunk",
					"page", pageName, "block", b.UUID,
					"originalLen", len(runes), "truncatedLen", len(runes)/2)
				vec, err = embedder.Embed(ctx, truncated)
				if err == nil {
					// Retry succeeded — store the embedding with the truncated chunk.
					if storeErr := s.InsertEmbedding(b.UUID, embedder.ModelID(), vec, truncated); storeErr != nil {
						logger.Warn("failed to persist embedding after retry",
							"page", pageName, "block", b.UUID, "err", storeErr)
					} else {
						count++
					}
					if len(b.Children) > 0 {
						count += GenerateEmbeddings(s, embedder, pageName, b.Children, currentPath)
					}
					continue
				}
			}
			logger.Warn("failed to generate embedding",
				"page", pageName, "block", b.UUID, "chunkLen", len([]rune(chunk)), "err", err)
			continue
		}

		if err := s.InsertEmbedding(b.UUID, embedder.ModelID(), vec, chunk); err != nil {
			logger.Warn("failed to persist embedding",
				"page", pageName, "block", b.UUID, "err", err)
			continue
		}
		count++

		if len(b.Children) > 0 {
			count += GenerateEmbeddings(s, embedder, pageName, b.Children, currentPath)
		}
	}
	return count
}
