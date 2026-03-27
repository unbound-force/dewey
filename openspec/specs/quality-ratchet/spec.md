## Requirements

### Requirement: Decomposed Event Handling

The `(*Client).handleEvent` method in `vault/vault.go` MUST dispatch to per-event-type handler methods. Each handler MUST be independently testable.

#### Scenario: File creation triggers handleFileWrite
- **GIVEN** a `.md` file is created in the vault directory
- **WHEN** the fsnotify watcher delivers a Create event
- **THEN** `handleEvent` delegates to `handleFileWrite` which indexes the file, rebuilds backlinks, and optionally persists to store

#### Scenario: File deletion triggers handleFileRemove
- **GIVEN** a `.md` file is deleted from the vault directory
- **WHEN** the fsnotify watcher delivers a Remove event
- **THEN** `handleEvent` delegates to `handleFileRemove` which removes the page from the in-memory index and optionally removes it from the store

#### Scenario: File rename triggers handleFileRename
- **GIVEN** a `.md` file is renamed in the vault directory
- **WHEN** the fsnotify watcher delivers a Rename event
- **THEN** `handleEvent` delegates to `handleFileRename` which removes the old page name from the index (the new name will trigger a Create event)

#### Scenario: Non-markdown files are ignored
- **GIVEN** a file without the `.md` extension is created in the vault directory
- **WHEN** the fsnotify watcher delivers any event
- **THEN** `handleEvent` returns without processing

### Requirement: Decomposed Server Tool Registration

The `newServer` function in `server.go` MUST use per-category registration helper functions. Each helper MUST register only the tools in its category.

#### Scenario: Navigate tools are registered by helper
- **GIVEN** a Backend implementation is provided
- **WHEN** `newServer` is called
- **THEN** `registerNavigateTools` registers all navigate category tools (get_page, get_block, list_pages, get_links, traverse, and conditionally get_references)

#### Scenario: Write tools are skipped in read-only mode
- **GIVEN** `readOnly` is true
- **WHEN** `newServer` is called
- **THEN** `registerWriteTools` is not called and no write tools are registered

#### Scenario: Total tool count is unchanged
- **GIVEN** any valid backend and server configuration
- **WHEN** `newServer` is called with the same parameters as before this change
- **THEN** the total number of registered tools MUST be identical to the pre-change count

### Requirement: Decomposed Incremental Indexing

The `(*VaultStore).IncrementalIndex` method in `vault/vault_store.go` MUST split its logic into a vault walk phase and a page diff phase, each independently testable.

#### Scenario: Walk phase returns file inventory
- **GIVEN** a vault directory with markdown files
- **WHEN** the walk phase executes
- **THEN** it returns a map of page names to content hashes and a map of page names to file metadata (path, content, info)

#### Scenario: Diff phase identifies changes
- **GIVEN** a set of current file hashes and a set of stored hashes
- **WHEN** the diff phase executes
- **THEN** it returns categorized lists: new pages (in current but not stored), changed pages (hash differs), and deleted pages (in stored but not current)

#### Scenario: Incremental index produces identical results
- **GIVEN** a vault with a known set of files
- **WHEN** `IncrementalIndex` is called before and after this refactoring
- **THEN** the returned `IndexStats` MUST be identical and the store state MUST be identical

### Requirement: GoDoc-Enhanced Type Classification

All `UnmarshalJSON` methods in `types/logseq.go` MUST have GoDoc comments that explicitly state the interface contract, including the `implements json.Unmarshaler` phrasing.

#### Scenario: BlockEntity UnmarshalJSON has contract GoDoc
- **GIVEN** the `(*BlockEntity).UnmarshalJSON` method
- **WHEN** Gaze analyzes its GoDoc comment
- **THEN** the godoc signal SHOULD contribute a positive weight toward contractual classification

#### Scenario: PageRef UnmarshalJSON has contract GoDoc
- **GIVEN** the `(*PageRef).UnmarshalJSON` method
- **WHEN** Gaze analyzes its GoDoc comment
- **THEN** the godoc signal SHOULD contribute a positive weight toward contractual classification

### Requirement: Q3 Test Contract Strengthening

Existing tests for Q3-classified functions in `tools/` MUST be strengthened with assertions that verify observable side effects, not merely exercise code paths.

#### Scenario: GetWhiteboard test asserts return structure
- **GIVEN** a mock backend with whiteboard data
- **WHEN** `GetWhiteboard` is called
- **THEN** the test MUST assert that the returned result contains the expected page connections, embedded pages, and block references from the mock data

#### Scenario: JournalSearch test asserts search results
- **GIVEN** a mock backend with journal entries matching a search query
- **WHEN** `JournalSearch` is called
- **THEN** the test MUST assert that the returned results contain the expected matching blocks with correct date context

#### Scenario: AnalysisHealth test asserts health status
- **GIVEN** a mock backend with analysis pages having varying link counts
- **WHEN** `AnalysisHealth` is called
- **THEN** the test MUST assert that pages with fewer than 3 links and no decisions are flagged as unhealthy

#### Scenario: FindByTag test asserts tag hierarchy
- **GIVEN** a mock backend with pages tagged with parent and child tags
- **WHEN** `FindByTag` is called
- **THEN** the test MUST assert that results include both direct tag matches and child tag matches

#### Scenario: TopicClusters test asserts cluster structure
- **GIVEN** a mock backend with two disconnected groups of pages
- **WHEN** `TopicClusters` is called
- **THEN** the test MUST assert that exactly two clusters are returned, each with the correct hub page identified

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

### Requirement: CI Quality Gate Compliance

The project MUST pass all Gaze CI quality gates with headroom. Actual CI thresholds (from `.github/workflows/ci.yml`):
- CRAPload MUST be <= 15
- GazeCRAPload MUST be <= 34

Post-ratchet-2 actuals: CRAPload=10, GazeCRAPload=33.
