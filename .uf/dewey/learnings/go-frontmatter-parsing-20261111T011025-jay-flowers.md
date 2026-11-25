---
tag: go-frontmatter-parsing
author: jay-flowers
category: gotcha
created_at: 2026-11-11T01:10:25Z
identity: go-frontmatter-parsing-20261111T011025-jay-flowers
tier: draft
---

In package main, frontmatter is parsed with gopkg.in/yaml.v3 on the block returned by splitFrontmatter (parseLearningFrontmatter). A hand-rolled single-key scanner added next to it (gate.go frontmatterValue) silently accepts frontmatter that is invalid YAML elsewhere, because it only pattern-matches one line — and it matches a nested key at any indentation. When a spec says "unparseable definition MUST be refused", reuse yaml.Unmarshal into a small struct rather than scanning lines; otherwise the fail-closed guarantee is weakened without any test noticing.
