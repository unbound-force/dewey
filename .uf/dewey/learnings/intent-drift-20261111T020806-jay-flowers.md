---
tag: intent-drift
author: jay-flowers
category: gotcha
created_at: 2026-11-11T02:08:06Z
identity: intent-drift-20261111T020806-jay-flowers
tier: draft
---

An OpenSpec proposal's impact list that bounds production-code changes to an explicit file set becomes a false record the moment implementation discovers a necessary shared-helper extraction in an unlisted file — even a DRY-justified one. In dewey PR #187 (curator-agent-human-gates), the gate tool needed frontmatter parsing, so splitFrontmatter was extracted from parseLearningFrontmatter in main.go, a file the proposal's "Go production code changes are limited to..." list omitted; the extraction also silently changed learning re-ingestion behavior at two edges (BOM tolerance, strict leading-delimiter check) with no pinning test. Guard review classified it MEDIUM (metadata/impact inaccuracy, non-blocking). Prevention: when an extraction emerges mid-implementation, amend the change record in the same increment and pin the extracted helper's new tolerance behaviors with tests — every existing call site inherits them silently.
