# Tasks: Background Index

**Input**: Design documents from `/specs/012-background-index/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, quickstart.md

**Organization**: Tasks are grouped into four phases: infrastructure changes to `server.go` and `tools/indexing.go`, core restructure of `executeServe()` and `initObsidianBackend()` in `main.go`, tests across both files, and CI verification.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Infrastructure (Server Config + Indexing Mutex)

**Purpose**: Add the `indexReady` and `indexMutex` plumbing to `server.go` and `tools/indexing.go` before restructuring `main.go`. These are leaf changes with no cross-file dependencies.

- [x] T001 [P] [US2] Add `indexReady *atomic.Bool` field to `serverConfig` in `server.go` and add `WithIndexReady(ready *atomic.Bool) serverOption` constructor. Update `registerHealthTool()` to include `"indexing": !cfg.indexReady.Load()` in the `dewey` info map when `cfg.indexReady` is non-nil (FR-007, D2). Import `sync/atomic`.

- [x] T002 [P] [US3] Update `NewIndexing()` in `tools/indexing.go` to accept an optional `*sync.Mutex` parameter: `NewIndexing(s *store.Store, e embed.Embedder, vaultPath string, mu *sync.Mutex) *Indexing`. When `mu` is non-nil, use it as the `Indexing.mu` field (external mutex injection per D1). When `mu` is nil, use an internally-created mutex (backward compatible). Change `mu` field type from `sync.Mutex` to `*sync.Mutex` and initialize in constructor. Update all `ix.mu.TryLock()` / `ix.mu.Unlock()` call sites to use the pointer. Update the `NewIndexing()` call in `server.go` `newServer()` to pass `cfg.indexMutex` (add `indexMutex *sync.Mutex` field and `WithIndexMutex(mu *sync.Mutex) serverOption` to `serverConfig`).

**Checkpoint**: `go build ./...` passes. `go test ./tools/...` passes (existing tests use `NewIndexing(s, nil, dir)` which still works with nil mutex creating an internal one). No behavioral change yet.

---

## Phase 2: Core Restructure (executeServe + initObsidianBackend)

**Purpose**: Restructure the startup sequence so the MCP server starts before vault indexing. This is the core change — ~50 lines in `main.go`.

**CRITICAL**: Phase 1 must be complete before this phase begins.

- [x] T003 [US1] Split `initObsidianBackend()` in `main.go` — remove the three slow operations: `indexVault(vc)` call (lines 559-567), `LoadExternalPages` block (lines 573-582), and `vc.Watch()` block (lines 585-592). The function retains vault path resolution, store open, Ollama check, vault creation, and embedder setup. It must also return the `*vault.Client` (via type assertion or additional return value) so the caller can pass it to the background goroutine. Update the cleanup function to NOT close the store on indexVault error (that path is removed). The function signature may add a `*vault.Client` return or the caller type-asserts `b.(*vault.Client)`.

- [x] T004 [US1] Add background indexing goroutine to `executeServe()` in `main.go`. After `newServer()` and before `runServer()`, create `indexMu := &sync.Mutex{}` and `indexReady := &atomic.Bool{}`, pass them via `WithIndexMutex(indexMu)` and `WithIndexReady(indexReady)` server options. Launch a goroutine that: (1) acquires `indexMu.Lock()`, (2) calls `indexVault(vc)` with error logging on failure (no return — per D5/FR-008), (3) calls `LoadExternalPages` + `BuildBacklinks` (existing code moved here), (4) calls `vc.Watch()` with error logging on failure, (5) sets `indexReady.Store(true)`, (6) calls `indexMu.Unlock()`. Import `sync` and `sync/atomic`.

- [x] T005 [US2] Add structured diagnostics logging for background indexing in `main.go`. Log `"background indexing started"` immediately when the goroutine begins. Log `"background indexing complete"` with `elapsed` duration when all operations finish. Log `"background indexing failed"` with `err` if `indexVault()` returns an error (per D6/FR-007). Ensure the "server ready" log line (already existing) appears BEFORE the background indexing log lines (per FR-010).

**Checkpoint**: `go build ./...` passes. `dewey serve --vault <path> --no-embeddings` starts in <2 seconds and logs "server ready" before "background indexing started". Manual verification: `dewey serve` on a large vault shows sub-second startup.

---

## Phase 3: Tests

**Purpose**: Update existing tests and add new tests for the restructured code. All tests must pass with `go test -race -count=1 ./...`.

- [x] T006 [P] [US1] Update `main_test.go` startup tests. The `TestInitObsidianBackend_*` tests call `initObsidianBackend()` which no longer performs indexing. Update assertions: (1) `TestInitObsidianBackend_WithMarkdownFiles` — after `initObsidianBackend()`, pages are NOT yet indexed (in-memory index is empty). Call `indexVault(vc)` explicitly in the test before asserting `GetAllPages`. (2) Verify `initObsidianBackend()` returns in <1 second (no indexing delay). (3) If `NewIndexing` signature changed, update any test calls.

- [x] T007 [P] [US3] Update `tools/indexing_test.go` for external mutex. Add test `TestIndexing_ExternalMutex_SharedLock`: create a `sync.Mutex`, lock it externally, pass to `NewIndexing(s, nil, dir, mu)`, call `Index()` — verify "already in progress" error. Add test `TestIndexing_ExternalMutex_NilFallback`: pass `nil` mutex to `NewIndexing`, verify `Index()` and `Reindex()` still use internal mutual exclusion (lock `ix.mu` directly, call the other — should get "already in progress"). Update existing `TestIndexing_Index_ConcurrentCallRejected` and `TestIndexing_Reindex_ConcurrentCallRejected` to use the new `NewIndexing` signature (pass `nil` for backward compat).

- [x] T008 [US1] Add background indexing integration test in `main_test.go`. Test `TestBackgroundIndexing_ServerStartsBeforeIndexing`: create a temp vault with markdown files, call `initObsidianBackend()` (fast path), verify backend is non-nil, create `indexReady := &atomic.Bool{}` and `indexMu := &sync.Mutex{}`, launch background goroutine (same pattern as `executeServe`), verify `indexReady.Load() == false` immediately, wait for goroutine to complete, verify `indexReady.Load() == true`, verify pages are now in the in-memory index via `GetAllPages()`.

**Checkpoint**: `go test -race -count=1 ./...` passes. All existing tests pass. New tests cover the restructured startup sequence, external mutex injection, and background indexing completion.

---

## Phase 4: Verification

**Purpose**: CI parity gate — replicate CI checks locally before declaring implementation complete.

- [x] T009 [US1] CI parity gate. Read `.github/workflows/ci.yml` to identify exact CI commands. Run locally: (1) `go build ./...`, (2) `go vet ./...`, (3) `go test -race -count=1 ./...`, (4) `go test -race -count=1 -coverprofile=coverage.out ./...` followed by `gaze report ./... --coverprofile=coverage.out --max-crapload=48 --max-gaze-crapload=18 --min-contract-coverage=70` (if Gaze is installed). All must pass. Fix any failures before marking complete.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Infrastructure)**: No dependencies — can start immediately
  - T001 and T002 are independent (`server.go` vs `tools/indexing.go`) and can run in parallel
- **Phase 2 (Core Restructure)**: Depends on Phase 1 completion — T003/T004/T005 use the options and mutex from T001/T002
  - T003 must complete before T004 (T004 uses the split `initObsidianBackend`)
  - T005 can run alongside T004 (logging is additive) but is simpler to do sequentially
- **Phase 3 (Tests)**: Depends on Phase 2 completion — tests validate the restructured code
  - T006 and T007 are independent (`main_test.go` vs `tools/indexing_test.go`) and can run in parallel
  - T008 depends on T003/T004 being complete (tests the background goroutine pattern)
- **Phase 4 (Verification)**: Depends on Phase 3 completion — all tests must pass first

### Parallel Opportunities

```
Phase 1: T001 ──┐
                 ├── Phase 2: T003 → T004 → T005
         T002 ──┘
                        │
                        ▼
Phase 3: T006 ──┐
                 ├── Phase 4: T009
         T007 ──┤
         T008 ──┘
```

### Files Modified Per Task

| Task | Files | Story |
|------|-------|-------|
| T001 | `server.go` | US2 |
| T002 | `tools/indexing.go`, `server.go` | US3 |
| T003 | `main.go` | US1 |
| T004 | `main.go` | US1 |
| T005 | `main.go` | US1 |
| T006 | `main_test.go` | US1 |
| T007 | `tools/indexing_test.go` | US3 |
| T008 | `main_test.go` | US1 |
| T009 | (verification only) | US1 |

---

## Implementation Strategy

### Sequential Execution (Single Developer)

1. Complete T001 + T002 (parallel-safe, different files) → `go build ./...`
2. Complete T003 → T004 → T005 (sequential, all in `main.go`) → manual smoke test
3. Complete T006 + T007 (parallel-safe, different test files) + T008 → `go test -race -count=1 ./...`
4. Complete T009 → CI parity verified

### Story Traceability

- **US1 (Instant MCP Server Startup)**: T003, T004, T005, T006, T008, T009
- **US2 (Indexing Status Visibility)**: T001, T005
- **US3 (Mutual Exclusion with Live Reindex)**: T002, T007
<!-- spec-review: passed -->
