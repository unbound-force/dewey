# Design: Re-index Knowledge Stores From Their Files

## Context

See proposal. Two independent defects on one path: knowledge-store
pages are only indexed right after a curation run creates files, and
when they are indexed their frontmatter is dropped.

## Goals / Non-Goals

**Goals**

- A server start after `dewey reindex` or graph.db loss restores every
  configured store's pages from disk.
- Knowledge-store pages carry frontmatter properties like every other
  file-backed page family.

**Non-Goals**

- Pruning pages whose files were deleted from a store (pre-existing
  behavior; a full `dewey reindex` clears them).
- Indexing stores from the `dewey index` / `dewey reindex` CLI
  commands. Learnings are restored at server startup too; this keeps
  the two file-backed families symmetric.
- Trust pathspecs for workspace-layout pages (#210).

## Decisions

### D1 Re-index in the deferred index, after learnings

`deferredIndex` already runs `reIngestLearnings` under `indexMu`
before `indexVault`. Knowledge stores go right after it, for the same
reason: re-ingested pages exist before the vault index, backlinks and
search are built. Background curation uses `TryLock` on the same
mutex, so it cannot race the startup pass.

### D2 Extract `reIndexKnowledgeStores`

Mirrors `reIngestLearnings`: `(store, embedder, vaultPath,
maxChunkChars) -> (int, error)`, nil store is a no-op, missing config
is a no-op, invalid config is returned for the caller to log. It
delegates per store to the existing `tools.IndexKnowledgeStore`, so
curation and startup share one implementation.

### D3 Properties encoding matches the other indexers

`json.Marshal(props)` of the `vault.ParseDocument` frontmatter map,
empty string when there is no frontmatter — the same encoding as
`vault_store.go` and `buildSourcePropsJSON`. `json.Marshal` sorts map
keys, so the string is stable and safe to compare.

### D4 Backfill on the unchanged-content path, without block work

Comparing `Properties` alongside the tier on the content-hash-match
path updates the page row only. Rows from earlier binaries (empty
`Properties`) are upgraded on the next startup without deleting
blocks or regenerating embeddings, and the return value (pages whose
blocks were indexed) is unchanged.

## Risks / Trade-offs

- **Startup cost**: one read + parse + `GetPage` per store file on
  every start, with no block or embedding work for unchanged pages.
  The first start after a reindex embeds every store page, as
  curation's auto-index would.
- **Search results shift**: curated pages marked `stale-unverified` or
  `superseded` are now down-weighted, and `validity` filters include
  curated pages. This is the intended behavior of validity-aware
  retrieval.
