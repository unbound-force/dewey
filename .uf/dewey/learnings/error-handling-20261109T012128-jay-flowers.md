---
tag: error-handling
author: jay-flowers
category: pattern
created_at: 2026-11-09T01:21:28Z
identity: error-handling-20261109T012128-jay-flowers
tier: draft
---

Dual-write partial failures must surface in the machine-readable result, not stderr alone. When a tool persists state to the authoritative store and then to a recovery artifact (markdown backup), a second-write failure after a successful store write must not be reported as total failure: count the mutation against the store and attach a structured warning to the result. Precedents: dewey PR #146's supersedes_warning (store_learning back-link failure after store success, tools/learning.go:350) and PR #159 feedback item 7 (fixed.anchor_drift undercount in lint's fixAnchorDrift/markStaleUnverified). Without this, result fields understate persisted state (Constitution III Observable Quality), MCP consumers cannot observe the divergence, and re-ingestion after graph.db loss silently reverts the mutation from the un-updated backup.
