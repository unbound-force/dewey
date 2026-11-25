---
tag: vault-store
author: jay-flowers
category: gotcha
created_at: 2026-11-07T16:51:33Z
identity: vault-store-20261107T165133-jay-flowers
tier: draft
---

In the dewey vault package, error-path tests inject failures by closing the SQLite store (the *StoreErrorSurfaces family, e.g. TestGetPage_StoreErrorSurfaces, TestGetPageBlocksTree_StoreErrorSurfaces). Store-close fails the FIRST query, so it cannot reach error branches that need a split outcome — an early lookup succeeding-and-missing while a later one fails, such as the prefix loop in findStorePage (exact name, then compiled/<name>, then learning/<name>), and vs.store is a concrete *store.Store with no fault-injection seam. When the unreachable branch mirrors a tested guard line-for-line (same GetPageFoldCase call, same sp != nil || err != nil condition, same return), the proportionate remedy is a test comment documenting the bounded gap — not interface indirection added solely to cover one branch. A single-use seam is an unnecessary abstraction under the Zero-Waste Mandate, and converting the shared store field to an interface ripples across every VaultStore method that uses it, beyond a review-phase minor fix.
