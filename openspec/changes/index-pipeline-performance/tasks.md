## 1. Batch Embedding (FR-100)

- [ ] 1.1 Refactor `GenerateEmbeddings()` in
  `vault/parse_export.go` to flatten the block tree into a
  list of `(block, chunk, headingPath)` tuples, then batch
  embed using `EmbedBatch()` with a batch size of 32.
  Add fallback: if `EmbedBatch()` fails, retry the batch
  individually via `Embed()` with existing truncation logic.
  **Files**: `vault/parse_export.go`

- [ ] 1.2 Add tests for batch embedding: verify batch calls
  reduce round-trips, verify fallback on batch failure,
  verify empty blocks are skipped, verify embedding count
  is correct.
  **Files**: `vault/parse_export_test.go`

## 2. Concurrent Source Fetching (FR-101)

- [ ] 2.1 Refactor `FetchAll()` in `source/manager.go` to
  use `errgroup` with `SetLimit(4)` for concurrent source
  fetching. Preserve non-fatal error handling (FR-020):
  source failures are recorded in `FetchResult` but do not
  cancel other goroutines. Thread-safe result aggregation
  via mutex.
  **Files**: `source/manager.go`

- [ ] 2.2 Add tests for concurrent fetching: verify
  concurrency via synchronization barrier (per FR-101
  scenario), verify single-source filter bypasses
  concurrency, verify one source failure does not block
  others. Ensure mock sources are thread-safe (atomic or
  mutex-protected counters) for `-race` compatibility.
  **Files**: `source/manager_test.go`

## 3. Shared Concurrent Document Indexing (FR-102, D6)

- [ ] 3.1 Extract a shared `IndexDocuments()` function from
  `cli.go:indexDocuments()` and `tools/indexing.go:indexDocuments()`.
  The shared function uses `errgroup.WithContext` with
  `SetLimit(4)` for concurrent source processing. On the
  first persistence error, the context is cancelled and
  remaining goroutines stop. Each source processes its
  documents sequentially. Thread-safe accumulation of
  `totalIndexed` counter. Preserve per-source logging with
  source ID. Both CLI and MCP reindex call sites invoke the
  shared function.
  **Files**: `cli.go`, `tools/indexing.go`, and a new shared
  location (e.g., `vault/index.go` or `indexer/` package)

- [ ] 3.2 Add tests for concurrent indexing: verify total
  count is accurate across concurrent sources, verify
  source-level errors cancel remaining goroutines, verify
  concurrency via synchronization barrier (per FR-102
  scenario).
  **Files**: test file alongside the shared function

## 4. Verification

- [ ] 4.1 Run CI-equivalent checks: `go build ./...`,
  `go vet ./...`, `go test -race -count=1 ./...`, and
  confirm all pass. Derive exact commands from
  `.github/workflows/` per CI Parity Gate.
- [ ] 4.2 Manual smoke test: run `dewey index` on a repo
  with multiple sources. Capture before/after wall-clock
  times to verify performance improvement. Verify indexed
  output matches expectations.
- [ ] 4.3 Verify constitution alignment: composability
  (Embedder/Source interfaces unchanged), observable quality
  (same graph.db output), testability (mock embedder/source
  used in tests)
- [ ] 4.4 Documentation impact: No updates needed -- this
  change is internal performance optimization with no
  user-facing behavior changes (same commands, same flags,
  same output format). AGENTS.md, README.md, and website
  documentation are unaffected. No website documentation
  sync issue required (exempt: internal refactor).
