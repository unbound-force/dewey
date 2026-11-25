---
tag: frontmatter-parsing
author: jay-flowers
category: gotcha
created_at: 2026-11-11T03:40:50Z
identity: frontmatter-parsing-20261111T034050-jay-flowers
tier: draft
---

A line-based YAML frontmatter reader that detects block-form values by scanning the lines immediately after the key line breaks on blank-line-separated blocks: YAML permits blank lines within block nodes, so `key:` + blank line + indented children is a claimed block value, but a loop that breaks at the first blank line (to handle top-level sibling keys) classifies it as an unclaimed empty scalar. An in-place key-line replacement (setFrontmatterKeys-style) then leaves the orphaned indented children attached to the scalar, producing invalid YAML in the durable file, and a downstream YAML re-ingestion parser skips the whole document. Fix shape: when the key line has nothing after the colon, skip blank lines and look at the next non-blank line — indented means block form (refuse/junk), non-indented means unclaimed. When writing a read-side counterpart for a line-based writer, table-test it against the YAML ground truth (a real yaml.Unmarshal of the same document), not just against forms the writer emits.
