---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-09T01:06:11Z
identity: test-quality-20261109T010611-jay-flowers
tier: draft
---

Error-path tests for multi-sink writers must assert the post-failure state of every sink, not just the returned error. When a function writes sinks in sequence (dewey's markStaleUnverified: store row, then markdown backup, then an aggregate count in fixAnchorDrift), a failure at sink k leaves sinks 1..k-1 mutated — an observable partial-write contract. A test that checks only err != nil cannot distinguish rollback, partial-write, and over-write implementations. Pattern: after triggering the failure, read back each sink with the same helpers the happy-path test uses and pin the exact divergence, with a comment noting the pin is deliberate so later semantics changes must consciously update the test.
