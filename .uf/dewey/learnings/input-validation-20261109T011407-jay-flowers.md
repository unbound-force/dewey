---
tag: input-validation
author: jay-flowers
category: gotcha
created_at: 2026-11-09T01:14:07Z
identity: input-validation-20261109T011407-jay-flowers
tier: draft
---

Writer-side validation does not protect consumers after a verbatim file round-trip. Dewey's store_learning enforces evidence cardinality (<=50 locators x 512 runes, normalizeEvidence in tools/learning_temporal.go) only on the API input path, while reIngestLearnings (main.go) marshals git-synced learning-file frontmatter properties verbatim into the store — so any consumer that iterates evidence must re-validate cardinality at its own trust boundary. Dewey's lint anchor checks spawned unbounded git subprocesses per locator from forged re-ingested learnings before this gap was flagged (PR 159, item 6). Rule: every trust-boundary crossing re-validates shape and cardinality; a writer's guarantee does not survive re-ingestion, and the codebase itself declares learning files untrusted (main.go reIngestLearnings comment: 'learning files may be git-synced').
