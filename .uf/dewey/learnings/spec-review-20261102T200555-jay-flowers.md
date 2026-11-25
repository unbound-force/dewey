---
tag: spec-review
author: jay-flowers
category: gotcha
created_at: 2026-11-02T20:05:55Z
identity: spec-review-20261102T200555-jay-flowers
tier: draft
---

When reviewing speckit quickstart validation scenarios, verify each expected outcome against the actual CLI command path invoked. In dewey, `dewey status` (cli.go newStatusCmd → queryStoreStatus) opens the SQLite store directly and never constructs the embedding provider, so it cannot produce provider-degradation or availability logs — those come only from createIndexEmbedder (index/reindex/compile/curate paths) and the serve startup path. Degradation smoke tests should invoke `dewey doctor` (which probes the embedding layer) or `dewey serve`, not `status`.
