package sanitize

import (
	"strings"
	"testing"
)

func TestValidateStructure_InvisibleChars(t *testing.T) {
	t.Run("above threshold produces finding", func(t *testing.T) {
		// 20 zero-width spaces interspersed between words.
		content := strings.Repeat("word\u200B", 20)
		findings := ValidateStructure(content, 5)

		if len(findings) == 0 {
			t.Fatal("expected at least one finding for 20 invisible chars with threshold 5, got none")
		}

		found := false
		for _, f := range findings {
			if f.Pattern == "invisible-unicode" {
				found = true
				if f.Severity != "medium" {
					t.Errorf("expected severity %q, got %q", "medium", f.Severity)
				}
				if f.Category != "structure" {
					t.Errorf("expected category %q, got %q", "structure", f.Category)
				}
				if !strings.Contains(f.Message, "20") {
					t.Errorf("expected message to contain count 20, got %q", f.Message)
				}
			}
		}
		if !found {
			t.Error("expected finding with pattern 'invisible-unicode', not found")
		}
	})

	t.Run("below threshold produces no finding", func(t *testing.T) {
		// 3 zero-width spaces, threshold is 5.
		content := "hello\u200Bworld\u200Bfoo\u200Bbar"
		findings := ValidateStructure(content, 5)

		for _, f := range findings {
			if f.Pattern == "invisible-unicode" {
				t.Errorf("expected no invisible-unicode finding for 3 chars with threshold 5, got %+v", f)
			}
		}
	})

	t.Run("exactly at threshold produces no finding", func(t *testing.T) {
		// 5 zero-width spaces, threshold is 5 — should NOT trigger (count must exceed threshold).
		content := strings.Repeat("a\u200B", 5)
		findings := ValidateStructure(content, 5)

		for _, f := range findings {
			if f.Pattern == "invisible-unicode" {
				t.Errorf("expected no finding when count equals threshold, got %+v", f)
			}
		}
	})
}

func TestValidateStructure_DataURI(t *testing.T) {
	t.Run("image with data URI produces finding", func(t *testing.T) {
		content := "Some text\n![img](data:text/html;base64,abc)\nMore text"
		findings := ValidateStructure(content, 5)

		if len(findings) == 0 {
			t.Fatal("expected at least one finding for data URI image, got none")
		}

		found := false
		for _, f := range findings {
			if f.Pattern == "data-uri" {
				found = true
				if f.Severity != "high" {
					t.Errorf("expected severity %q, got %q", "high", f.Severity)
				}
				if f.Category != "structure" {
					t.Errorf("expected category %q, got %q", "structure", f.Category)
				}
				if f.Line != 2 {
					t.Errorf("expected line 2, got %d", f.Line)
				}
			}
		}
		if !found {
			t.Error("expected finding with pattern 'data-uri', not found")
		}
	})

	t.Run("link with data URI produces finding", func(t *testing.T) {
		content := "[click here](data:text/html;base64,PHNjcmlwdD4=)"
		findings := ValidateStructure(content, 5)

		found := false
		for _, f := range findings {
			if f.Pattern == "data-uri" {
				found = true
				if f.Severity != "high" {
					t.Errorf("expected severity %q, got %q", "high", f.Severity)
				}
			}
		}
		if !found {
			t.Error("expected finding with pattern 'data-uri' for link syntax, not found")
		}
	})
}

func TestValidateStructure_HeadingDepth(t *testing.T) {
	t.Run("seven hashes produces finding", func(t *testing.T) {
		content := "# Valid\n## Also valid\n####### Too deep"
		findings := ValidateStructure(content, 5)

		found := false
		for _, f := range findings {
			if f.Pattern == "heading-depth" {
				found = true
				if f.Severity != "low" {
					t.Errorf("expected severity %q, got %q", "low", f.Severity)
				}
				if f.Category != "structure" {
					t.Errorf("expected category %q, got %q", "structure", f.Category)
				}
				if f.Line != 3 {
					t.Errorf("expected line 3, got %d", f.Line)
				}
			}
		}
		if !found {
			t.Error("expected finding with pattern 'heading-depth', not found")
		}
	})

	t.Run("six hashes produces no finding", func(t *testing.T) {
		content := "###### Six levels is valid"
		findings := ValidateStructure(content, 5)

		for _, f := range findings {
			if f.Pattern == "heading-depth" {
				t.Errorf("expected no heading-depth finding for 6 hashes, got %+v", f)
			}
		}
	})

	t.Run("hashes in middle of line are not headings", func(t *testing.T) {
		// Hashes not at the start of a line should not trigger.
		content := "This has ####### in the middle"
		findings := ValidateStructure(content, 5)

		for _, f := range findings {
			if f.Pattern == "heading-depth" {
				t.Errorf("expected no heading-depth finding for mid-line hashes, got %+v", f)
			}
		}
	})
}

func TestValidateStructure_SuspiciousHTML(t *testing.T) {
	t.Run("script tag produces finding", func(t *testing.T) {
		content := "<script>alert(1)</script>"
		findings := ValidateStructure(content, 5)

		found := false
		for _, f := range findings {
			if f.Pattern == "suspicious-html" {
				found = true
				if f.Severity != "high" {
					t.Errorf("expected severity %q, got %q", "high", f.Severity)
				}
				if f.Category != "structure" {
					t.Errorf("expected category %q, got %q", "structure", f.Category)
				}
			}
		}
		if !found {
			t.Error("expected finding with pattern 'suspicious-html' for <script> tag, not found")
		}
	})

	t.Run("iframe tag produces finding", func(t *testing.T) {
		content := "<iframe src=\"https://evil.com\"></iframe>"
		findings := ValidateStructure(content, 5)

		found := false
		for _, f := range findings {
			if f.Pattern == "suspicious-html" {
				found = true
			}
		}
		if !found {
			t.Error("expected finding with pattern 'suspicious-html' for <iframe> tag, not found")
		}
	})

	t.Run("event handler produces finding", func(t *testing.T) {
		content := "<img src=x onerror=alert(1)>"
		findings := ValidateStructure(content, 5)

		found := false
		for _, f := range findings {
			if f.Pattern == "html-event-handler" {
				found = true
				if f.Severity != "high" {
					t.Errorf("expected severity %q, got %q", "high", f.Severity)
				}
			}
		}
		if !found {
			t.Error("expected finding with pattern 'html-event-handler', not found")
		}
	})

	t.Run("safe HTML produces no finding", func(t *testing.T) {
		content := "<p>text</p>\n<div>block</div>\n<strong>bold</strong>"
		findings := ValidateStructure(content, 5)

		for _, f := range findings {
			if f.Pattern == "suspicious-html" || f.Pattern == "html-event-handler" {
				t.Errorf("expected no HTML finding for safe tags, got %+v", f)
			}
		}
	})
}

func TestValidateStructure_CleanContent(t *testing.T) {
	content := `# Welcome

This is a normal Markdown document with **bold** and *italic* text.

## Section Two

- List item one
- List item two
- List item three

### Code Example

` + "```go\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n```" + `

Here is a [normal link](https://example.com) and an image:

![photo](https://example.com/photo.jpg)

###### Deepest Valid Heading

> A blockquote with some wisdom.
`

	findings := ValidateStructure(content, 5)

	if len(findings) != 0 {
		t.Errorf("expected no findings for clean Markdown content, got %d: %+v", len(findings), findings)
	}
}

func TestValidateStructure_ReturnsEmptySlice(t *testing.T) {
	// Verify the function returns an empty slice (not nil) for clean content,
	// ensuring callers can safely iterate without nil checks.
	findings := ValidateStructure("clean content", 5)

	if findings == nil {
		t.Error("expected non-nil empty slice, got nil")
	}
}
