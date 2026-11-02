---
tag: vault-store-bridge
author: jay-flowers
category: pattern
created_at: 2026-11-02T03:15:49Z
identity: vault-store-bridge-20261102T031549-jay-flowers
tier: draft
---

When a change introduces a clean bridge layer over a wrapped dependency (dewey's VaultStore read helpers: GetStorePage, ListStorePages, SearchStoreBlocks — each centralizing nil-store checks and error wrapping) while simultaneously generalizing a legacy helper that accessed the wrapped field directly (storeLearningTagResults → storePagesTagResults), the generalized helper tends to preserve the old direct-field access pattern even though the rest of the change routes through the bridge. Audit for this drift by grepping changed lines for the inner-field dereference (`.store.` reaching past the wrapper). The drift usually has a functional driver — here the tag matcher needed the row's SourceID, which the bridge's converted entity type drops — so the fix is either extending the bridge's return type (entity + sourceID pairs) or documenting the bypass reason at the call site. Rule: a bridge layer is only the pattern if every consumer uses it; one justified bypass invites the next unjustified one.
