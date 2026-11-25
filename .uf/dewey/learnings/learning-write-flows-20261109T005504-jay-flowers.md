---
tag: learning-write-flows
author: jay-flowers
category: gotcha
created_at: 2026-11-09T00:55:04Z
identity: learning-write-flows-20261109T005504-jay-flowers
tier: draft
---

Dewey's learning temporal-metadata writes are dual-written (SQLite store + .uf/dewey/learnings markdown backup), and the partial-failure contract is load-bearing: #146's linkSuperseded/markSupersededFile return structured warning strings that surface in the store_learning result (supersedes_warning) when the store write succeeds but the backup write fails, while PR #159's lint anchor fix copied the same flow with a bare-error contract that fixAnchorDrift logs and swallows — under-counting fixed.anchor_drift and hiding store/backup divergence from the tool result. When extending learning metadata writes, extract the shared backup-frontmatter helper with one partial-failure contract instead of copying the flow: fail closed before any write (unreadable properties, unsafe identity), warn structurally after a partial write, and keep ValidLearningIdentity enforcement inside the helper next to filepath.Join so the path-traversal guard travels with the path construction rather than relying on caller discipline.
