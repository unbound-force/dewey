package embed

import (
	"strings"
	"testing"
)

// TestPrepareChunk_BasicHeadingPath verifies the heading hierarchy context
// is prepended correctly.
func TestPrepareChunk_BasicHeadingPath(t *testing.T) {
	result := PrepareChunk("setup.md", []string{"Installation", "From Source"}, "Run `make install` to build from source.")

	want := "setup.md > Installation > From Source\n\nRun `make install` to build from source."
	if result != want {
		t.Errorf("PrepareChunk() =\n%q\nwant:\n%q", result, want)
	}
}

// TestPrepareChunk_NoHeadings verifies behavior when there are no headings.
func TestPrepareChunk_NoHeadings(t *testing.T) {
	result := PrepareChunk("readme.md", nil, "This is the introduction.")

	want := "readme.md\n\nThis is the introduction."
	if result != want {
		t.Errorf("PrepareChunk(no headings) =\n%q\nwant:\n%q", result, want)
	}
}

// TestPrepareChunk_EmptyHeadingPath verifies behavior with empty heading slice.
func TestPrepareChunk_EmptyHeadingPath(t *testing.T) {
	result := PrepareChunk("readme.md", []string{}, "Content here.")

	want := "readme.md\n\nContent here."
	if result != want {
		t.Errorf("PrepareChunk(empty headings) =\n%q\nwant:\n%q", result, want)
	}
}

// TestPrepareChunk_DeeplyNested verifies deeply nested heading paths.
func TestPrepareChunk_DeeplyNested(t *testing.T) {
	path := []string{"Chapter 1", "Section A", "Subsection i", "Paragraph 1"}
	result := PrepareChunk("book.md", path, "Deep content.")

	want := "book.md > Chapter 1 > Section A > Subsection i > Paragraph 1\n\nDeep content."
	if result != want {
		t.Errorf("PrepareChunk(deeply nested) =\n%q\nwant:\n%q", result, want)
	}
}

// TestPrepareChunk_EmptyContent verifies behavior with empty content.
func TestPrepareChunk_EmptyContent(t *testing.T) {
	result := PrepareChunk("page.md", []string{"Heading"}, "")

	want := "page.md > Heading\n\n"
	if result != want {
		t.Errorf("PrepareChunk(empty content) =\n%q\nwant:\n%q", result, want)
	}
}

// TestPrepareChunk_WhitespaceContent verifies content is trimmed.
func TestPrepareChunk_WhitespaceContent(t *testing.T) {
	result := PrepareChunk("page.md", nil, "  \n  content with whitespace  \n  ")

	want := "page.md\n\ncontent with whitespace"
	if result != want {
		t.Errorf("PrepareChunk(whitespace) =\n%q\nwant:\n%q", result, want)
	}
}

// TestPrepareChunk_EmptyHeadingInPath verifies empty strings in heading path are skipped.
func TestPrepareChunk_EmptyHeadingInPath(t *testing.T) {
	result := PrepareChunk("page.md", []string{"Heading", "", "SubHeading"}, "Content.")

	want := "page.md > Heading > SubHeading\n\nContent."
	if result != want {
		t.Errorf("PrepareChunk(empty heading in path) =\n%q\nwant:\n%q", result, want)
	}
}

// TestPrepareChunk_Truncation verifies content is truncated at maxChunkChars.
func TestPrepareChunk_Truncation(t *testing.T) {
	// Create content that exceeds maxChunkChars.
	longContent := strings.Repeat("a", maxChunkChars+500)
	result := PrepareChunk("page.md", nil, longContent)

	if len(result) > maxChunkChars {
		t.Errorf("PrepareChunk(long content) length = %d, want <= %d", len(result), maxChunkChars)
	}
}

// TestPrepareChunk_TruncationPreservesContext verifies the context path
// is preserved even when content is truncated.
func TestPrepareChunk_TruncationPreservesContext(t *testing.T) {
	longContent := strings.Repeat("x", maxChunkChars+100)
	result := PrepareChunk("setup.md", []string{"Installation"}, longContent)

	if !strings.HasPrefix(result, "setup.md > Installation\n\n") {
		t.Error("truncated chunk should preserve context path prefix")
	}
	if len(result) > maxChunkChars {
		t.Errorf("truncated chunk length = %d, want <= %d", len(result), maxChunkChars)
	}
}
