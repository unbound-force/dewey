---
tag: untrusted-input
author: jay-flowers
category: gotcha
created_at: 2026-11-09T00:04:09Z
identity: untrusted-input-20261109T000409-jay-flowers
tier: draft
---

Validation enforced at one API entry point is not inherited by sibling ingestion paths. In dewey, store_learning caps evidence at 50 locators × 512 runes via normalizeEvidence, but reIngestLearnings copies frontmatter evidence verbatim into the store, so stored properties carry unbounded, untrusted cardinality. When a stored field feeds a resource-consuming operation (lint anchor checks spawn up to 3 git subprocesses per locator), re-validate bounds at the consumption site or at every write path — upstream validation at a single API entry point is not a store-wide invariant.
