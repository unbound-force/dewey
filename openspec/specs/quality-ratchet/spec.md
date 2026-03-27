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

### Requirement: CI Quality Gate Compliance

The project MUST pass all Gaze CI quality gates. Actual CI thresholds (from `.github/workflows/ci.yml`):
- CRAPload MUST be <= 15
- GazeCRAPload MUST be <= 34

Post-implementation actuals: CRAPload=13, GazeCRAPload=34.
