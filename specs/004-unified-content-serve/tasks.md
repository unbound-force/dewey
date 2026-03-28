# Tasks: Unified Content Serve

**Input**: Design documents from `/specs/004-unified-content-serve/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/

**Tests**: Test tasks are included because the constitution (Principle IV) requires contract-level tests for all new exported functions, and the Verification Strategy specifies unit, integration, and regression test categories.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story. US2 (content persistence) is ordered before US1 (search) because US2 is a prerequisite — content must be persisted before it can be served.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup

**Purpose**: No new project initialization needed — this feature modifies existing packages. Setup phase creates the one new file and verifies the development environment.

- [x] T001 Create `vault/parse_export.go` with exported `ParseDocument(docID, content string, modTime time.Time) (props map[string]any, blocks []types.BlockEntity)` function wrapping `parseFrontmatter()` and `parseMarkdownBlocks()` per research R2
- [x] T002 [P] Verify development environment: `go build ./...`, `go vet ./...`, `go test -race -count=1 ./...` all pass on current `main` state

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Store methods and vault data model changes that ALL user stories depend on. No user story work can begin until this phase is complete.

**CRITICAL**: No user story work can begin until this phase is complete.

- [x] T003 Add `ListPagesExcludingSource(sourceID string) ([]*Page, error)` method to `store/store.go` — query: `SELECT ... FROM pages WHERE source_id != ?` ordered by name
- [x] T004 [P] Add `DeletePagesBySource(sourceID string) (int64, error)` method to `store/store.go` — query: `DELETE FROM pages WHERE source_id = ?` with CASCADE, returning rows affected. Wrap in transaction for atomicity (FR-013)
- [x] T005 [P] Add `ListPagesBySource(sourceID string) ([]*Page, error)` method to `store/store.go` — query: `SELECT ... FROM pages WHERE source_id = ?` ordered by name (FR-010)
- [x] T006 Add `sourceID string` and `readOnly bool` fields to `cachedPage` struct in `vault/vault.go`. Set `sourceID = "disk-local"` and `readOnly = false` in `parseFile()` for all disk-loaded pages
- [x] T007 [P] Add `reconstructBlockTree(flat []*store.Block) []types.BlockEntity` function to `vault/vault_store.go` per research R5 — convert flat `[]*store.Block` with `ParentUUID`/`Position` into nested `[]types.BlockEntity` with `Children`
- [x] T008 Write tests for `ParseDocument()` in `vault/parse_export_test.go` — test markdown with headings, frontmatter, plain text (no headings), and empty content
- [x] T009 [P] Write tests for `ListPagesExcludingSource`, `DeletePagesBySource`, `ListPagesBySource` in `store/store_test.go` — use in-memory SQLite, verify correct filtering, CASCADE behavior, and row count
- [x] T010 [P] Write tests for `reconstructBlockTree` in `vault/vault_store_test.go` — test flat→tree conversion with nested blocks, single root, multiple roots, and empty input

**Checkpoint**: Foundation ready. `ParseDocument()`, store source methods, `reconstructBlockTree()`, and `cachedPage` struct changes are all in place. User story implementation can now begin.

---

## Phase 3: User Story 2 — Full Content Persistence During Indexing (Priority: P1)

**Goal**: Upgrade `dewey index` to parse documents into blocks, links, and embeddings — not just page metadata. Prerequisite for US1.

**Independent Test**: Run `dewey index` with a GitHub source, then query `graph.db` directly to verify blocks, links, and embeddings exist for external pages.

### Implementation

- [x] T011 [US2] Upgrade `indexDocuments()` in `cli.go` to call `vault.ParseDocument(doc.ID, doc.Content, doc.FetchedAt)` for each document, generating blocks and extracting frontmatter properties
- [x] T012 [US2] After parsing, persist blocks to store via `store.InsertBlock()` (recursive, matching `VaultStore.persistBlocks()` pattern in `vault/vault_store.go:508-530`). Delete existing blocks first for re-index (FR-004 replace strategy)
- [x] T013 [US2] After parsing, extract wikilinks from blocks via `parser.Parse(block.Content)` and persist as `store.InsertLink()` records (FR-002). Delete existing links first for re-index
- [x] T014 [US2] Create an `embed.OllamaEmbedder` in `newIndexCmd()` in `cli.go` using `DEWEY_EMBED_ENDPOINT` / `DEWEY_EMBED_MODEL` env vars per research R4. Call `embedder.Available()` and log warning if unavailable (FR-003 graceful degradation)
- [x] T015 [US2] If embedder is available, generate embeddings per-block after parsing and persist via `store.InsertEmbedding()`. Skip gracefully if unavailable (FR-003)
- [x] T016 [US2] Implement namespace prefixing for external page names: `strings.ToLower(sourceID + "/" + docID)` per research R6 in `indexDocuments()` in `cli.go` (FR-007)
- [x] T017 [US2] Implement auto-purge: at the start of `dewey index`, compare source IDs from `sources.yaml` against source IDs in store. Call `store.DeletePagesBySource()` for orphaned sources and log count (FR-013)
- [x] T018 [US2] Add structured logging at phase boundaries in `indexDocuments()` in `cli.go`: log start/completion of parsing, link extraction, embedding generation with counts and elapsed time (FR-014)
- [x] T019 [US2] Write integration test in `cli_test.go` or `integration_test.go`: populate an in-memory store with `indexDocuments()` using mock `source.Document` data, verify blocks, links exist in store. Verify re-index replaces old blocks when content hash changes (FR-004)
- [x] T020 [US2] Write test for auto-purge behavior: insert pages for two sources, remove one source from config, run purge logic, verify only the removed source's pages are deleted (FR-013)

**Checkpoint**: `dewey index` now persists full content (blocks, links, embeddings) for all source documents. External content is recoverable from `graph.db` without re-fetching. Re-indexing replaces stale content. Orphaned sources are auto-purged.

---

## Phase 4: User Story 1 — Search Across All Indexed Sources (Priority: P1) MVP

**Goal**: Teach `dewey serve` to load external-source pages from `graph.db` on startup, making them queryable via all MCP tools.

**Independent Test**: Run `dewey index` with a GitHub source, start `dewey serve`, and use `dewey_search` MCP tool — results MUST include GitHub-sourced content.

### Implementation

- [x] T021 [US1] Implement `LoadExternalPages(c *Client) (int, error)` on `VaultStore` in `vault/vault_store.go` — call `store.ListPagesExcludingSource("disk-local")`, for each page: convert `store.Page` → `types.PageEntity`, load blocks via `store.GetBlocksByPage()`, reconstruct tree via `reconstructBlockTree()`, build `cachedPage` with `sourceID` and `readOnly = true`, call `c.applyPageIndex(page)` (FR-005)
- [x] T022 [US1] Integrate `LoadExternalPages()` into startup in `main.go` — call after `indexVault()` completes but before `vc.BuildBacklinks()`, so external pages participate in backlink and search index construction. Log count of loaded pages (FR-014)
- [x] T023 [US1] Add structured logging in `LoadExternalPages()` in `vault/vault_store.go`: log start, page count, block count, and elapsed time (FR-014)
- [x] T024 [US1] Update `dewey status` command in `cli.go` to report per-source page counts by calling `store.ListPagesBySource()` for each source or `store.CountPagesBySource()` (FR-010)
- [x] T025 [US1] Write test for `LoadExternalPages` in `vault/vault_store_test.go` — pre-populate in-memory store with external pages + blocks, call `LoadExternalPages()`, verify pages appear in vault's `pages` map with correct `sourceID`, `readOnly`, and reconstructed block trees
- [x] T026 [US1] Write integration test: pre-populate store with external pages, create vault, load external pages, call `BuildBacklinks()`, verify external pages are searchable via `FullTextSearch()` and appear in `GetAllPages()` results

**Checkpoint**: `dewey serve` loads external pages from `graph.db` on startup. All 40 MCP tools return results including external-source content. `dewey status` shows per-source page counts. This is the MVP.

---

## Phase 5: User Story 4 — Read-Only Protection for External Content (Priority: P2)

**Goal**: Write operations on external-source pages return clear errors. Local pages remain fully writable.

**Independent Test**: Load an external-source page, call `dewey_update_block` on its block — verify error response with source identification.

### Implementation

- [x] T027 [US4] Add write guard check to `AppendBlockInPage()` in `vault/vault.go` — if target page has `readOnly == true`, return `fmt.Errorf("page %q is read-only (source: %s)", page.entity.Name, page.sourceID)` (FR-008)
- [x] T028 [P] [US4] Add write guard check to `PrependBlockInPage()` in `vault/vault.go` — same pattern as T027
- [x] T029 [P] [US4] Add write guard check to `UpdateBlock()` in `vault/vault.go` — look up block in `blockIndex`, find owning page, check `readOnly` (FR-008)
- [x] T030 [P] [US4] Add write guard check to `RemoveBlock()` in `vault/vault.go` — same pattern as T029
- [x] T031 [P] [US4] Add write guard check to `InsertBlock()` in `vault/vault.go` — check if referenced parent block belongs to a read-only page (FR-008)
- [x] T032 [P] [US4] Add write guard check to `MoveBlock()` in `vault/vault.go` — check both source and target blocks for read-only pages (FR-008)
- [x] T033 [P] [US4] Add write guard check to `DeletePage()` in `vault/vault.go` — check `readOnly` before file deletion (FR-008)
- [x] T034 [P] [US4] Add write guard check to `RenamePage()` in `vault/vault.go` — check `readOnly` before rename (FR-008)
- [x] T035 [US4] Write tests for all 8 write guards in `vault/vault_test.go` — for each guarded method: create a `cachedPage` with `readOnly = true`, call the method, verify error message contains page name and source ID. Also test that `readOnly = false` pages pass through without error (FR-009)

**Checkpoint**: All 8 write methods reject external pages with clear error messages. Local pages continue to work normally. No in-memory state corruption on rejected writes.

---

## Phase 6: User Story 3 — Cross-Source Backlinks (Priority: P2)

**Goal**: Wikilinks in external content create backlinks to local pages (and vice versa).

**Independent Test**: Create a local page, create a GitHub issue referencing it via `[[page-name]]`, index, serve, and query backlinks — the GitHub issue appears as a referring page.

### Implementation

- [x] T036 [US3] Verify that `BuildBacklinks()` in `vault/vault.go` already processes external pages loaded by `LoadExternalPages()` — since external pages are added to `c.pages` via `applyPageIndex()`, `buildBacklinks()` (in `vault/index.go`) should iterate them automatically. Write a test confirming this behavior rather than new code.
- [x] T037 [US3] Write test for cross-source backlinks in `vault/vault_store_test.go` or `vault/vault_test.go` — create a local page "architecture", create an external page with block content `[[architecture]]`, load both into vault, call `BuildBacklinks()`, verify `GetPageLinkedReferences("architecture")` includes the external page as a backlink source
- [x] T038 [US3] Write test for bidirectional external-to-external backlinks — two external pages referencing each other via wikilinks, verify both directions appear in backlinks

**Checkpoint**: Cross-source backlinks work naturally. No additional code needed beyond what US1 and US2 provide — this phase is primarily verification and testing.

---

## Phase 7: User Story 5 — Semantic Search Across All Sources (Priority: P3)

**Goal**: `dewey_semantic_search` returns results from external-source content when embeddings are available.

**Independent Test**: Index GitHub content with Ollama running, call `dewey_semantic_search` — external-source results appear with similarity scores.

### Implementation

- [x] T039 [US5] Verify that embeddings generated during `dewey index` (T015) are queryable by `store.SearchSimilar()` and `store.SearchSimilarFiltered()` — these existing methods already query the `embeddings` table joined with `pages`. Write a test confirming external-source embeddings appear in semantic search results.
- [x] T040 [US5] Write test for graceful degradation in `cli_test.go` or `integration_test.go` — run indexing without Ollama available, verify blocks and links are still persisted but no embeddings exist. Verify keyword search still works for external content (FR-003)

**Checkpoint**: Semantic search includes external-source content when embeddings are available. Graceful degradation when Ollama is unavailable.

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: Verification, documentation, and quality assurance across all stories.

- [x] T041 Run full CI-equivalent checks: `go build ./...`, `go vet ./...`, `go test -race -count=1 ./...` — all must pass
- [x] T042 Run `go test -race -count=1 -coverprofile=coverage.out ./...` and verify Gaze quality gates: `gaze report ./... --coverprofile=coverage.out --max-crapload=15 --max-gaze-crapload=34` — no regressions
- [x] T043 [P] Verify backward compatibility: confirm all existing tool tests pass without modification with external pages loaded in the vault (FR-011, SC-005)
- [x] T044 [P] Update `README.md` if `dewey index` or `dewey status` output format changed. Update `dewey index` section to mention block/link/embedding persistence.
- [x] T045 [P] Update `AGENTS.md` Active Technologies section to reflect 004 changes if not already done by the agent context update script
- [x] T046 Validate quickstart.md scenarios end-to-end: run `dewey index` with a configured source, `dewey serve`, verify search returns external content, verify write rejection

**Checkpoint**: All tests pass, Gaze quality gates met, backward compatibility verified, documentation updated.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately
- **Foundational (Phase 2)**: Depends on Phase 1 (T001 creates `parse_export.go` used by T008)
- **US2 (Phase 3)**: Depends on Phase 2 — uses `ParseDocument()`, store methods, `reconstructBlockTree()`
- **US1 (Phase 4)**: Depends on Phase 3 (US2) — external content must be persisted before it can be loaded
- **US4 (Phase 5)**: Depends on Phase 2 only — `cachedPage.readOnly` field must exist, but no dependency on US1/US2 content
- **US3 (Phase 6)**: Depends on Phase 4 (US1) — pages must be loadable to test backlinks
- **US5 (Phase 7)**: Depends on Phase 3 (US2) — embeddings must be generated during indexing
- **Polish (Phase 8)**: Depends on all prior phases

### User Story Dependencies

- **US2 (P1)**: Can start after Foundational (Phase 2) — No dependencies on other stories
- **US1 (P1)**: Depends on US2 — content must exist in store before vault can load it
- **US4 (P2)**: Can start after Foundational (Phase 2) — Independent of US1/US2 (only needs `readOnly` field)
- **US3 (P2)**: Depends on US1 — backlinks require external pages in the vault
- **US5 (P3)**: Depends on US2 — embeddings must be generated during indexing

### Parallel Opportunities

- T002, T003, T004, T005 can run in parallel (different methods in different locations)
- T007, T008, T009, T010 can run in parallel (different test files)
- T027-T034 can all run in parallel (different write methods in the same file, no interdependencies)
- US4 (Phase 5) can run in parallel with US1 (Phase 4) after US2 completes

---

## Parallel Example: Phase 2 (Foundational)

```bash
# Launch parallelizable store methods:
Task: "Add ListPagesExcludingSource() to store/store.go"      # T003
Task: "Add DeletePagesBySource() to store/store.go"            # T004 [P]
Task: "Add ListPagesBySource() to store/store.go"              # T005 [P]

# Launch parallelizable vault functions:
Task: "Add reconstructBlockTree() to vault/vault_store.go"     # T007 [P]

# Launch parallelizable tests:
Task: "Write tests for ParseDocument() in vault/parse_export_test.go"  # T008
Task: "Write tests for store source methods in store/store_test.go"    # T009 [P]
Task: "Write tests for reconstructBlockTree in vault/vault_store_test.go"  # T010 [P]
```

## Parallel Example: Phase 5 (Write Guards)

```bash
# All 8 write guards can be applied in parallel (same file, different methods):
Task: "Add write guard to AppendBlockInPage()"    # T027
Task: "Add write guard to PrependBlockInPage()"   # T028 [P]
Task: "Add write guard to UpdateBlock()"           # T029 [P]
Task: "Add write guard to RemoveBlock()"           # T030 [P]
Task: "Add write guard to InsertBlock()"           # T031 [P]
Task: "Add write guard to MoveBlock()"             # T032 [P]
Task: "Add write guard to DeletePage()"            # T033 [P]
Task: "Add write guard to RenamePage()"            # T034 [P]
```

---

## Implementation Strategy

### MVP First (US2 + US1)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (CRITICAL — blocks all stories)
3. Complete Phase 3: US2 — Full Content Persistence
4. Complete Phase 4: US1 — Search Across All Sources
5. **STOP and VALIDATE**: External content is searchable via MCP tools
6. Deploy/demo if ready — this is the core value

### Incremental Delivery

1. Setup + Foundational → Foundation ready
2. Add US2 (content persistence) → Content persisted in graph.db
3. Add US1 (serve from store) → **MVP: External content searchable** 
4. Add US4 (write guards) → Safety: external content is read-only
5. Add US3 (backlinks) → Cross-source knowledge graph
6. Add US5 (semantic search) → Full semantic search across all sources
7. Polish → Documentation, quality gates, backward compat verification

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- US2 is ordered before US1 because it is a prerequisite (content must be persisted before served)
- US4 (write guards) can be implemented in parallel with US1 after foundational phase
- FR-012 (WAL mode) requires no tasks — already enabled per research R1
- Each user story should be independently testable at its checkpoint
- Commit after each task or logical group
