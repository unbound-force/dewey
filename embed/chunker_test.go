package embed

import (
	"strings"
	"testing"
)

// TestPrepareChunk_BasicHeadingPath verifies the heading hierarchy context
// is prepended correctly.
func TestPrepareChunk_BasicHeadingPath(t *testing.T) {
	result := PrepareChunk("setup.md", []string{"Installation", "From Source"}, "Run `make install` to build from source.", DefaultMaxChunkChars)

	want := "setup.md > Installation > From Source\n\nRun `make install` to build from source."
	if result != want {
		t.Errorf("PrepareChunk() =\n%q\nwant:\n%q", result, want)
	}
}

// TestPrepareChunk_NoHeadings verifies behavior when there are no headings.
func TestPrepareChunk_NoHeadings(t *testing.T) {
	result := PrepareChunk("readme.md", nil, "This is the introduction.", DefaultMaxChunkChars)

	want := "readme.md\n\nThis is the introduction."
	if result != want {
		t.Errorf("PrepareChunk(no headings) =\n%q\nwant:\n%q", result, want)
	}
}

// TestPrepareChunk_EmptyHeadingPath verifies behavior with empty heading slice.
func TestPrepareChunk_EmptyHeadingPath(t *testing.T) {
	result := PrepareChunk("readme.md", []string{}, "Content here.", DefaultMaxChunkChars)

	want := "readme.md\n\nContent here."
	if result != want {
		t.Errorf("PrepareChunk(empty headings) =\n%q\nwant:\n%q", result, want)
	}
}

// TestPrepareChunk_DeeplyNested verifies deeply nested heading paths.
func TestPrepareChunk_DeeplyNested(t *testing.T) {
	path := []string{"Chapter 1", "Section A", "Subsection i", "Paragraph 1"}
	result := PrepareChunk("book.md", path, "Deep content.", DefaultMaxChunkChars)

	want := "book.md > Chapter 1 > Section A > Subsection i > Paragraph 1\n\nDeep content."
	if result != want {
		t.Errorf("PrepareChunk(deeply nested) =\n%q\nwant:\n%q", result, want)
	}
}

// TestPrepareChunk_EmptyContent verifies behavior with empty content.
func TestPrepareChunk_EmptyContent(t *testing.T) {
	result := PrepareChunk("page.md", []string{"Heading"}, "", DefaultMaxChunkChars)

	want := "page.md > Heading\n\n"
	if result != want {
		t.Errorf("PrepareChunk(empty content) =\n%q\nwant:\n%q", result, want)
	}
}

// TestPrepareChunk_WhitespaceContent verifies content is trimmed.
func TestPrepareChunk_WhitespaceContent(t *testing.T) {
	result := PrepareChunk("page.md", nil, "  \n  content with whitespace  \n  ", DefaultMaxChunkChars)

	want := "page.md\n\ncontent with whitespace"
	if result != want {
		t.Errorf("PrepareChunk(whitespace) =\n%q\nwant:\n%q", result, want)
	}
}

// TestPrepareChunk_EmptyHeadingInPath verifies empty strings in heading path are skipped.
func TestPrepareChunk_EmptyHeadingInPath(t *testing.T) {
	result := PrepareChunk("page.md", []string{"Heading", "", "SubHeading"}, "Content.", DefaultMaxChunkChars)

	want := "page.md > Heading > SubHeading\n\nContent."
	if result != want {
		t.Errorf("PrepareChunk(empty heading in path) =\n%q\nwant:\n%q", result, want)
	}
}

// TestPrepareChunk_Truncation verifies content is truncated at maxChunkChars.
func TestPrepareChunk_Truncation(t *testing.T) {
	// Create content that exceeds DefaultMaxChunkChars.
	longContent := strings.Repeat("a", DefaultMaxChunkChars+500)
	result := PrepareChunk("page.md", nil, longContent, DefaultMaxChunkChars)

	if len([]rune(result)) > DefaultMaxChunkChars {
		t.Errorf("PrepareChunk(long content) rune length = %d, want <= %d", len([]rune(result)), DefaultMaxChunkChars)
	}
}

// TestPrepareChunk_TruncationPreservesContext verifies the context path
// is preserved even when content is truncated.
func TestPrepareChunk_TruncationPreservesContext(t *testing.T) {
	longContent := strings.Repeat("x", DefaultMaxChunkChars+100)
	result := PrepareChunk("setup.md", []string{"Installation"}, longContent, DefaultMaxChunkChars)

	if !strings.HasPrefix(result, "setup.md > Installation\n\n") {
		t.Error("truncated chunk should preserve context path prefix")
	}
	if len([]rune(result)) > DefaultMaxChunkChars {
		t.Errorf("truncated chunk rune length = %d, want <= %d", len([]rune(result)), DefaultMaxChunkChars)
	}
}

// TestPrepareChunk_SmallLimit verifies truncation with a very small maxChars.
func TestPrepareChunk_SmallLimit(t *testing.T) {
	result := PrepareChunk("page.md", []string{"Heading"}, "Some content here.", 50)

	if len([]rune(result)) > 50 {
		t.Errorf("PrepareChunk(small limit) rune length = %d, want <= 50", len([]rune(result)))
	}
	// With maxChars=50, the context path "page.md > Heading\n\n" (21 chars) fits,
	// and some content should be present.
	if !strings.HasPrefix(result, "page.md > Heading") {
		t.Error("PrepareChunk(small limit) should preserve context path when it fits")
	}
}

// TestPrepareChunk_LargeLimit verifies no truncation with a large maxChars.
func TestPrepareChunk_LargeLimit(t *testing.T) {
	content := "Short content."
	result := PrepareChunk("page.md", nil, content, 12288)

	want := "page.md\n\nShort content."
	if result != want {
		t.Errorf("PrepareChunk(large limit) =\n%q\nwant:\n%q", result, want)
	}
}

// TestPrepareChunk_ExactBoundary verifies behavior when content is exactly at maxChars.
func TestPrepareChunk_ExactBoundary(t *testing.T) {
	// Build a chunk that is exactly 100 runes: "page.md\n\n" (9 chars) + 91 chars of content.
	prefix := "page.md\n\n"
	contentLen := 100 - len([]rune(prefix))
	content := strings.Repeat("b", contentLen)
	result := PrepareChunk("page.md", nil, content, 100)

	if len([]rune(result)) != 100 {
		t.Errorf("PrepareChunk(exact boundary) rune length = %d, want 100", len([]rune(result)))
	}
}

// TestPrepareChunk_NoTruncation verifies content shorter than maxChars is returned fully.
func TestPrepareChunk_NoTruncation(t *testing.T) {
	content := strings.Repeat("c", 100)
	result := PrepareChunk("page.md", nil, content, 12288)

	want := "page.md\n\n" + content
	if result != want {
		t.Errorf("PrepareChunk(no truncation) =\n%q\nwant:\n%q", result, want)
	}
}
