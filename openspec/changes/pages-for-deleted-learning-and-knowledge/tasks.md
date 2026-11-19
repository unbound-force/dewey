<!--
  [P] marks tasks eligible for parallel execution.
  Add [P] when a task: (a) touches different files from
  other [P] tasks in the same group, (b) has no dependency
  on prior tasks in the group, (c) can safely execute
  without ordering constraints.
  Do NOT add [P] when tasks modify the same file —
  parallel workers will cause merge conflicts.
  Tasks without [P] run sequentially first, then [P]
  tasks run in parallel.
-->

## 1. Implementation

- [x] 1.1 `main.go`: add orphan pruning to the tail of
  `reIngestLearnings` — after the directory scan, list
  `learning/*` pages by `source_id = "learning"`, build the
  file identity set from the entries just scanned (same filters
  and `ValidLearningIdentity` guard), delete orphans via
  `DeletePage` (cascades to blocks/embeddings), and return or
  log the count when > 0 (LR-FR-003, design D1/D2/D4). Fail
  closed: only a not-exist directory counts as an empty file
  set; any other scan or listing error skips pruning with a
  warning. Per-page delete failures warn and continue; the
  count logs successes only. No new embeddings; unchanged-file
  path untouched.
- [x] 1.2 `tools/curate.go`: add orphan pruning to the tail of
  `IndexKnowledgeStore` — after the store directory scan, list
  pages for `knowledge-{store}` source, build the file name set
  from the entries just scanned (same filters: skip non-`.md`,
  `_index.md`, dot-files), delete orphans via `DeletePage`, and
  extend the return to `(indexed, pruned int)` so both callers
  (`reIndexKnowledgeStores`, `autoIndexKnowledgeStore`) adapt
  (CKS-FR-003, design D1/D2/D4). Fail closed: only a not-exist
  directory counts as an empty file set; other scan or listing
  errors skip pruning with a warning. Per-page delete failures
  warn and continue; the count logs successes only. No
  embeddings; stores removed from `knowledge-stores.yaml` are
  out of scope.
- [x] 1.3 `main.go`: aggregate per-store prune counts in
  `reIndexKnowledgeStores` (from the `(indexed, pruned)`
  return) and log when > 0, matching the existing re-index
  count log (CKS-FR-003, design D4). Must not introduce a
  schema change or new `metadata` key.

## 2. Tests

- [x] 2.1 `main_test.go`: `TestReIngestLearnings_`
  `PrunesDeletedLearning` — setup: two learnings indexed,
  delete one `.md` file, run `reIngestLearnings`, assert the
  deleted identity's `learning/*` page and its blocks are gone
  and the survivor remains; plus `NoFileNoPrune` (no deletion
  when files intact), `MissingDirPrunesAll`, and
  `TransientScanErrorSkipsPrune` (unreadable directory prunes
  nothing) cases (LR-FR-003).
- [x] 2.2 `tools/curate_test.go`: `TestIndexKnowledgeStore_`
  `PrunesDeletedPage` — setup: store with two files indexed,
  delete one, run `IndexKnowledgeStore`, assert the deleted
  `knowledge/<store>/*` page and its blocks are gone and the
  survivor remains; plus `IgnoresIndexAndDotFiles`, `MissingDir`,
  and `TransientScanErrorSkipsPrune` (unreadable directory
  prunes nothing) cases (CKS-FR-003).
- [x] 2.3 `main_test.go` and `tools/curate_test.go`:
  regression — run `go test -race -count=1 ./...` with the
  change applied and confirm every existing re-ingest and
  knowledge-store test still passes; unchanged-file fixtures
  must still report zero embeddings and zero prune logs when no
  file was deleted (epic constraint: startup behavior for
  vaults not hitting these cases stays the same).

## 3. Verification

- [x] 3.1 CI parity: `go build -v ./...`, `go vet ./...`,
  `go test -race -count=1 -coverprofile=coverage.out ./...`,
  and the Gaze threshold gate from `.github/workflows/ci.yml`
  (`gaze crap --format=json --coverprofile=... --max-crapload=`
  `48 --max-gaze-crapload=45 ./...`).
- [x] 3.2 Manual check: in a temp vault with one learning and
  one store file indexed, delete each file, restart the
  server (call `reIngestLearnings` + `reIndexKnowledgeStores`
  or `dewey serve`), and verify `ListPagesBySource` no longer
  returns the deleted pages and `semantic_search` no longer
  returns them, while unchanged pages still return.
- [x] 3.3 Constitution alignment: confirm no new dependency or
  config, no schema change, no file format change, and that
  both prunes are covered by tests (constitution III and IV;
  proposal alignment).

<!-- scaffolded by uf vdev -->
