---
tag: test-coverage
author: jay-flowers
category: decision
created_at: 2026-11-07T16:51:30Z
identity: test-coverage-20261107T165130-jay-flowers
tier: draft
---

Closed-store error injection (s.Close() before the read) fails every store query uniformly, so in a multi-lookup function like vault findStorePage it can exercise only the first lookup's error guard; a mid-function error branch that needs an earlier lookup to clean-miss and a later one to fail is unreachable without a fault-injection seam over the concrete *store.Store. When the untested branch is line-for-line identical to a guard the closed-store test does cover, the proportionate disposition of LOW coverage feedback is a test comment documenting the limitation — an interface seam added solely for branch coverage trades Zero-Waste Mandate compliance for marginal regression protection on a two-line guard.
