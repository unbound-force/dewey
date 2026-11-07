---
tag: cross-package-contracts
author: jay-flowers
category: gotcha
created_at: 2026-11-05T16:45:15Z
identity: cross-package-contracts-20261105T164515-jay-flowers
tier: draft
---

A seam-test pattern must be re-applied per writer/reader pair, not once per codebase. Dewey PR 145 established the pattern (feed the producer's rendered output to the consumer's parser — TestLint_ParsesCurateRenderedQuotedFrontmatter for curate→lint quoted frontmatter), yet PR 156 introduced a new cross-package format pair (tools temporalFields renders learning frontmatter; main.go learningFrontmatter re-parses it during re-ingestion) with only per-side fixture tests, and the same drift-gap feedback recurred within a week. Heuristic for designs and reviews: when a change adds a writer for a format another package parses (or a parser for a format another package writes), enumerate the writer/reader pairs, confirm each pair — including the new one — has a seam test chaining the real writer's output into the real reader, and name that seam test in the change's design/tasks so it ships with the feature instead of arriving as review feedback.
