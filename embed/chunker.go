package embed

import "strings"

// maxChunkChars is the approximate character limit for a chunk.
// Granite-embedding:30m has a 512-token context window; at ~4 chars/token
// this gives ~2000 chars. We use 2000 as a conservative limit.
const maxChunkChars = 2000

// PrepareChunk creates an embedding-ready chunk from block content by
// prepending the heading hierarchy context path. This provides semantic
// context for the embedding model — a block about "From Source" under
// "Installation" in "setup.md" becomes:
//
//	"setup.md > Installation > From Source\n\ncontent..."
//
// Returns the formatted chunk string, truncated to maxChunkChars (~512
// tokens) to fit within the embedding model's context window. Empty
// headings in the path are skipped. Uses rune-based truncation to avoid
// splitting multi-byte UTF-8 characters.
//
// Design decision: Block-level chunking was chosen over page-level or
// fixed-size windows because blocks are the natural semantic units in
// a Markdown vault (Decision 4 in research.md). Each block has a stable
// UUID enabling incremental re-embedding when content changes.
func PrepareChunk(pageName string, headingPath []string, content string) string {
	var sb strings.Builder

	// Build context path: "pageName > heading1 > heading2"
	sb.WriteString(pageName)
	for _, h := range headingPath {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		sb.WriteString(" > ")
		sb.WriteString(h)
	}

	// Separate context path from content with double newline.
	sb.WriteString("\n\n")
	sb.WriteString(strings.TrimSpace(content))

	result := sb.String()

	// Truncate to fit within the embedding model's context window.
	// Use rune-based truncation to avoid splitting multi-byte UTF-8 characters,
	// which would produce invalid UTF-8 and corrupt embedding input.
	runes := []rune(result)
	if len(runes) > maxChunkChars {
		result = string(runes[:maxChunkChars])
	}

	return result
}
