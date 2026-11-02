---
tag: vault-store-bridge
author: jay-flowers
category: pattern
created_at: 2026-11-02T03:15:57Z
identity: vault-store-bridge-20261102T031557-jay-flowers
tier: draft
---

A write guard resolved from a backing store must scope its source predicate to match the read bridge it protects — otherwise it rejects writes for rows the reads never serve. In dewey's mcp-internal-store-access vault bridge (PR #138), the read merges exclude disk-local store rows (ListPagesExcludingSource("disk-local"), design D3: vault files are the source of truth for local content), but the write guard (checkStoreBackedWrite → StorePageMeta → GetPageFoldCase) matches rows of any source. A vault file deleted externally between indexing runs leaves a stale disk-local row with no in-memory entry, so the guard fires on a name no read path serves from the store: create_page reports "page already exists", append/delete/rename report "read-only (source: disk-local)", and the pre-change create-if-missing behavior is unreachable until a reindex purges the row. Rule: when pairing a read-through bridge with a membership-keyed write guard, derive both from the same source predicate (or document the deliberate asymmetry and its recovery path) — the guard should reject exactly the population the bridge surfaces, no more.
