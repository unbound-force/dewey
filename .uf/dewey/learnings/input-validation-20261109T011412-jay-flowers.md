---
tag: input-validation
author: jay-flowers
category: gotcha
created_at: 2026-11-09T01:14:12Z
identity: input-validation-20261109T011412-jay-flowers
tier: draft
---

Cardinality caps enforced on a write API do not bound work at consumption sites when a second write path bypasses them. In dewey, store_learning caps evidence at 50 locators (tools/learning_temporal.go maxEvidenceEntries, enforced by normalizeEvidence), but startup re-ingestion (main.go reIngestLearnings) copies learning frontmatter into properties verbatim — it re-validates identity (ValidLearningIdentity) and tier (forced to draft) but not evidence cardinality. Any consumer that fans out per-element work over re-ingested collections (the lint anchor check spawns 2 git subprocesses per locator) must re-validate cardinality at the point of consumption, mirroring the documented cap and failing closed on out-of-contract data the same way non-hex as_of pins are skipped (PR 159 feedback item 6).
