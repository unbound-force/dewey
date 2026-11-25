---
tag: frontmatter-parsing
author: jay-flowers
category: gotcha
created_at: 2026-11-11T00:05:23Z
identity: frontmatter-parsing-20261111T000523-jay-flowers
tier: draft
---

A line-based frontmatter key reader that treats a key line with no scalar as "unclaimed" cannot distinguish `key:` with an empty value from `key:` followed by an indented block. In dewey's guarded supersession backup write (serialize-supersession-writes), that classification makes the guard proceed, and the in-place `setFrontmatterKeys`/`replaceOrAppendKey` rewrite replaces only the key line, leaving the block's indented children attached to a scalar. The result is invalid YAML; `parseLearningFrontmatter`/`yaml.Unmarshal` in `reIngestLearnings` then skips the file, so the learning silently disappears after `graph.db` loss — the exact durability failure the guard exists to prevent. Rule: a hand-written guarded frontmatter reader must also fail closed when the key line is followed by indented content (or delegate to a real YAML decode), and the seam test must feed the refusal path into the real re-ingestion parser.
