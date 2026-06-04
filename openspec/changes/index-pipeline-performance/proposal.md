## Why

The `dewey index` command takes 30-60+ seconds on repos with
multiple sources. Three compounding bottlenecks cause this:

1. **Per-block embedding**: `GenerateEmbeddings()` calls
   `embedder.Embed()` once per block -- a separate HTTP
   round-trip to Ollama for each block. The `EmbedBatch()`
   method exists on both `OllamaEmbedder` and `VertexEmbedder`
   but is never used during indexing.

2. **Sequential source fetching**: `FetchAll()` iterates
   sources in a `for` loop with zero concurrency. With 18+
   sources (disk, GitHub, web), each source blocks the next.

3. **Sequential document indexing**: `indexDocuments()`
   processes sources and documents sequentially. Parse,
   persist, and embed operations for each document block
   until the next document can start.

This makes `dewey index` (and `uf init`, which invokes it)
appear to hang and degrades the developer experience when
onboarding new projects or re-indexing.

## What Changes

1. Replace per-block `Embed()` calls with batched
   `EmbedBatch()` calls in `GenerateEmbeddings()`
2. Add concurrency to `FetchAll()` using `errgroup` with
   bounded workers
3. Add concurrency to `indexDocuments()` for parallelizing
   across sources

## Capabilities

### New Capabilities
- `batch-embedding`: Embedding generation uses `EmbedBatch()`
  to send multiple chunks per HTTP request, reducing
  round-trips by 10-50x
- `concurrent-fetch`: Source fetching runs with bounded
  concurrency via `errgroup`
- `concurrent-index`: Document indexing parallelizes across
  independent source namespaces

### Modified Capabilities
- `GenerateEmbeddings`: Collects chunks across a page's block
  tree and calls `EmbedBatch()` instead of per-block `Embed()`
- `FetchAll`: Fetches sources concurrently with a configurable
  worker limit
- `indexDocuments`: Processes sources concurrently with a
  worker limit

### Removed Capabilities
- None

## Impact

- `vault/parse_export.go`: `GenerateEmbeddings()` refactored
  to collect chunks and call `EmbedBatch()`
- `source/manager.go`: `FetchAll()` refactored to use
  `errgroup` with bounded concurrency
- `cli.go`: `indexDocuments()` refactored for concurrent
  source processing
- `embed/embed.go`: No changes needed (batch API already
   implemented)
- Existing tests must be updated to cover concurrent behavior
- No configuration changes. One new direct dependency:
   `golang.org/x/sync/errgroup` -- provides bounded
   concurrency with error propagation via `SetLimit()`.
   The Go standard library's `sync.WaitGroup` does not
   support worker limits or first-error collection,
   making errgroup the idiomatic choice. This is a
   well-maintained Go sub-repository with no transitive
   dependencies.

## Constitution Alignment

Assessed against the Unbound Force org constitution.

### I. Autonomous Collaboration

**Assessment**: N/A

This change is internal to Dewey's indexing pipeline. It does
not affect artifact-based communication between heroes. The
same indexed data is produced, just faster.

### II. Composability First

**Assessment**: PASS

Dewey remains independently installable and usable. The
`Embedder` interface is unchanged. `FetchAll` and
`indexDocuments` signatures are preserved. The only new
dependency (`golang.org/x/sync/errgroup`) is a Go
sub-repository with no transitive dependencies.

### III. Observable Quality

**Assessment**: PASS

The indexed output (pages, blocks, embeddings, links in
`graph.db`) is identical regardless of whether processing
was sequential or concurrent. Logging and error reporting
are preserved -- source-level errors remain non-fatal per
001-FR-020 (source failure resilience).

### IV. Testability

**Assessment**: PASS

The `Embedder` interface already supports test doubles.
`FetchAll` operates on the `Source` interface. Concurrent
behavior can be tested with mock sources and embedders
without requiring Ollama or network access.
