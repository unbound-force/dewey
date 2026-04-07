# Tasks: Live Reindex

**Input**: Design documents from `/specs/011-live-reindex/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, quickstart.md

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Types and Infrastructure

**Purpose**: Add shared types and server plumbing that all subsequent phases depend on

- [x] T001 [P] [US1] Add `IndexInput` struct (with optional `SourceID` field) and `ReindexInput` struct (empty) to `types/tools.go` — these are the MCP tool input types per plan D5
- [x] T002 [P] [US1] Add `vaultPath string` field to `serverConfig` struct and `WithVaultPath(p string) serverOption` constructor in `server.go` — follows the `WithEmbedder`/`WithPersistentStore` pattern per plan D4
- [x] T003 [US1] Pass `WithVaultPath(vp)` in `initObsidianBackend()` in `main.go` — append to `srvOpts` alongside the existing `WithPersistentStore(s)` call (line ~502)

**Checkpoint**: Types and server config are ready. `go build ./...` passes.

---

## Phase 2: Core Implementation — US1 Agent-Triggered Incremental Index (Priority: P1)

**Goal**: An agent can call the `index` MCP tool to fetch and index configured sources while the server is running

**Independent Test**: Configure a disk source, add a file, call `index`, verify the file appears in search results

### Implementation

- [x] T004 [US1] Create `tools/indexing.go` — define `Indexing` struct with `sync.Mutex`, `embed.Embedder`, `*store.Store`, `vaultPath string` fields; implement `NewIndexing(e, s, vaultPath)` constructor following the `Learning` struct pattern per plan D1
- [x] T005 [US1] Implement `Index` handler method on `Indexing` in `tools/indexing.go` — validate store non-nil, acquire mutex with `TryLock()` (return error result if locked per FR-005), load source configs from `sources.yaml`, build last-fetched times from store, create `source.Manager`, call `FetchAll`, orchestrate indexing pipeline (`vault.ParseDocument`, `vault.PersistBlocks`, `vault.PersistLinks`, `vault.GenerateEmbeddings`), return structured JSON summary per plan D2 and FR-004. Support optional `source_id` filter per FR-002

**Checkpoint**: `Index` handler compiles. `go build ./...` passes.

---

## Phase 3: Core Implementation — US2 Agent-Triggered Full Reindex (Priority: P2)

**Goal**: An agent can call the `reindex` MCP tool to delete external source content and rebuild from scratch

**Independent Test**: Index a source, delete a file from the source directory, call `reindex`, verify the deleted file's page is removed

### Implementation

- [x] T006 [US2] Implement `Reindex` handler method on `Indexing` in `tools/indexing.go` — validate store non-nil, acquire shared mutex with `TryLock()` (same mutex as `Index` per FR-005), load source configs, delete pages for each source where `cfg.ID != "disk-local"` and `cfg.ID != "learning"` via `store.DeletePagesBySource()` per FR-009/R5, create `source.Manager`, call `FetchAll` with `force=true`, orchestrate indexing pipeline, return structured JSON rebuild summary per plan D3

**Checkpoint**: Both `Index` and `Reindex` handlers compile. `go build ./...` passes.

---

## Phase 4: Server Registration

**Purpose**: Wire the indexing tools into the MCP server so they are discoverable by agents

- [x] T007 [US1] Add `registerIndexingTools(srv *mcp.Server, indexing *tools.Indexing) int` function to `server.go` — register `index` tool (description: trigger source re-indexing) and `reindex` tool (description: delete and rebuild external source index), following the `registerLearningTools` pattern (line ~391). Return count of 2
- [x] T008 [US1] Wire `registerIndexingTools` into `newServer()` in `server.go` — inside the second `!readOnly` block (near line ~87), create `tools.NewIndexing(cfg.embedder, cfg.store, cfg.vaultPath)` and add `registerIndexingTools(srv, indexing)` to `toolCount`. Indexing tools are write operations — excluded in read-only mode per plan D4
- [x] T009 [US1] Update tool count expectations in `server_test.go` `TestNewServer_ToolCount` — increase `wantMin`/`wantMax` by 2 for all read-write test cases: "non-DataScript read-write" (30→32, 35→37), "DataScript read-write" (38→40, 42→44), "vault read-write" (31→33, 36→38). Read-only cases unchanged (indexing tools are write-only per plan D8)

**Checkpoint**: `go test -race -count=1 .` passes with updated tool counts. Server registers 2 new tools in read-write mode.

---

## Phase 5: Slash Commands — US3 Slash Command Fallback (Priority: P3)

**Goal**: Developers can type `/dewey-index` or `/dewey-reindex` in OpenCode to trigger indexing via the agent

**Independent Test**: Type `/dewey-index` in OpenCode, verify the agent calls the `index` MCP tool

### Implementation

- [x] T010 [P] [US3] Create `.opencode/command/dewey-index.md` — slash command definition with frontmatter (description), instructions for the agent to call the `index` MCP tool with optional `source_id` parameter, expected output format (structured summary per FR-004). Follow existing command conventions (see `gaze.md` for pattern) per FR-010
- [x] T011 [P] [US3] Create `.opencode/command/dewey-reindex.md` — slash command definition with frontmatter (description), instructions for the agent to call the `reindex` MCP tool, include warning that this deletes and rebuilds all external source content (per US2 acceptance scenario 1), expected output format per FR-010

**Checkpoint**: Both slash command files exist and follow the `.opencode/command/` convention.

---

## Phase 6: Tests

**Purpose**: Verify tool handlers, error paths, mutual exclusion, and reindex safety

- [x] T012 [US1] Create `tools/indexing_test.go` with test cases per coverage strategy in plan.md:
  - `TestIndexing_Index_NilStore` — verify error result when store is nil (FR-008)
  - `TestIndexing_Index_ConcurrentCall` — verify mutex rejection with `TryLock()` (FR-005)
  - `TestIndexing_Reindex_NilStore` — verify error result when store is nil (FR-008)
  - `TestIndexing_Reindex_ConcurrentCall` — verify mutex rejection (FR-005)
  - `TestIndexing_Reindex_PreservesDiskLocal` — pre-populate store with `disk-local` source pages, run reindex, verify pages survive (FR-009)
  - `TestIndexing_Reindex_PreservesLearning` — pre-populate store with `learning` source pages, run reindex, verify pages survive (FR-009)
  - Use in-memory SQLite (`:memory:`) for store, standard library `testing` only, no external assertion libraries per testing conventions

**Checkpoint**: `go test -race -count=1 ./tools/ -run TestIndexing` passes. All test cases cover contract surface and error paths.

---

## Phase 7: Verification

**Purpose**: CI parity gate — replicate CI checks locally before declaring complete

- [x] T013 [US1] Run CI parity gate per AGENTS.md Technical Guardrails — read `.github/workflows/ci.yml` to identify exact commands, then execute: `go build ./...`, `go vet ./...`, `go test -race -count=1 ./...`. All must pass. Verify no regressions in existing tests (SC-005). Check documentation impact: update AGENTS.md if new patterns introduced, verify GoDoc comments on all exported symbols in `tools/indexing.go`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Types & Infrastructure)**: No dependencies — can start immediately
  - T001 and T002 are parallel (different files)
  - T003 depends on T002 (needs `WithVaultPath` to exist)
- **Phase 2 (Index Handler)**: Depends on T001 (needs `IndexInput` type) and T002 (needs `vaultPath` in config)
  - T004 depends on T001, T002
  - T005 depends on T004
- **Phase 3 (Reindex Handler)**: Depends on T004 (needs `Indexing` struct)
  - T006 depends on T004, T005 (shares mutex, same struct)
- **Phase 4 (Server Registration)**: Depends on T005, T006 (needs both handlers)
  - T007 depends on T005, T006
  - T008 depends on T002, T007
  - T009 depends on T008
- **Phase 5 (Slash Commands)**: No code dependencies — can run in parallel with Phase 4
  - T010 and T011 are parallel (different files)
- **Phase 6 (Tests)**: Depends on T005, T006 (needs handlers to test)
  - T012 can run in parallel with Phase 4 (different files)
- **Phase 7 (Verification)**: Depends on ALL previous phases

### Parallel Opportunities

```
Phase 1: T001 ─┐     T002 ─── T003
                │        │
Phase 2:        └── T004 ┘
                     │
                    T005
                     │
Phase 3:            T006
                     │
Phase 4:       T007 ─┤         Phase 5: T010 ┐ (parallel)
                T008 ┤                  T011 ┘
                T009 ┤
                     │         Phase 6: T012 (parallel with Phase 4)
Phase 7:            T013 (depends on all)
```

---

## Implementation Strategy

### Sequential (Single Developer)

1. Phase 1: T001 → T002 → T003 (types and plumbing)
2. Phase 2: T004 → T005 (Index handler)
3. Phase 3: T006 (Reindex handler)
4. Phase 4: T007 → T008 → T009 (server wiring)
5. Phase 5: T010, T011 (slash commands)
6. Phase 6: T012 (tests)
7. Phase 7: T013 (CI parity gate)

### Commit Strategy

- Commit after Phase 1 (types + infrastructure)
- Commit after Phase 3 (both handlers complete)
- Commit after Phase 4 (server wiring + test count update)
- Commit after Phase 5 + 6 (slash commands + tests)
- Final verification in Phase 7

---

## Notes

- The `indexDocuments()` function in `cli.go` (package main) cannot be imported by `tools/`. The `Indexing` struct reimplements the orchestration (~50 lines) calling the same shared functions from `vault/` — this is intentional per plan D6.
- The `sync.Mutex` is shared between `Index` and `Reindex` — they are mutually exclusive with each other, not just with themselves.
- Read-only mode excludes indexing tools entirely — no test count changes needed for read-only test cases.
- All test cases use in-memory SQLite (`:memory:`) and standard library `testing` — no external assertion libraries per AGENTS.md testing conventions.
<!-- spec-review: passed -->
