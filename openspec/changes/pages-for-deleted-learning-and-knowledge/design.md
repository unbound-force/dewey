# Design: Prune Pages For Deleted Learning And
# Knowledge Store Files

## Context

See proposal. `reIngestLearnings` (main.go, package main) and
`reIndexKnowledgeStores` / `tools.IndexKnowledgeStore` only add or
refresh pages for files that exist. A deletion — manual or by pull
— leaves an orphan `learning/*` or `knowledge/<store>/*` page in
`graph.db` that search keeps returning. The CLI orphan purge
(`purgeOrphanedSources`, cli.go:878) covers only dropped sources, not
deleted files, and #220 flagged the store gap as pre-existing.

This change runs in epic #226 Phase 2, after #221, on the same
startup passes, respecting the epic's hard rules: unchanged files
stay cheap (no re-embed), no `graph.db` schema change, learning
file format unchanged, and `dewey serve` startup for untouched
vaults stays the same with existing tests passing.

## Goals / Non-Goals

### Goals

- A deleted learning file or store file disappears from the index
  after a pull and a server restart, without `dewey reindex`.
- Startup cost for vaults without deletions stays near today's —
  one extra listing and an empty set difference.
- The pruning decision stays file-exact (no guesses, no validity
  tricks) and mirrors the count log the re-ingest pass already
  emits.

### Non-Goals

- Pruning by content hash, validity, or time. Deletion is file
  existence only; stale content is handled by #221's re-ingest.
- Touching `dewey index` / `dewey reindex` CLI paths beyond
  keeping them in sync when trivial. Startup is the authority.
- Watching the learnings or store directory at runtime. The change
  is restart-based, as the issue asks.
- Changing the learning file format, frontmatter, or the store
  schema. New `metadata` keys are allowed by the epic but not
  needed here.
- Pruning compiled articles or other page families.
- Pruning pages of stores whose config entry was removed from
  `knowledge-stores.yaml`. De-configured-source cleanup belongs
  to the `purgeOrphanedSources` family (cli.go) and is a
  separate change; this change prunes only within stores still
  configured (issue #222 scope, one PR per issue).

## Decisions

### D1 Prune at the tail of each startup pass, not as a new pass

The issue asks "each startup pass should finish by deleting"
its orphans. The implementation follows that phrasing:

- `reIngestLearnings` finishes its directory scan, then deletes
  `learning/*` orphans.
- `IndexKnowledgeStore` (one per configured store) finishes its
  directory scan, then deletes that store's orphans. The wrapper
  `reIndexKnowledgeStores` aggregates the per-store counts.

This keeps the two families symmetric and the ordering obvious:
re-ingest / re-index first, prune second. No new top-level step
is needed in `deferredIndex`.

Alternatives rejected: a separate `pruneOrphans` function run after
both passes would cross family concerns and need to know both
namespaces; pruning before re-ingest would delete pages that the
scan is about to re-create from files still on disk.

### D2 File set from the scan just performed, store set from a
### listing

Each function already has the exact file set:

- `reIngestLearnings` loops `ReadDir(learningsDir)` and skips
  non-`.md` entries. That set becomes the "files on disk" set
  (identity = file name without `.md`, filtered by
  `ValidLearningIdentity` as the scan does).
- `IndexKnowledgeStore` loops `ReadDir(storePath)` with its own
  skips (`_index.md`, dot-files, non-`.md`). That set becomes its
  "files on disk" set.

The "pages in store" sets come from existing queries:

- Learnings: `ListPagesBySource("learning")` — the source used by
  both `store_learning` and the re-ingest insert.
- Stores: `ListPagesBySource(knowledgeSourceID(name))` — the
  per-store source id that #220 introduced.

Orphans are the page identities with no file. Deletion reuses the
existing `DeletePage` (which cascades to `blocks` and `embeddings`
through `ON DELETE CASCADE`) — no new SQL, no migration.

A single bulk `DELETE ... WHERE name NOT IN (...)` would be faster
but would add SQL and diverge from the per-page cascade the tests
already cover. The per-page loop is bounded by construction: the
orphan set cannot exceed the family's indexed page count (N
learnings, M store files), the worst case — an entire family's
directory removed — is a one-time full-family deletion of that
bounded size, and every subsequent start sees an empty
difference at O(N+M) map probes with no deletion work. If a
future vault profile ever makes the one-time worst case
measurably slow, the bulk `DELETE` is the escape hatch; it is
deliberately not built now (YAGNI).

**Review fix (review-council finding 3, HIGH, iteration 1).** The
learning file-set key is the frontmatter identity, which cannot be
known before the file is read — unlike the knowledge-store path,
whose key is the directory entry's base name recorded before
`os.ReadFile`. A learning file that exists but transiently fails to
read (I/O error, per-file permission change on a listable
directory) or fails frontmatter parsing (partial write during a
pull, merge-conflict markers breaking YAML) is therefore absent
from the file set through no operator intent, and treating its page
as an orphan would mass-delete pages on a transient fault — the
exact outcome the fail-closed invariant forbids. The pass
consequently fails closed at the pass level: any per-file read or
parse failure marks the file set incomplete and the pass skips
pruning entirely (a warning is logged with the failure count),
retried on the next startup once the files read cleanly — mirroring
the directory-level scan-error treatment. Sanctioned skips
(directories, non-`.md` entries, missing or unsafe identities)
remain deliberate exclusions and do not suppress pruning.

### D3 No extra embeddings, ever

Pruning deletes rows — it never re-embeds. The "unchanged files
stay cheap" invariant from #221 and #220 is therefore untouched:
both passes keep their content-hash guards, and the pruning path
itself performs no embedding work.

The extra cost for a vault without deletions is one page listing
and one map lookup per existing page. For a vault with N learnings
and M store files this is O(N+M) map probes, with no file reads
beyond the scan already done.

### D4 Logging matches the re-ingest line

`reIngestLearnings` returns a count and `deferredIndex` logs
"learnings re-ingested from files" when count > 0
(main.go:721). `reIndexKnowledgeStores` does the same for its
indexed count. Pruning follows that shape, with the return
contract pinned: `IndexKnowledgeStore` currently returns a
single `int` (indexed count); this change extends it to
`(indexed int, pruned int)` — no error, matching its
warn-and-continue shape — and both callers adapt:
`reIndexKnowledgeStores` (main.go) aggregates the pruned counts
and logs once when the aggregate is > 0, and
`autoIndexKnowledgeStore` (tools/curate.go, used by background
curation) logs its own pruned count the same way:

- `reIngestLearnings` gains an orphan count and logs it on the same
  call site, or the function itself logs — at most one line per
  startup, count > 0 only.
- The knowledge path logs once per store or once aggregated, count
  > 0 only, with the same verbosity.

This satisfies Observable Quality (constitution III) without new
machine-parseable output: an operator can read `dewar.log` or
stderr and see that N learnings and M store pages were pruned.

### D5 No schema change, no new metadata

The epic forbids `graph.db` schema changes without a migration and
a `schema_version` bump. Deletion uses existing tables and keys;
no new columns, no new tables, and no new `metadata` entries are
required. If a future change (#224) records the build that pruned,
it can add a metadata key then.

### D6 Phase ordering with #221

#221 lands the three-branch hash comparison on the same functions.
This change rebases onto it. The implementation touches the tail
of each function, after #221's branches, so a three-way merge has
no overlapping lines. The spec phase lists this as a dependency so
reviewers do not approve this change before its predecessor's
contracts are in place.

## Risks / Trade-offs

- **A file deleted and re-added with new content between two
  starts.** The second start re-creation sees a missing page and
  inserts the new file — correct. No special case is needed.
- **A store directory removed entirely.** `ReadDir` fails with
  `IsNotExist`; the pass treats the file set as empty and prunes
  every page for that source. This matches `reIngestLearnings`'s
  existing early return for a missing learnings directory — both
  treat "no directory means no files". Any other `ReadDir`
  error (permissions, I/O, transient fault) or a
  `ListPagesBySource` error skips pruning for that pass and logs
  a warning: pruning fails closed so a transient fault can never
  mass-delete pages (LR-FR-003/CKS-FR-003). A store removed from
  `knowledge-stores.yaml` rather than from disk is out of
  scope — see Non-Goals.
- **Concurrent file deletion during the scan.** The scan captures
  directory entries once; a file removed after `ReadDir` but
  before the store listing is still an orphan and is deleted.
  A file created in the same window is re-ingested on the next
  start.
- **Existing tests.** Every existing re-ingest and store test
  asserts on pages whose files exist; adding pruning does not
  change those fixtures. A vault without deletions sees the same
  log output as before (no pruned line), so behavior is
  byte-identical for non-affected cases.
- **Manual deletion without a git commit.** The index converges to
  the files on disk regardless of git state. Git is not consulted.

## Constitution Alignment

References the assessments in proposal.md.

- **Autonomous Collaboration (PASS):** files stay the durable
  record; a deletion committed and pulled converges without out of
  band signals.
- **Composability First (PASS):** no config, no flags, no new
  deps; vaults without deletions pay one extra query and no new
  behavior.
- **Observable Quality (PASS):** pruning counts are logged like the
  re-ingest counts, so the index state is auditable at rest.
- **Testability (PASS):** both functions keep their `(store,
  embedder, vaultPath)` shape and are tested with an in-memory
  store and `t.TempDir()` fixtures; orphan cases are added without
  external services.

<!-- scaffolded by uf vdev -->
