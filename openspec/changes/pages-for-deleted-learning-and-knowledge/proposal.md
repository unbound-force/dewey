# Proposal: Prune Pages For Deleted Learning And
# Knowledge Store Files

## Why

When a file under `.uf/dewey/learnings/` or
`.uf/dewey/knowledge/<store>/` is deleted — by a person or by a
`git pull` that removes it — its page stays in `graph.db`.

Verified against `main` (`2a7f014`):

- `reIngestLearnings` (main.go) scans the learnings directory and
  only adds or refreshes pages for files that exist. It never
  deletes a `learning/*` page.
- `reIndexKnowledgeStores` (main.go, added in #220) delegates to
  `tools.IndexKnowledgeStore`, which likewise only adds or refreshes
  store pages for files present in the store directory.
- The orphan purge in `dewey index` (cli.go:878,
  `purgeOrphanedSources`) only covers sources removed from
  `sources.yaml`. #220 calls the missing store deletion out as
  pre-existing.

So search keeps returning learnings and curated pages that no longer
exist anywhere, and validity ranking cannot help because nothing
marks the pages gone. Only a full `dewey reindex` clears them
(`DELETE` + rebuild), which the epic exists to avoid.

Part of epic #226. Issue #226 Phase 2 sequences this change after
#221 (re-ingest on file change) because both touch the same startup
passes (`reIngestLearnings`, `reIndexKnowledgeStores`) — this
avoids conflicting PRs. Tracked from jflowers/gh-hud#138; when this
ships gh-hud can stop treating deleted learnings as a reindex
signal.

## What Changes

Each startup pass finishes by deleting pages whose file is gone,
logging the count the way the re-ingest pass does.

1. **Learning pruning at the end of `reIngestLearnings`.**
   After the directory scan, list `learning/*` pages from the
   store, compare their identities to the file set just scanned,
   and delete those whose file no longer exists. Block and
   embedding rows go with the page through the existing foreign
   key cascades. A count is logged.
2. **Store pruning at the end of each
   `IndexKnowledgeStore` invocation.** After the store directory
   scan, list pages for that store's `knowledge-{name}` source and
   delete those with no file in the directory. The startup wrapper
   `reIndexKnowledgeStores` aggregates the per-store counts and
   logs them, or logs each store individually — the visible
   behavior is one count per pass, matching the re-ingest log.

Both passes share the epic's standing rules: unchanged files stay
cheap (no block or embedding work when the content hash has not
changed), no `graph.db` schema change, and the learning file format
and frontmatter do not change.

## Capabilities

### New Capabilities
- None. This extends existing file-backed recovery to cover
  deletion.

### Modified Capabilities
- `learning-reingest`: the startup re-ingestion pass prunes
  `learning/*` pages whose files were deleted.
- `curated-knowledge-stores`: the startup re-index pass prunes
  `knowledge/<store>/*` pages whose store files were deleted.

### Removed Capabilities
- None.

## Impact

- **Files modified**: `main.go` (end of `reIngestLearnings` and
  `reIndexKnowledgeStores` / `tools.IndexKnowledgeStore`),
  `main_test.go`, `tools/curate_test.go`.
- **Runtime behavior**: `dewey serve` startup does one extra
  `ListPagesBySource` + set-difference per pass and deletes orphan
  rows. For vaults that have not deleted files the extra query
  returns no orphans and the startup log stays silent — behavior is
  byte-identical to before.
- **No schema change**: deletion uses existing `DeletePage` /
  `DeleteBlocksByPage` and foreign keys. No migration and no
  `schema_version` bump. New keys in `metadata` are allowed but not
  needed here.
- **One PR per issue**: this change does only #222. The epic's
  other pieces (#221, #223, #224, #225) are separate runs and
  changes.
- **Backward compatibility**: fully compatible. Vaults without
  deletions behave exactly as before, and every existing test keeps
  passing.

## Constitution Alignment

Assessed against the Unbound Force org constitution
(.specify/memory/constitution.md) and the Dewey constitution
v1.4.0.

### I. Autonomous Collaboration

**Assessment**: PASS

File-backed pages remain the self-sufficient durable record. A
deletion committed to git is reflected in every checkout's index
after a pull and a restart, with no coordination beyond the server.

### II. Composability First

**Assessment**: PASS

No new dependencies, configuration, or CLI surface. Vaults without
learnings or knowledge stores, or without deletions, hit the same
code path they do today — a fast directory scan, a page listing, an
empty difference, and no log output.

### III. Observable Quality

**Assessment**: PASS

Each pruning pass logs its deleted count, mirroring the existing
re-ingest log line. A human or agent can audit `.uf/dewey/` and
`graph.db` to see which pages exist and that deleted files no longer
contribute to search results.

### IV. Testability

**Assessment**: PASS

Both passes are functions that take store, embedder and vault path
and are exercised with an in-memory store and `t.TempDir()` files.
Orphan setup uses the same helpers as existing re-ingest tests;
embedder fakes prove unchanged files trigger no embedding work.

<!-- scaffolded by uf vdev -->
