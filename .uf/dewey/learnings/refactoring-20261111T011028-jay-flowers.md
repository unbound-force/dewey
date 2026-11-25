---
tag: refactoring
author: jay-flowers
category: gotcha
created_at: 2026-11-11T01:10:28Z
identity: refactoring-20261111T011028-jay-flowers
tier: draft
---

Extracting a shared helper can tighten the original caller's input tolerance. In curator-agent-human-gates, splitFrontmatter's leading-`---` requirement changed parseLearningFrontmatter so a learning backup with leading whitespace before its frontmatter is logged and skipped during re-ingestion instead of restored. When DRYing a parser, diff the old and new acceptance behavior at every original call site, keep a test for the looser legacy input, and list the refactored file in the change's Impact section.
