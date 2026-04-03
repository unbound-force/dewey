# Tasks: Store Learning MCP Tool

**Input**: Design documents from `/specs/008-store-learning/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, quickstart.md

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup (Input Type)

**Purpose**: Define the MCP tool input struct that all subsequent phases depend on

- [x] T001 [US1] Add `StoreLearningInput` struct to `types/tools.go` after the `SemanticSearchFilteredInput` section (line ~231), before the Journal section. Fields: `Information string` (required, `json:"information"`) and `Tags string` (optional, `json:"tags,omitempty"`). Include GoDoc comment and `jsonschema` tags per existing conventions. Maps to FR-001.

**Checkpoint**: `go build ./...` passes — new type is valid Go

---

## Phase 2: Core Implementation (Tool Handler)

**Purpose**: Create the `Learning` struct and `StoreLearning` handler — the core business logic

- [x] T002 [US1] Create `tools/learning.go` with package declaration, imports, `Learning` struct (fields: `embedder embed.Embedder`, `store *store.Store`), and `NewLearning(e embed.Embedder, s *store.Store) *Learning` constructor. Follow the `tools/semantic.go` pattern (Dependency Inversion Principle). Include GoDoc comments on struct and constructor.
- [x] T003 [US1] Implement `StoreLearning` handler method on `Learning` struct in `tools/learning.go`. Signature: `func (l *Learning) StoreLearning(ctx context.Context, req *mcp.CallToolRequest, input types.StoreLearningInput) (*mcp.CallToolResult, any, error)`. Implement: (1) validate `input.Information` is non-empty (FR-007), (2) validate `l.store` is non-nil (FR-007), (3) generate unique page name `learning/{unixMillis}` and doc ID `learning-{unixMillis}` (R8), (4) build properties JSON with tags if provided (FR-004, R3), (5) call `store.InsertPage` with `SourceID: "learning"` (FR-003, R2), (6) call `vault.ParseDocument` + `vault.PersistBlocks` (FR-002, R6), (7) call `vault.GenerateEmbeddings` if embedder available, else set informational message (FR-005, FR-009), (8) return JSON with UUID, page name, and message (FR-006). Use `errorResult()` and `jsonTextResult()` helpers from `tools/helpers.go`.

**Checkpoint**: `go build ./...` passes — handler compiles with all dependencies resolved

---

## Phase 3: Registration (Wire into Server)

**Purpose**: Register the new tool in the MCP server, gated by `!readOnly`

- [x] T004 [US1] Add `registerLearningTools(srv *mcp.Server, learning *tools.Learning)` function to `server.go`. Register `dewey_store_learning` tool using the typed handler pattern (`mcp.AddTool`). Tool description: "Store a learning (insight, pattern, gotcha) with optional tags. The learning is persisted with embeddings and immediately searchable via dewey_semantic_search. Use to build semantic memory across sessions." Maps to FR-010.
- [x] T005 [US1] Add learning tool instantiation and registration call in `newServer()` in `server.go`. Place inside the existing `if !readOnly` block (after write/decision tools) or in a new `if !readOnly` block after `registerSemanticTools`. Create `learning := tools.NewLearning(cfg.embedder, cfg.store)` and call `registerLearningTools(srv, learning)`. Maps to FR-010.

**Checkpoint**: `go build ./...` passes — tool #41 is registered. `go vet ./...` clean.

---

## Phase 4: Tests

**Purpose**: Contract-level tests covering happy path, error paths, and graceful degradation

- [x] T006 [P] [US1] Create `tools/learning_test.go` with `TestStoreLearning_Basic`: create in-memory store, mock embedder, call `StoreLearning` with valid information text. Assert: no error, result contains UUID (non-empty), result contains page name matching `learning/` prefix, result is not an error result. Validates FR-001, FR-002, FR-006.
- [x] T007 [P] [US1] Add `TestStoreLearning_EmptyInformation` to `tools/learning_test.go`: call `StoreLearning` with empty `Information` field. Assert: result `IsError` is true, error text contains "information parameter is required". Validates FR-007.
- [x] T008 [P] [US1] Add `TestStoreLearning_NilStore` to `tools/learning_test.go`: create `Learning` with nil store. Call `StoreLearning` with valid input. Assert: result `IsError` is true, error text mentions persistent storage. Validates FR-007.
- [x] T009 [P] [US1] Add `TestStoreLearning_WithTags` to `tools/learning_test.go`: call `StoreLearning` with `Tags: "gotcha, vault-walker"`. Assert: stored page properties contain `"tags"` key with the tag values. Query store to verify page properties JSON. Validates FR-004.
- [x] T010 [P] [US1] Add `TestStoreLearning_EmbedderUnavailable` to `tools/learning_test.go`: create `Learning` with a mock embedder whose `Available()` returns false. Call `StoreLearning`. Assert: result is NOT an error (learning is stored), result message contains informational text about embeddings not generated. Validates FR-009.
- [x] T011 [P] [US1] Add `TestStoreLearning_NilEmbedder` to `tools/learning_test.go`: create `Learning` with nil embedder. Call `StoreLearning`. Assert: result is NOT an error (learning is stored), result message contains informational text about embeddings. Validates FR-009.
- [x] T012 [US1] Add `TestStoreLearning_Searchable` to `tools/learning_test.go`: store a learning about "scaffold patterns in Go templates", then call `store.SearchSimilar` (or verify page exists via `store.GetPageByName`). Assert: the stored learning page exists in the store with correct source_id "learning" and content block. Validates FR-002, SC-001.
- [x] T013 [US3] Add `TestStoreLearning_FilterBySourceType` to `tools/learning_test.go`: store a learning, then call `store.SearchSimilarFiltered` with `SourceType: "learning"`. Assert: the learning appears in filtered results. If embedder mock doesn't support full vector search, verify via `store.GetPageByName` that `source_id` is "learning" and `inferSourceType` would return "learning". Validates FR-003, SC-004.

**Checkpoint**: `go test -race -count=1 ./tools/...` — all 8 tests pass

---

## Phase 5: Verification (CI Parity Gate)

**Purpose**: Full CI-equivalent validation before declaring implementation complete

- [x] T014 Run CI parity gate: execute `go build ./...`, `go vet ./...`, and `go test -race -count=1 ./...` from repo root. All must pass. Fix any failures before proceeding. Validates SC-005 (existing 40 tools unaffected).
- [x] T015 Validate documentation: verify GoDoc comments exist on `StoreLearningInput`, `Learning` struct, `NewLearning`, `StoreLearning` method, and `registerLearningTools`. Update `AGENTS.md` if needed (tool count reference). Assess README.md impact.

**Checkpoint**: All CI checks pass. Implementation is complete and ready for review.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies — start immediately
- **Phase 2 (Core)**: Depends on Phase 1 (uses `StoreLearningInput` type)
- **Phase 3 (Registration)**: Depends on Phase 2 (uses `Learning` struct and `NewLearning`)
- **Phase 4 (Tests)**: Depends on Phase 2 (tests the handler). T006-T011 are parallel (different test functions, same file). T012-T013 depend on T006 pattern but can be written in any order.
- **Phase 5 (Verification)**: Depends on all previous phases

### Parallel Opportunities

```
Phase 4 parallel batch:
  T006: TestStoreLearning_Basic
  T007: TestStoreLearning_EmptyInformation
  T008: TestStoreLearning_NilStore
  T009: TestStoreLearning_WithTags
  T010: TestStoreLearning_EmbedderUnavailable
  T011: TestStoreLearning_NilEmbedder
```

All six tests touch the same file (`tools/learning_test.go`) but are independent test functions. When implemented by a single agent, write them sequentially in one pass. The `[P]` marker indicates they have no logical dependencies on each other.

### User Story Coverage

| Story | Tasks | Coverage |
|-------|-------|----------|
| US1 — Store and Retrieve | T001-T012 | FR-001 through FR-010, SC-001, SC-005 |
| US2 — Persist Across Sessions | (inherent) | FR-008 — SQLite persistence is automatic; no task needed |
| US3 — Coexist with Other Content | T013 | FR-003, SC-003, SC-004 |

US2 requires no dedicated implementation task because learnings use `source_id = "learning"` which is not a configured source — `dewey reindex` never deletes them (R7). Persistence is inherent to the SQLite store.
<!-- spec-review: passed -->
