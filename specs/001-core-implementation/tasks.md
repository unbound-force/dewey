# Tasks: Dewey Core Implementation

**Input**: Design documents from `/specs/001-core-implementation/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/

**Tests**: Tests are included for all new packages. The constitution (Principle IV: Testability) requires testable packages. Existing graphthulhu tests serve as the backward compatibility gate.

**Organization**: Tasks are grouped by user story. US1 (Persistence) must complete before US2 (Vector Search) or US3 (Content Sources) can begin. US4 (CLI) tasks are distributed across the stories they enable.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3, US4)
- Include exact file paths in descriptions

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Add new dependencies and create package scaffolding

- [x] T001 Add `modernc.org/sqlite` v1.47.0 dependency via `go get modernc.org/sqlite@v1.47.0` in go.mod
- [x] T002 Add `github.com/k3a/html2text` v1.4.0 dependency via `go get github.com/k3a/html2text@v1.4.0` in go.mod
- [x] T002A Add `github.com/spf13/cobra` dependency via `go get github.com/spf13/cobra@latest` in go.mod
- [x] T002B Add `github.com/charmbracelet/log` dependency via `go get github.com/charmbracelet/log@latest` in go.mod
- [x] T003 [P] Create store/ package directory with store.go containing Store struct and constructor in store/store.go
- [x] T004 [P] Create embed/ package directory with embed.go containing Embedder interface definition in embed/embed.go
- [x] T005 [P] Create source/ package directory with source.go containing Source interface definition in source/source.go
- [x] T006 Run `go build ./...` to verify new dependencies resolve and existing code compiles

**Checkpoint**: Dependencies added, package scaffolding in place, existing tests still pass.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: SQLite store foundation that all user stories depend on

**CRITICAL**: No user story work can begin until this phase is complete

- [x] T007 Implement schema migration system with version tracking and up/down migrations in store/migrate.go
- [x] T008 Implement SQLite database initialization with WAL mode, foreign keys, and schema creation (pages, blocks, links, metadata tables per data-model.md) in store/store.go
- [x] T009 Implement Page CRUD operations (InsertPage, GetPage, ListPages, UpdatePage, DeletePage) with content_hash change detection in store/store.go
- [x] T010 Implement Block CRUD operations (InsertBlock, GetBlock, GetBlocksByPage, DeleteBlocksByPage) with hierarchical parent/child support in store/store.go
- [x] T011 Implement Link operations (InsertLink, GetForwardLinks, GetBackwardLinks, DeleteLinksByPage) in store/store.go
- [x] T012 Implement IndexMetadata operations (GetMeta, SetMeta) for schema_version, page_count, last_full_index_at tracking in store/store.go
- [x] T013 Write tests for store package: test all CRUD operations, schema migration, content_hash change detection, and cascade deletes using in-memory SQLite in store/store_test.go
- [x] T014 Run `go test ./...` to verify store package tests pass and existing tests remain green
- [x] T014A Refactor CLI from flag.FlagSet to cobra: create root command with version, create serve/journal/add/search subcommands matching existing behavior, replace flag parsing with cobra flags, preserve all existing CLI contracts (FR-026) in cmd/ directory structure
- [x] T014B Migrate all logging from fmt.Fprintf(os.Stderr, ...) to charmbracelet/log: replace log prefixes in main.go, cli.go, vault/vault.go, and server.go with structured logger (FR-027)
- [x] T014C Write tests for refactored CLI commands: verify each existing subcommand (serve, journal, add, search, version) produces identical behavior after cobra migration in cmd/root_test.go
- [x] T014D Add `go-custom.md` convention pack override for AP-007 (flat package layout retained from graphthulhu fork) in .opencode/unbound/packs/go-custom.md
- [x] T014E Run `go test ./...` to verify CLI refactor and logging migration do not break existing tests
- [x] T014F Remove `windows` from `.goreleaser.yaml` goos list -- target macOS and Linux only in .goreleaser.yaml

**Checkpoint**: Foundation ready -- SQLite store with full CRUD for pages, blocks, links. CLI refactored to cobra. Logging migrated to charmbracelet/log. All user story implementation can now begin.

---

## Phase 3: User Story 1 - Persistent Knowledge Graph Index (Priority: P1) MVP

**Goal**: Persist the knowledge graph to SQLite so subsequent sessions load from disk with incremental updates only. All 37 existing MCP tools produce identical results.

**Independent Test**: Start Dewey on a test vault, verify `.dewey/graph.db` is created. Stop, modify one file, restart, confirm only the changed file is re-indexed. Run `go test ./...` to confirm all existing tools pass.

### Implementation for User Story 1

- [x] T015 [US1] Create a VaultStore adapter that bridges the existing vault.Client interface with the store.Store persistence layer, converting between in-memory types (types.PageEntity, types.BlockEntity) and store operations in vault/vault_store.go
- [x] T016 [US1] Implement incremental indexing logic: on startup, load persisted pages from store, compare content_hash of each local .md file against stored hash, re-index only changed/new files, remove deleted files in vault/vault_store.go
- [x] T017 [US1] Implement first-run full-index path: when no `.dewey/graph.db` exists, build complete index from all .md files and persist to store in vault/vault_store.go
- [x] T018 [US1] Implement corruption detection: validate schema_version on load, catch SQLite errors, fall back to full re-index with warning logged to stderr in vault/vault_store.go
- [x] T019 [US1] Update the file system watcher (vault/vault.go watchLoop) to persist index changes to store alongside in-memory updates when a persistent store is available in vault/vault.go
- [x] T020 [US1] Update main.go serve command to initialize Store, create `.dewey/` directory if needed, and pass the store to the vault client for persistent indexing in main.go
- [x] T021 [US1] Implement `dewey init` CLI subcommand: create `.dewey/` directory, write default config.yaml (embedding model setting) and sources.yaml (disk source only), append `.dewey/` to .gitignore if needed in cli.go
- [x] T022 [US1] Implement `dewey status` CLI subcommand: load store, report page count, block count, source info, and index path to stdout (text format); support --json flag for structured output in cli.go
- [x] T023 [US1] Register `init` and `status` subcommands in main.go command routing alongside existing serve/journal/add/search/version in main.go
- [x] T024 [US1] Write tests for VaultStore adapter: test full-index, incremental-index (changed/new/deleted files), and corruption recovery using testdata fixtures and in-memory SQLite in vault/vault_store_test.go
- [x] T025 [P] [US1] Write tests for dewey init and dewey status CLI commands: test directory creation, default config content, idempotent re-init, status output format (text and JSON), uninitialized error handling in cli_test.go
- [x] T026A [P] [US1] Write tests for corrupted/incompatible index detection and full re-index recovery in store/store_test.go
- [x] T026B [P] [US1] Write tests for disk space exhaustion: verify Dewey continues from in-memory index when SQLite write fails in store/store_test.go
- [x] T026C [P] [US1] Write tests for concurrent `.dewey/` access with file-level locking in store/store_test.go
- [x] T026D [US1] Run `go test ./...` to verify all 37 existing MCP tool tests still pass with the persistence layer active (backward compatibility gate SC-003)

**Checkpoint**: Dewey persists its index to `.dewey/graph.db`. Subsequent startups load from disk and only re-index changed files. `dewey init` and `dewey status` CLI commands work. All existing tests pass. Edge cases (corruption, disk space, concurrency) are tested.

---

## Phase 4: User Story 2 - Semantic Search via Vector Embeddings (Priority: P2)

**Goal**: Generate and persist vector embeddings for indexed content. Expose 3 new MCP tools for semantic search. Gracefully degrade when embedding model is unavailable.

**Independent Test**: Index a test vault, issue a semantic search with different terminology, verify conceptually related results returned. Verify graceful degradation when Ollama is not running.

### Implementation for User Story 2

- [x] T026 [P] [US2] Implement Embedder interface and OllamaEmbedder: HTTP client for POST /api/embed, model availability check via GET /api/tags, batch embedding support, connection error handling in embed/embed.go
- [x] T027 [P] [US2] Implement block-to-chunk preparation (Chunker): prepend heading hierarchy context path to block content, handle token length considerations in embed/chunker.go
- [x] T028 [US2] Implement embedding storage: add embeddings table to store schema, InsertEmbedding, GetEmbedding, GetAllEmbeddings, DeleteEmbeddingsByBlock operations, store vectors as float32 BLOBs in store/embeddings.go
- [x] T029 [US2] Implement brute-force cosine similarity search: load all embeddings into memory, compute cosine similarity against query vector, return top-k results sorted by score with configurable threshold in store/embeddings.go
- [x] T030 [US2] Implement filtered similarity search: combine vector similarity with metadata filters (source_type, source_id, has_property, has_tag) by joining embeddings with pages table in store/embeddings.go
- [x] T031 [US2] Implement embedding types: SemanticSearchInput, SimilarInput, SemanticSearchFilteredInput, and SemanticSearchResult with provenance metadata fields (source, source_id, similarity, indexed_at) in types/tools.go
- [x] T032 [US2] Implement dewey_semantic_search MCP tool: accept query string, embed via Embedder, search via store cosine similarity, return results with provenance metadata per contracts/mcp-tools.md in tools/semantic.go
- [x] T033 [US2] Implement dewey_similar MCP tool: accept page name or block UUID, look up existing embedding, search for similar blocks via store, return ranked results per contracts/mcp-tools.md in tools/semantic.go
- [x] T034 [US2] Implement dewey_semantic_search_filtered MCP tool: accept query + filters (source_type, source_id, has_property, has_tag), use filtered similarity search from store per contracts/mcp-tools.md in tools/semantic.go
- [x] T035 [US2] Register 3 new semantic search MCP tools in server.go, conditional on Embedder availability (register tools always, but return error message when embedder unavailable) in server.go
- [x] T036 [US2] Update health MCP tool to include Dewey-specific fields: embeddingModel, embeddingAvailable, embeddingCount, embeddingCoverage, sources list per contracts/mcp-tools.md in tools/navigate.go
- [x] T037 [US2] Integrate embedding generation into the indexing pipeline: after a page is indexed (or re-indexed), generate embeddings for its blocks via Embedder and persist via store in vault/vault_store.go
- [x] T038 [US2] Update `dewey status` CLI to include embedding coverage, model name, and model availability in cli.go
- [x] T039 [P] [US2] Write tests for OllamaEmbedder using httptest mock server: test embed, batch embed, model check, connection refused, model not found errors in embed/embed_test.go
- [x] T040 [P] [US2] Write tests for Chunker: test heading hierarchy context prepending, edge cases (no headings, deeply nested), content truncation in embed/chunker_test.go
- [x] T041 [P] [US2] Write tests for embedding storage and cosine similarity: test insert/get/delete, cosine ranking correctness with known fixture vectors, filtered search with metadata in store/embeddings_test.go
- [x] T042A [P] [US2] Write tests for semantic search MCP tools: test dewey_semantic_search, dewey_similar, dewey_semantic_search_filtered with mock Embedder and in-memory store, verify input validation, error messages per contracts/mcp-tools.md, provenance metadata fields including origin_url, empty index behavior, embedding-unavailable degradation, and dewey_similar "neither page nor uuid" error in tools/semantic_test.go
- [x] T042B [P] [US2] Write test for updated health MCP tool: verify Dewey-specific fields (embeddingModel, embeddingAvailable, embeddingCount, embeddingCoverage, sources array) are present and correctly populated in tools/navigate_test.go
- [x] T042C [P] [US2] Write test for removed embedding model: verify keyword tools work, semantic tools return clear error when model is unavailable in tools/semantic_test.go
- [x] T042D [US2] Run `go test ./...` to verify all tests pass including semantic search tools and backward compatibility

**Checkpoint**: Dewey generates and persists embeddings. 3 new MCP tools serve semantic search. Health tool reports embedding status. Graceful degradation when Ollama unavailable. All edge cases tested.

---

## Phase 5: User Story 3 - Pluggable Content Sources (Priority: P3)

**Goal**: Support multiple content sources (disk, GitHub, web crawl) with a pluggable interface, configurable refresh intervals, and local caching.

**Independent Test**: Configure a GitHub source, run `dewey index`, verify issues appear in search results with correct provenance. Run again and verify incremental fetch.

### Implementation for User Story 3

- [x] T043 [P] [US3] Define Source interface contract: List() []Document, Fetch(id) Document, Diff() []Change, Meta() SourceMetadata methods, plus Document and Change types in source/source.go
- [x] T044 [P] [US3] Implement source configuration parsing: load `.dewey/sources.yaml`, parse source entries with type-specific config schemas (disk, github, web), validate required fields in source/config.go
- [x] T045 [P] [US3] Implement disk source: refactor existing vault file scanning into the Source interface, implement List/Fetch/Diff using filesystem and content_hash comparison in source/disk.go
- [x] T046 [US3] Implement sources table operations in store: InsertSource, GetSource, ListSources, UpdateSourceStatus, UpdateLastFetched per data-model.md schema in store/store.go
- [x] T047 [P] [US3] Implement GitHub source: use `gh` CLI auth token, fetch issues/PRs/READMEs via GitHub REST API, convert to Document format, support incremental fetch via updated_at timestamps in source/github.go
- [x] T048 [P] [US3] Implement web crawl source: HTTP client with rate limiting and robots.txt parsing, HTML-to-text via k3a/html2text, configurable crawl depth, local cache in `.dewey/cache/` in source/web.go
- [x] T049 [US3] Implement source manager: orchestrate fetching across all configured sources, check refresh intervals, handle source failures gracefully (log warning, continue with others), report summary in source/manager.go
- [x] T050 [US3] Implement `dewey index` CLI subcommand: load sources config, invoke source manager, index fetched documents into store, generate embeddings if available, support --source and --force flags per contracts/cli-commands.md in cli.go
- [x] T051 [US3] Implement `dewey source add` CLI subcommand: validate arguments, append source entry to `.dewey/sources.yaml`, support github and web source types per contracts/cli-commands.md in cli.go
- [x] T052 [US3] Register `index` and `source` subcommands in main.go command routing in main.go
- [x] T053 [US3] Update `dewey status` CLI to include per-source status (type, page count, last fetched, status, error message) in cli.go
- [x] T054 [P] [US3] Write tests for source config parsing: test valid configs, missing fields, unknown source types in source/config_test.go
- [x] T055 [P] [US3] Write tests for disk source: test List/Fetch/Diff with testdata fixtures in source/disk_test.go
- [x] T056 [P] [US3] Write tests for GitHub source: test API calls with recorded HTTP responses via httptest, test incremental fetch, test rate limit handling in source/github_test.go
- [x] T057 [P] [US3] Write tests for web crawl source: test HTML-to-text conversion, robots.txt compliance, rate limiting, depth limiting with local httptest server in source/web_test.go
- [x] T058A [P] [US3] Write tests for source manager: test refresh interval enforcement, source failure isolation (one source fails, others continue), multi-source orchestration, summary reporting in source/manager_test.go
- [x] T058B [P] [US3] Write tests for dewey index and dewey source add CLI commands: test uninitialized error, source add validation, duplicate source rejection, index with fixture sources in cli_test.go
- [x] T058C [P] [US3] Write test for GitHub API rate limit handling: verify partial fetch, warning, and continuation in source/github_test.go
- [x] T058D [P] [US3] Write test for non-HTML content (PDF, binary) skip with warning during web crawl in source/web_test.go
- [x] T058E [US3] Run `go test ./...` to verify all tests pass including source integration and backward compatibility

**Checkpoint**: All 3 source types work. `dewey index` fetches and indexes external content. `dewey source add` configures new sources. Sources cached locally with refresh intervals. Source manager, CLI commands, and edge cases all tested.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Final validation, edge cases, and cleanup

- [x] T059 Implement file-level locking for `.dewey/` directory to prevent concurrent write corruption (edge case from spec) in store/store.go
- [x] T060 Implement disk space error handling: catch SQLite write errors, log warning, continue operating from in-memory index (edge case from spec) in store/store.go
- [x] T061 Update README.md with Dewey-specific documentation: new CLI commands (init, index, status, source add), MCP tool descriptions (semantic_search, similar, semantic_search_filtered), setup workflow in README.md
- [x] T062 Update .goreleaser.yaml to include new dependencies in the release binary build in .goreleaser.yaml
- [x] T063 Run full `go build ./...`, `go test ./...`, `go vet ./...` suite to verify clean build, all tests pass, no vet warnings
- [x] T064 Run backward compatibility validation: confirm all 37 original MCP tools produce identical results with persistence active (SC-003)
- [x] T065 Write end-to-end integration test: dewey init → dewey index (fixture vault) → MCP tool queries (keyword + semantic) → verify results with provenance metadata including origin_url → dewey status → verify output in integration_test.go
- [x] T066 Write benchmark test for incremental startup: create 200-file fixture vault, persist index, modify 3 files, measure time from store.Open() to ready-to-serve, assert <2s per SC-001 in vault/vault_store_test.go
- [x] T067 Configure Homebrew tap distribution: add `brews:` section to .goreleaser.yaml pointing to unbound-force/homebrew-tap, configure formula with Ollama as recommended dependency (SC-005) in .goreleaser.yaml

**Checkpoint**: All user stories verified. Edge cases handled. Integration and performance benchmarks pass. Documentation updated. Ready for PR.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies -- can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion -- BLOCKS all user stories
- **US1 Persistence (Phase 3)**: Depends on Foundational (Phase 2)
- **US2 Vector Search (Phase 4)**: Depends on US1 (Phase 3) -- needs store for embedding persistence
- **US3 Content Sources (Phase 5)**: Depends on US1 (Phase 3) -- needs store for source caching. Can run in parallel with US2 if store interface is stable.
- **Polish (Phase 6)**: Depends on all user story phases

### User Story Dependencies

- **US1 (P1)**: Depends on Foundational (Phase 2) only. No other story dependencies. This IS the MVP.
- **US2 (P2)**: Depends on US1 -- needs the store package for embedding persistence and the vault_store adapter for the indexing pipeline integration.
- **US3 (P3)**: Depends on US1 -- needs the store package for source table operations and caching. Partially parallelizable with US2 (source interface work is independent of embedding work).
- **US4 (P4)**: CLI tasks are distributed: `init`/`status` in US1, `index`/`source add` in US3. Not a separate phase.

### Within Each User Story

- Store/model operations before service logic
- Service logic before MCP tool registration
- MCP tool registration before CLI command integration
- Tests written alongside implementation (same phase)

### Parallel Opportunities

Within Phase 4 (US2): T026 (OllamaEmbedder), T027 (Chunker) can run in parallel since they're in different files with no dependencies on each other. T039, T040, T041 (tests) can run in parallel.

Within Phase 5 (US3): T043 (Source interface), T044 (config parsing) can run in parallel. T054-T057 (tests) can all run in parallel.

Between Phases: Once US1 is complete, US2 and US3 can be partially parallelized -- US2's embedding work (T026-T027) and US3's source interface work (T043-T045) have no direct dependency on each other.

---

## Parallel Example: User Story 2

```bash
# Phase 4 parallel opportunities:
# Step 1 - These can run in parallel (different files, no deps):
Task: "T026 [P] [US2] Implement OllamaEmbedder in embed/embed.go"
Task: "T027 [P] [US2] Implement Chunker in embed/chunker.go"

# Step 2 - After T026+T027 complete, tests can run in parallel:
Task: "T039 [P] [US2] Write OllamaEmbedder tests in embed/embed_test.go"
Task: "T040 [P] [US2] Write Chunker tests in embed/chunker_test.go"
Task: "T041 [P] [US2] Write embedding storage tests in store/embeddings_test.go"
```

---

## Parallel Example: User Story 3

```bash
# Phase 5 parallel opportunities:
# Step 1 - Interface and config can be done in parallel:
Task: "T043 [P] [US3] Define Source interface in source/source.go"
Task: "T044 [P] [US3] Implement config parsing in source/config.go"

# Step 2 - All three source implementations can be parallelized:
Task: "T045 [US3] Implement disk source in source/disk.go"
Task: "T047 [US3] Implement GitHub source in source/github.go"
Task: "T048 [US3] Implement web crawl source in source/web.go"

# Step 3 - All tests can run in parallel:
Task: "T054 [P] [US3] Write config tests in source/config_test.go"
Task: "T055 [P] [US3] Write disk source tests in source/disk_test.go"
Task: "T056 [P] [US3] Write GitHub source tests in source/github_test.go"
Task: "T057 [P] [US3] Write web crawl source tests in source/web_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001-T006)
2. Complete Phase 2: Foundational (T007-T014)
3. Complete Phase 3: User Story 1 (T015-T025)
4. **STOP and VALIDATE**: Run `go test ./...`, verify `.dewey/graph.db` is created, verify incremental indexing, verify `dewey init` and `dewey status` work
5. Dewey is now a meaningful upgrade over graphthulhu: persistent index with faster restarts

### Incremental Delivery

1. Setup + Foundational → SQLite store ready
2. US1 (Persistence) → MVP: persistent index, init/status CLI
3. US2 (Vector Search) → Semantic search: 3 new MCP tools, embedding pipeline
4. US3 (Content Sources) → Cross-repo context: GitHub + web crawl, index/source CLI
5. Polish → Edge cases, docs, release prep
6. Each story adds value without breaking previous stories

---

## Notes

- [P] tasks = different files, no dependencies on incomplete tasks
- [Story] label maps task to specific user story for traceability
- US4 (CLI) tasks are distributed across US1 and US3 since CLI commands are thin wrappers around underlying capabilities
- The `dewey serve` command backward compatibility (FR-025) is verified by the existing test suite running after each story phase
- Commit after each completed phase for clean git history
- Stop at any checkpoint to validate the story independently
