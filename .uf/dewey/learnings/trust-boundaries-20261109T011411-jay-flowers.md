---
tag: trust-boundaries
author: jay-flowers
category: pattern
created_at: 2026-11-09T01:14:11Z
identity: trust-boundaries-20261109T011411-jay-flowers
tier: draft
---

Validation enforced on a write API is not a property of stored data when a second ingestion path exists. In dewey, store_learning caps evidence at 50 locators (normalizeEvidence, tools/learning_temporal.go), but reIngestLearnings (main.go) copies frontmatter properties verbatim into the store, so any consumer that parses stored properties — like the lint anchor checks — sees unbounded cardinality. The #156 hardening shows the template: reIngestLearnings already re-validates identity (ValidLearningIdentity) and forces tier draft at the consumption site. Rule of thumb: for every field declared untrusted at a trust boundary, enumerate every path that crosses the boundary (API writes, file re-ingestion) and either validate once at a single chokepoint or re-validate at each consumption site — sharing the API path's constants rather than re-declaring bounds.
