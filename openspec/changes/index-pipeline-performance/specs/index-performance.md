## ADDED Requirements

### Requirement: FR-100 Batch Embedding Generation

`GenerateEmbeddings()` MUST use `EmbedBatch()` to send
multiple chunks per HTTP request instead of calling `Embed()`
per block. Chunks MUST be collected across the block tree
(including children) and batched into groups of up to 32.

#### Scenario: Batch embedding reduces round-trips
- **GIVEN** a page with 64 non-empty blocks
- **WHEN** `GenerateEmbeddings()` is called
- **THEN** `EmbedBatch()` is called exactly 2 times (2
  batches of 32), not 64 individual `Embed()` calls

#### Scenario: Batch failure falls back to individual
- **GIVEN** a batch of 32 chunks where one exceeds the
  model's context length
- **WHEN** `EmbedBatch()` returns any error
- **THEN** each chunk in the failed batch is retried
  individually via `Embed()`, including the existing
  truncation retry for context-length errors. Partial
  batch success is not expected (Ollama returns
  all-or-nothing). The fallback MUST be logged at WARN
  level with the error reason.

#### Scenario: Empty blocks are skipped
- **GIVEN** a page with 10 blocks, 3 of which are empty
  or whitespace-only
- **WHEN** `GenerateEmbeddings()` is called
- **THEN** only 7 chunks are collected for batching

### Requirement: FR-101 Concurrent Source Fetching

`FetchAll()` MUST fetch sources concurrently using bounded
workers. The worker limit MUST be 4 (defined as a named
constant).

#### Scenario: Independent sources fetch concurrently
- **GIVEN** 4 mock sources whose `List()` blocks on a
  shared barrier channel until all 4 goroutines are active
- **WHEN** `FetchAll()` is called
- **THEN** all 4 sources reach the barrier concurrently
  (proving parallel execution), and all complete
  successfully after the barrier is released

#### Scenario: Source failure does not cancel others
- **GIVEN** 3 sources configured, where source B fails
- **WHEN** `FetchAll()` is called
- **THEN** sources A and C complete successfully, and the
  `FetchResult` records source B's failure as a warning
  (non-fatal per 001-FR-020: source failure resilience)

#### Scenario: Source name filter still works
- **GIVEN** 5 sources configured and `sourceName` is set
  to "local"
- **WHEN** `FetchAll()` is called
- **THEN** only the "local" source is fetched (no
  concurrency needed for a single source)

### Requirement: FR-102 Concurrent Document Indexing

`indexDocuments()` MUST process sources concurrently using
bounded workers. The worker limit MUST be 4 (same named
constant as FR-101). Documents within a single source MAY
be processed sequentially.

#### Scenario: Sources indexed concurrently
- **GIVEN** 4 sources with mock documents, where the
  indexing callback blocks on a shared barrier channel
  until all 4 goroutines are active
- **WHEN** `indexDocuments()` is called
- **THEN** all 4 sources reach the barrier concurrently
  (proving parallel execution), and all complete
  successfully after the barrier is released

#### Scenario: Index totals are accurate
- **GIVEN** 3 sources with 10, 20, and 30 documents
  respectively
- **WHEN** `indexDocuments()` completes
- **THEN** the returned total equals 60

## MODIFIED Requirements

### Requirement: GenerateEmbeddings internal behavior

The `GenerateEmbeddings()` function signature MUST NOT
change. Batch size (32) is an internal constant. The
function MUST continue to return the count of successfully
generated embeddings. Internally, the implementation changes
from recursive per-block `Embed()` calls to a flatten-then-
batch approach using `EmbedBatch()`.

Previously: `GenerateEmbeddings` called `Embed()` per block
in a recursive loop.

### Requirement: FetchAll internal behavior

The `FetchAll()` function signature MUST NOT change
(returns `(*FetchResult, map[string][]Document)`). Internally,
the implementation changes from sequential source iteration
to concurrent fetching with mutex-protected result
aggregation. Non-fatal error handling (001-FR-020: source
failure resilience) is preserved.

## REMOVED Requirements

None.
