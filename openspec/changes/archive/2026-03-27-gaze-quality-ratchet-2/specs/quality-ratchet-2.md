## ADDED Requirements

### Requirement: Health Tool Test Coverage

The `registerHealthTool` function in `server.go` MUST have test coverage exercising all configuration combinations through the MCP server interface.

#### Scenario: Health tool with nil store and nil embedder
- **GIVEN** a server created with no store and no embedder options
- **WHEN** the `health` tool is called
- **THEN** the JSON response MUST contain `dewey.persistent == false`, `dewey.embeddingAvailable == false`, and `dewey.embeddingCount == 0`

#### Scenario: Health tool with persistent store and sources
- **GIVEN** a server created with a non-nil store containing pages, blocks, and source records
- **WHEN** the `health` tool is called
- **THEN** the JSON response MUST contain `dewey.persistent == true`, `dewey.embeddingCount` reflecting the actual embedding count, and `dewey.sources` as an array with per-source status

#### Scenario: Health tool with available embedder
- **GIVEN** a server created with a non-nil embedder whose `Available()` returns true
- **WHEN** the `health` tool is called
- **THEN** the JSON response MUST contain `dewey.embeddingAvailable == true` and `dewey.embeddingModel` matching the embedder's `ModelID()`

#### Scenario: Health tool reports backend ping error
- **GIVEN** a backend whose `Ping()` returns an error
- **WHEN** the `health` tool is called
- **THEN** the JSON response MUST contain `status` beginning with `"error:"`

### Requirement: Decomposed Index Command

The `newIndexCmd` function in `cli.go` MUST delegate to private helpers for source loading, document indexing, and embedding generation. Each helper MUST be independently testable.

#### Scenario: Index command orchestration
- **GIVEN** a valid `.dewey/` directory with `sources.yaml` and `graph.db`
- **WHEN** `dewey index` is executed
- **THEN** the command MUST call source loading, document indexing, and embedding generation in sequence

#### Scenario: Document indexing helper
- **GIVEN** a set of fetched documents from multiple sources
- **WHEN** the indexing helper is called
- **THEN** it MUST upsert pages and blocks into the store for each document and return the total count of indexed documents

### Requirement: Decomposed Semantic Similar

The `(*Semantic).Similar` method in `tools/semantic.go` MUST split its logic into input validation, query vector resolution, and result filtering, each independently testable.

#### Scenario: Input validation rejects missing fields
- **GIVEN** a `SimilarInput` with both `Page` and `UUID` empty
- **WHEN** the validation method is called
- **THEN** it MUST return an MCP error result indicating at least one field is required

#### Scenario: Query vector resolution by UUID
- **GIVEN** a `SimilarInput` with a non-empty `UUID`
- **WHEN** the vector resolution method is called
- **THEN** it MUST look up the embedding by UUID and return the vector

#### Scenario: Query vector resolution by page name
- **GIVEN** a `SimilarInput` with a non-empty `Page` and empty `UUID`
- **WHEN** the vector resolution method is called
- **THEN** it MUST find the first block with an embedding for that page and return its vector

#### Scenario: Result filtering excludes query document
- **GIVEN** similarity search results that include the query document itself
- **WHEN** the result filtering method is called
- **THEN** it MUST exclude the query document and return at most `limit` results

### Requirement: Navigate Test Contract Strengthening

Existing tests for `GetPage`, `GetBlock`, and `ListPages` in `tools/navigate_test.go` MUST verify observable return value structure, not merely check for non-error responses.

#### Scenario: GetPage test asserts block tree structure
- **GIVEN** a mock backend with a page containing nested blocks with links and properties
- **WHEN** `GetPage` is called
- **THEN** the test MUST assert that the response contains the correct block count, outgoing link names, backlink count, and property values from enriched blocks

#### Scenario: GetBlock test asserts ancestor chain
- **GIVEN** a mock backend with a block that has a parent page and sibling blocks
- **WHEN** `GetBlock` is called with `includeSiblings: true`
- **THEN** the test MUST assert that the response contains the ancestor chain and sibling blocks

#### Scenario: ListPages test asserts filtering and sorting
- **GIVEN** a mock backend with pages in multiple namespaces
- **WHEN** `ListPages` is called with a namespace filter
- **THEN** the test MUST assert that only pages matching the namespace are returned and they are in the expected sort order

### Requirement: Decomposed Status Command

The `newStatusCmd` function in `cli.go` MUST delegate to private helpers for store querying and output formatting. Each helper MUST be independently testable.

#### Scenario: Store query helper returns status data
- **GIVEN** a `.dewey/graph.db` with known page, block, and embedding counts
- **WHEN** the store query helper is called
- **THEN** it MUST return a struct containing accurate counts and source metadata

#### Scenario: Output formatting in JSON mode
- **GIVEN** a status data struct with known values
- **WHEN** the format helper is called with `jsonOutput == true`
- **THEN** it MUST produce valid JSON output with all expected fields

## MODIFIED Requirements

### Requirement: CI Quality Gate Headroom

The project MUST maintain headroom below the CI quality gates after this change. Previously: CRAPload=13 (gate: 15), GazeCRAPload=34 (gate: 34).

Updated targets:
- CRAPload MUST be <= 10 (down from 13)
- GazeCRAPload MUST be <= 30 (down from 34)

## REMOVED Requirements

None.
