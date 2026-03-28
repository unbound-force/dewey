package vault

import (
	"strings"
	"testing"
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
