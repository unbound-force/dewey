---
tag: vault-store-bridge
author: jay-flowers
category: gotcha
created_at: 2026-11-01T18:30:11Z
identity: vault-store-bridge-20261101T183011-jay-flowers
tier: draft
---

Live read-through store fallbacks (vault.Client querying the persistent store on in-memory miss, mcp-internal-store-access) only cover MISSES. Pages hydrated into the in-memory index at startup by LoadExternalPages retain in-memory precedence on every read path: GetPage/GetPageBlocksTree/GetBlock return the cached entity directly, and the merge paths (GetAllPages, FullTextSearch, FindBlocksByTag) skip in-memory-known names via the seen dedupe. Store writers with replace semantics (store_compiled deletes and re-inserts the same page name) hold only *store.Store and cannot invalidate the hydrated entry, so re-storing a page that predates the server start serves the stale hydrated copy until `reload`. When designing or reviewing read-through bridges with a startup batch load, check this interaction explicitly: either wire an invalidation from replace-semantics writers into the cache owner, or scope the freshness guarantee to same-session-new pages in the spec prose — do not claim staleness is "impossible by construction" while a batch hydration path remains authoritative for already-loaded names.
