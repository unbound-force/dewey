# Proposal: `dewey reindex` Restores Learnings and Knowledge Stores

## Why

Issue #223. `dewey reindex` deletes `graph.db` and rebuilds it from the
configured sources with `vault.IndexDocuments`. Learnings
(`.uf/dewey/learnings/`) and knowledge-store pages
(`.uf/dewey/knowledge/<store>/`) are file-backed but reached by no
source — the vault scan and disk sources skip hidden directories — so
they only come back when `dewey serve` starts and runs
`reIngestLearnings` and `reIndexKnowledgeStores`. Until then anything
reading the index (`dewey status`, `dewey search`, a CI job) sees no
learnings and no curated pages. gh-hud works around it by starting a
short `dewey serve` after every reindex and waiting for a log line
(jflowers/gh-hud#138).

## What Changes

1. **`restoreFileBackedPages`** (`main.go`): the two startup passes —
   `reIngestLearnings` and `reIndexKnowledgeStores`, with their
   existing warning/info logging — move into one function. `dewey
   serve`'s deferred index calls it exactly where it ran them before.
2. **`dewey reindex`** (`cli.go`): calls `restoreFileBackedPages` after
   `IndexDocuments` succeeds and before the index-build stamp, on the
   store it just created. The command already holds exclusive use of
   `graph.db` (it refuses to run while `dewey serve` holds the lock).
3. **Docs**: `reindex` long help, README `dewey reindex` section,
   AGENTS.md CLI table and File-Backed Learnings.

`dewey index` (including `--force`) is unchanged: it does not delete
`graph.db`, so learnings and store pages are still there, and serve
startup keeps converging changed files.

## Why this, not runtime file watching

The harness enhancement (E3b) offered two options: watch
`.uf/dewey/learnings/` while serving, or finish #223. This change is
the smaller one: two existing, tested functions called from one more
place, no new goroutine, watcher, debounce or locking against
`store_learning`'s own writes. Runtime watching remains a separate,
larger change.

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- `learning-reingest` — learnings (and knowledge-store pages) are also
  restored by `dewey reindex`, not only at `dewey serve` startup. This
  lifts the learning-reingest-on-file-change non-goal "Covering
  learnings from the `dewey index` / `dewey reindex` CLI commands" for
  `reindex`.

### Removed Capabilities
- None.

## Impact

- **Files**: `main.go`, `cli.go`, `cli_test.go`, `README.md`,
  `AGENTS.md`.
- **Runtime**: one extra pass at the end of `reindex`: reads every
  learning and store file, embeds them when the embedder is available
  (as a fresh `serve` start would).
- **Index format**: unchanged — the same pages serve startup writes,
  so no `IndexFormat` bump.
- **Website**: `dewey reindex` behavior changes (user-visible) — docs
  sync issue required.
- **Consumers**: gh-hud can drop its post-reindex warm-up
  (jflowers/gh-hud#138).

## Constitution Alignment

Assessed against the Unbound Force org constitution.

### I. Autonomous Collaboration

**Assessment**: PASS

The files stay the durable record; any process reading `graph.db`
after a reindex sees the full index without coordinating with a server.

### II. Composability First

**Assessment**: PASS

No new flag, dependency or configuration. Vaults with no learnings and
no `knowledge-stores.yaml` behave as before (both passes are no-ops).

### III. Observable Quality

**Assessment**: PASS

The same log lines serve emits ("learnings re-ingested from files",
"knowledge stores re-indexed from files", and warnings on failure) now
appear in the reindex log.

### IV. Testability

**Assessment**: PASS

`TestReindexCmd_RestoresFileBackedPages` runs the real command against
a temp vault with `--no-embeddings` and fails without the change.
