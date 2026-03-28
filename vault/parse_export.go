package vault

import (
	"database/sql"
	"fmt"
	"strings"

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
func ExtractHeadingFromContent(content string) string {
	firstLine := content
	if idx := strings.IndexByte(content, '\n'); idx >= 0 {
		firstLine = content[:idx]
	}
	return extractHeading(firstLine)
}
