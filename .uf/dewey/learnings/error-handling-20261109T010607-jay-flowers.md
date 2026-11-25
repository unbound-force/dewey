---
tag: error-handling
author: jay-flowers
category: gotcha
created_at: 2026-11-09T01:06:07Z
identity: error-handling-20261109T010607-jay-flowers
tier: draft
---

Dual-write fix paths need failure-path tests that pin the post-failure state of every sink, not just the error return and a success count. In dewey's lint anchor fix (PR 159), markStaleUnverified commits the SQLite store write (UpdatePage) before the markdown backup (markStaleFile); when the backup fails (file without frontmatter — setFrontmatterKeys errors), the store keeps validity:stale-unverified while fixed.anchor_drift excludes the learning, and TestMarkStaleUnverified_Errors passed regardless because it asserted only err != nil and count == 0. The suite pinned the pre-write validation failures (corrupt properties, unsafe identity) but not the mid-write failure, leaving the partial-failure contract invisible. Rule of thumb: for any operation that writes two sinks, assert each sink's state after EVERY failure injection point, and pin the contract the implementation deliberately adopts (rollback, write order, or surfaced warning like #146's supersedes_warning), so count-vs-state divergence cannot hide behind a passing test.
