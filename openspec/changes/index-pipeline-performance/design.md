## Context

`dewey index` processes sources, documents, and embeddings
entirely sequentially. Three levels of serialization compound:

1. `FetchAll()` in `source/manager.go` iterates sources in a
   `for` loop -- each `src.List()` blocks until complete
2. `indexDocuments()` in `cli.go` has nested sequential loops
   over sources then documents
3. `GenerateEmbeddings()` in `vault/parse_export.go` calls
   `embedder.Embed()` per block -- one HTTP round-trip to
   Ollama per block, despite `EmbedBatch()` being fully
   implemented on both `OllamaEmbedder` and `VertexEmbedder`

Per the proposal's constitution alignment, this change is
internal performance optimization. The indexed output
(graph.db contents) is identical. The `Embedder` and `Source`
interfaces are unchanged (Composability). Error handling
semantics are preserved (Observable Quality).

## Goals / Non-Goals

### Goals
- Reduce `dewey index` wall-clock time by 5-10x on typical
  configurations (18 sources, 200+ pages)
- Use `EmbedBatch()` to reduce HTTP round-trips to Ollama
  by batching embedding requests
- Add bounded concurrency to source fetching and document
  indexing
- Preserve identical indexed output (same graph.db contents)
- Preserve non-fatal error handling for source failures
  (001-FR-020: source failure resilience)

### Non-Goals
- Changing the web source rate limit (1s/request is a
  politeness constraint, not a bug)
- Background/async indexing (covered by spec 012)
- Changing the `Embedder` or `Source` interfaces
- Adding new CLI flags or configuration options for
  concurrency tuning (hardcoded defaults are fine for now;
  can be exposed later if needed)

## Decisions

### D1: Batch size of 32 for EmbedBatch

Collect up to 32 chunks before calling `EmbedBatch()`.
Rationale: Ollama's `/api/embed` endpoint accepts arrays
natively. 32 balances memory usage (32 * 768 chars max per
chunk) against round-trip reduction. The batch size is a
constant, not configurable -- avoids premature abstraction.

Truncated-chunk retry (context length overflow) falls back
to single `Embed()` for the failing chunk only, preserving
existing error recovery.

### D2: Flatten-then-batch in GenerateEmbeddings

The current implementation recurses into `b.Children` via
recursive `GenerateEmbeddings()` calls. To batch effectively,
flatten the block tree first (collect all non-empty blocks
with their block UUIDs and heading paths as `(blockUUID,
chunk, headingPath)` tuples), then batch-embed the collected
chunks. This replaces the recursive per-block approach with
a two-pass approach: collect, then embed. The block UUID
association is preserved so `InsertEmbedding` receives the
correct UUID for each embedding.

### D3: errgroup with bounded workers for FetchAll

Use `errgroup` with `SetLimit(4)` for concurrent source
fetching. Four concurrent fetches balance I/O parallelism
against system resource usage (each source may open files,
make HTTP requests, or call GitHub APIs).

Source failures remain non-fatal: each goroutine returns
`nil` from the errgroup perspective but records failures
in the `FetchResult`. This preserves 001-FR-020 semantics
(source failure resilience: failures are logged as warnings,
other sources continue). Both the `FetchResult` and the
`allDocs map[string][]Document` return values require
mutex-protected writes from concurrent goroutines.

### D4: Concurrent source-level indexing in indexDocuments

Process sources concurrently in `indexDocuments()` using
`errgroup.WithContext` and `SetLimit(4)`. On the first
persistence error (block/link write failure), the context
is cancelled, stopping remaining source goroutines. This
matches the current hard-error semantics and avoids wasted
work after a fatal error. Each source writes to a distinct
page namespace (`sourceID/docID`), so there are no write
conflicts in the store.

Within each source, documents are processed sequentially.
This avoids SQLite write contention -- the store uses
`SetMaxOpenConns(1)`, so all writes serialize through the
Go `database/sql` connection pool regardless of SQLite's
WAL-level locking.

Note: The existing `indexMu *sync.Mutex` in `main.go`
protects the `indexDocuments()` call site. All errgroup
goroutines run within the mutex-held scope and complete
before `indexDocuments()` returns (guaranteed by
`errgroup.Wait()`). The overall wall-clock time reduction
means background curation starvation (which uses `TryLock`
on the same mutex) is reduced, not increased.

### D5: Store thread safety

The store uses `SetMaxOpenConns(1)`, which means all
concurrent write operations serialize through Go's
`database/sql` connection pool. SQLite WAL mode supports
concurrent reads but writes are single-threaded regardless.
Concurrent source-level indexing means multiple goroutines
write to the store, but serialization at the connection pool
level prevents any data integrity issues. This is acceptable
because the bottleneck is embedding generation (HTTP I/O to
Ollama), not store writes. The `busy_timeout=5000` pragma
(5 seconds) provides tolerance for queued write attempts.

## Risks / Trade-offs

- **Risk**: Batch embedding may fail for a subset of chunks
  (e.g., one chunk exceeds context length). **Mitigation**:
  If `EmbedBatch()` fails, fall back to per-chunk `Embed()`
  for the batch, preserving existing truncation retry logic.
- **Risk**: Concurrent source fetching increases peak memory
  (multiple sources loaded simultaneously). **Mitigation**:
  Worker limit of 4 bounds the maximum concurrent memory
  usage. Typical source payloads are small (markdown files).
- **Risk**: SQLite write contention under concurrent indexing.
  **Mitigation**: WAL mode + source-level granularity (not
  document-level) limits contention. Write serialization is
  acceptable since I/O-bound embedding dominates.
- **Trade-off**: Log output from concurrent sources may
  interleave. Acceptable since each log line includes the
  source ID.

### D6: Shared indexDocuments function

The CLI (`cli.go`) and MCP reindex tool
(`tools/indexing.go`) each have their own
`indexDocuments()` with the same sequential bottleneck.
Rather than duplicating the concurrency refactor in both,
extract a shared `IndexDocuments()` function (exported,
in an appropriate package) that both call sites invoke.
This eliminates the duplication permanently and ensures
both CLI and MCP paths benefit from the performance
improvement.

## Scope Note

The concurrency changes to `FetchAll()` and
`GenerateEmbeddings()` also benefit `dewey reindex` and
background indexing during `dewey serve`, since both code
paths call the same functions.
