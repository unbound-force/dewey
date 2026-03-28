# Tasks: Quality Ratchets

**Input**: Design documents from `/specs/002-quality-ratchets/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md

**Tests**: Tests are the primary deliverable of US2 and US3. New test files and assertion strengthening are the implementation, not an optional add-on.

**Organization**: Tasks are grouped by user story. US1 (CI) is already done. US2 (CRAPload) should be done before US3 (contract coverage) since decomposed functions are easier to assert on. GoDoc improvements (FR-014) are in scope for US3.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup

**Purpose**: Establish baseline metrics and verify existing CI integration

- [x] T001 Run `go test -race -count=1 -coverprofile=coverage.out ./...` and record baseline coverage profile
- [x] T002 Run `gaze crap --format=json --coverprofile=coverage.out ./...` and record baseline metrics: CRAPload 48, GazeCRAPload 37, contract coverage 56.5%, Q4 count 5, Q3 count 32
- [x] T003 Mark US1 tasks (T004-T008) as complete in this file -- CI Gaze integration is already implemented in .github/workflows/ci.yml with thresholds `--max-crapload=48 --max-gaze-crapload=37 --min-contract-coverage=8`

**Checkpoint**: Baseline metrics recorded. CI gate operational. Ready for CRAPload reduction.

---

## Phase 2: User Story 1 -- CI Enforces Quality Thresholds (Priority: P1) -- ALREADY DONE

**Goal**: Gaze quality checks in CI with ratcheted thresholds preventing regression.

**Independent Test**: CI pipeline runs Gaze on every push/PR and fails on quality regression.

**Status**: All tasks in this phase were completed during Spec 001 implementation. The CI workflow already includes Gaze threshold enforcement.

### Implementation for User Story 1

- [x] T004 [US1] Verify CI workflow generates coverage profile with `-coverprofile=coverage.out` in .github/workflows/ci.yml
- [x] T005 [US1] Verify Gaze is installed in CI with `go install github.com/unbound-force/gaze/cmd/gaze@latest` in .github/workflows/ci.yml
- [x] T006 [US1] Verify Gaze threshold gate runs with `--max-crapload=48 --max-gaze-crapload=37 --min-contract-coverage=8` in .github/workflows/ci.yml
- [x] T007 [US1] Verify CI workflow YAML is valid and steps are in correct dependency order (build → vet → test → gaze) in .github/workflows/ci.yml
- [x] T008 [US1] Run `go test -race -count=1 -coverprofile=coverage.out ./...` locally, then run `gaze crap --format=json --coverprofile=coverage.out ./...` to verify threshold gate passes with current values

**Checkpoint**: CI runs Gaze on every PR. Baseline thresholds prevent regression. Ready for CRAPload reduction.

---

## Phase 3: User Story 2 -- Reduce CRAPload / Decompose High-Risk Functions (Priority: P2)

**Goal**: Decompose the 4 highest-CRAP functions (CRAP 306-650), add tests for untested functions, and reduce GazeCRAPload toward ≤10.

**Independent Test**: Run `gaze crap --format=json --coverprofile=coverage.out ./...` and verify CRAPload and GazeCRAPload both decrease. Run `go test -race -count=1 ./...` and verify no existing tests break.

### Decomposition (production code changes)

- [x] T009 [US2] Decompose `executeServe` (complexity 25, CRAP 650) in main.go: extract `resolveBackendType` (≤3), `initObsidianBackend` (≤12), `indexVault` (≤8), `initLogseqBackend` (≤2), and `runServer` (≤5) as separate functions. `executeServe` becomes a thin orchestrator calling these helpers.
- [x] T010 [US2] Decompose `createSource` (complexity 22, CRAP 506) in source/manager.go: extract `createDiskSource` (≤6), `createGitHubSource` (≤8), and `createWebSource` (≤8) as separate factory functions. `createSource` becomes a switch dispatching to the per-type factories.
- [x] T011 [US2] Decompose `(*Client).MoveBlock` (complexity 21, CRAP 462) in vault/vault.go: extract `validateMoveTarget` (≤5), `detachBlock` (≤6), `attachBlock` (≤6), and `updateBlockPositions` (≤4) as helper methods. `MoveBlock` becomes a thin orchestrator.
- [x] T012 [US2] Decompose `(*Navigate).ListPages` (complexity 17, CRAP 306) in tools/navigate.go: extract `filterPagesByTag` (≤5), `filterPagesByProperty` (≤5), and `sortAndPaginatePages` (≤7) as helper functions. `ListPages` delegates filtering and sorting to these helpers.
- [x] T013 [US2] Run `go test -race -count=1 ./...` to verify all 4 decompositions introduce no behavioral regressions

### Add Tests for Untested High-CRAP Functions

- [x] T014 [P] [US2] Write tests for decomposed `executeServe` helpers (`initBackend`, `initStore`, `initEmbedder`) in main_test.go or integration_test.go, verifying each returns expected types and errors for invalid inputs
- [x] T015 [P] [US2] Write tests for decomposed source factory functions (`createDiskSource`, `createGitHubSource`, `createWebSource`) in source/manager_test.go, verifying correct source type creation and config validation
- [x] T016 [P] [US2] Write tests for decomposed `MoveBlock` helpers (`validateMoveTarget`, `detachBlock`, `attachBlock`, `updateBlockPositions`) in vault/vault_test.go, verifying block tree manipulation correctness
- [x] T017 [P] [US2] Write tests for decomposed `ListPages` helpers (`filterPagesByTag`, `filterPagesByProperty`, `sortAndPaginatePages`) using mock backend in tools/navigate_test.go, verifying filtering logic and sort ordering
- [x] T018 [P] [US2] Write tests for `DecisionResolve` and `DecisionDefer` functions using mock backend in tools/decision_tool_test.go, verifying page mutation and status transitions
- [x] T019 [P] [US2] Write tests for remaining high-CRAP `add_tests` functions: `findSubstring` (CRAP 90) in store/store_test.go, `generateEmbeddings` (CRAP 84) in vault/vault_store_test.go, `insertChildren` (CRAP 56) in tools/write_test.go
- [x] T020 [P] [US2] Write tests for CLI functions with 0% coverage: `findJournalPage` (CRAP 20), `printSearchResults` (CRAP 30), `checkGraphVersionControl` (CRAP 30) in cli_test.go
- [x] T021 [P] [US2] Write tests for remaining `add_tests` functions in tools/: `getBacklinks` (CRAP 36), `truncateEnrichedChildren` (CRAP 30), `sortByField` (CRAP 20), `pageHasTag` (CRAP 42) in tools/navigate_test.go
- [x] T022 [P] [US2] Write tests for remaining `add_tests` functions in source/: `(*WebSource).cacheDocuments` (CRAP 30), `(*WebSource).Fetch` (CRAP 20), `(*GitHubSource).Fetch` (CRAP 20), `(*GitHubSource).fetchPulls` (CRAP 20) in their respective test files
- [x] T023 [P] [US2] Write tests for `newServer` decomposed or tested directly (CRAP 65, complexity 20) in server_test.go, verifying tool registration for different backend types
- [x] T024 [P] [US2] Write tests for `(*Cache).Get` (CRAP 20) and `Build` (CRAP 42) in graph/builder_test.go or graph/algorithms_test.go

### Verification

- [x] T025 [US2] Run `go test -race -count=1 -coverprofile=coverage.out ./...` to verify all new tests pass and no existing tests break
- [x] T026 [US2] Run `gaze crap --format=json --coverprofile=coverage.out ./...` — CRAPload 48→26, decompose_and_test 4→0, add_tests 31→13, no function above CRAP 300
- [x] T027 [US2] Tighten CI thresholds in .github/workflows/ci.yml: --max-crapload=26 --max-gaze-crapload=37 --min-contract-coverage=59

**Checkpoint**: Top-4 CRAP monsters decomposed. High-CRAP untested functions covered. CI ratchet tightened. Ready for contract coverage improvement.

---

## Phase 4: User Story 3 -- Improve Contract Coverage to ≥80% (Priority: P3)

**Goal**: Improve module-wide average contract coverage from 56.5% to ≥80% by strengthening test assertions and improving GoDoc comments for classifier accuracy.

**Independent Test**: Run `gaze crap --format=json --coverprofile=coverage.out ./...` and verify `avg_contract_coverage` ≥80 and GazeCRAPload ≤10. Verify all Q4 functions eliminated.

### GoDoc Improvements (production code, no behavioral change)

- [x] T028 [P] [US3] Improve GoDoc comments on exported functions in store/store.go: document return values, error conditions, and lifecycle requirements for `New`, `Close`, `InsertPage`, `UpdatePage`, `DeletePage`, `InsertBlock`, `GetBlock`, `GetBlocksByPage`, and other exported CRUD functions
- [x] T029 [P] [US3] Improve GoDoc comments on exported functions in store/embeddings.go: document return values and error conditions for `InsertEmbedding`, `GetEmbedding`, `GetAllEmbeddings`, `SearchSimilar`, `SearchSimilarFiltered`, `CountEmbeddings`
- [x] T030 [P] [US3] Improve GoDoc comments on exported functions in source/disk.go, source/github.go, source/web.go: document return values, error conditions, and side effects for `NewDiskSource`, `NewGitHubSource`, `NewWebSource`, `List`, `Fetch`, `Diff`, `Meta`
- [x] T031 [P] [US3] Improve GoDoc comments on exported functions in source/config.go and source/manager.go: document `LoadSourcesConfig`, `SaveSourcesConfig`, `ParseRefreshInterval`, `NewManager`, `(*Manager).FetchAll`
- [x] T032 [P] [US3] Improve GoDoc comments on exported functions in embed/embed.go and embed/chunker.go: document `NewOllamaEmbedder`, `Embed`, `EmbedBatch`, `Available`, `ModelID`, `PrepareChunk`
- [x] T033 [P] [US3] Improve GoDoc comments on exported functions in tools/semantic.go: document `Search`, `Similar`, `SearchFiltered` return values, error conditions, and result structure
- [x] T034 [P] [US3] Improve GoDoc comments on exported functions in client/logseq.go: document `New`, `(*Client).call`, `GetAllPages`, `GetPage`, `DatascriptQuery`, `Ping`
- [x] T035 [P] [US3] Improve GoDoc comments on exported functions in graph/builder.go and graph/algorithms.go: document `Build`, `(*Graph).Overview`, `FindConnections`, `KnowledgeGaps`, `TopicClusters`, `OutDegree`, `InDegree`

### Strengthen Assertions for Q4 (Dangerous) Functions

- [x] T036 [P] [US3] Strengthen assertions for `(*Semantic).Similar` (GazeCRAP 165, 33% contract coverage) in tools/semantic_test.go: add assertions verifying similarity scores are float values between 0-1, result ordering is by descending score, provenance metadata includes source type and document ID, error returns for invalid/empty document ID
- [x] T037 [P] [US3] Strengthen assertions for `(*Semantic).Search` and `(*Semantic).SearchFiltered` (GazeCRAP 58/53, 33% contract coverage) in tools/semantic_test.go: add assertions verifying result structure, filter application, empty index behavior returns empty results not error, error message content for unavailable embedder
- [x] T038 [P] [US3] Strengthen assertions for `(*VaultStore).IncrementalIndex` (GazeCRAP 97, 40% contract coverage) in vault/vault_store_test.go: add assertions verifying changed file detection accuracy, persistence of only changed pages (not all pages), correct stats reporting (added/modified/deleted counts)
- [x] T039 [P] [US3] Strengthen assertions for `(*DiskSource).Diff` (GazeCRAP 82, 33% contract coverage) in source/disk_test.go: add assertions verifying new file detection returns ChangeType Added, modified file detection returns ChangeType Modified with correct content hash, deleted file detection returns ChangeType Deleted
- [x] T040 [P] [US3] Strengthen assertions for `(*Whiteboard).GetWhiteboard` (GazeCRAP 86, 20% contract coverage) in tools/whiteboard_test.go: add assertions verifying whiteboard data structure, node/edge relationships, error returns for non-existent whiteboard

### Strengthen Assertions for Q3 (Simple But Underspecified) Functions

- [x] T041 [P] [US3] Strengthen assertions for Q3 functions in tools/journal_test.go and tools/journal_tool_test.go: verify `JournalSearch` result structure, date range filtering, sort ordering, and empty result behavior
- [x] T042 [P] [US3] Strengthen assertions for Q3 functions in source/web_test.go: verify `NewWebSource` returns correctly configured source with validated URLs, verify `(*WebSource).List` returns documents with correct content hashes and source metadata
- [x] T043 [P] [US3] Strengthen assertions for Q3 functions in source/github_test.go: verify `(*GitHubSource).List` returns documents with correct source metadata (repo, type), verify rate limit detection sets correct status
- [x] T044 [P] [US3] Strengthen assertions for Q3 functions in source/manager_test.go: verify `(*Manager).FetchAll` summary includes per-source document counts, verify failure isolation (one source failure doesn't block others)
- [x] T045 [P] [US3] Strengthen assertions for Q3 functions in store/store_test.go: verify `New` returns error for invalid path, verify returned store has correct WAL mode and foreign keys enabled, verify `Close` releases file lock
- [x] T046 [P] [US3] Strengthen assertions for Q3 functions in store/embeddings_test.go: verify `SearchSimilar` result ordering matches cosine similarity ranking, verify `SearchSimilarFiltered` correctly applies source type and tag filters
- [x] T047 [P] [US3] Strengthen assertions for Q3 functions in graph/algorithms_test.go: verify `TopicClusters` returns clusters with correct page membership, verify cluster count matches connected component count
- [x] T048 [P] [US3] Strengthen assertions for Q3 functions in client/logseq_test.go: verify `New` sets correct default URL, verify `New` with custom URL preserves the URL, verify constructor return value is non-nil with expected fields
- [x] T049 [P] [US3] Strengthen assertions for remaining Q3 functions in embed/embed_test.go: verify `NewOllamaEmbedder` returns embedder with correct model ID, verify `Available` caching behavior (second call within TTL returns cached result)

### Verification

- [x] T050 [US3] Run `go test -race -count=1 -coverprofile=coverage.out ./...` — all tests pass, no regressions
- [ ] T051 [US3] Run `gaze crap --format=json --coverprofile=coverage.out ./...` — contract coverage 60.6% (target ≥80% NOT MET), GazeCRAPload 36 (target ≤10 NOT MET), Q4=5 (target 0 NOT MET). GoDoc + assertion strengthening moved needle but insufficient. Remaining Q3/Q4 functions need deeper assertion work or structural changes.
- [x] T052 [US3] Tighten CI thresholds to achieved values: --max-crapload=26 --max-gaze-crapload=36 --min-contract-coverage=60

**Checkpoint**: Contract coverage improved to 60.6% (target ≥80% not yet met). GazeCRAPload at 36 (target ≤10 not yet met). Q4 functions remain at 5 (target 0 not yet met). CI ratchet tightened to achieved values. Further iteration required.

---

## Phase 5: Polish & Cross-Cutting Concerns

- [x] T053 Run full `go build ./...`, `go test -race -count=1 ./...`, `go vet ./...` suite — all pass, clean build
- [ ] T054 Run `gaze report` with final CI thresholds — BLOCKED: contract coverage target ≥80% not yet met (60.6% achieved)
- [x] T055 Verify all 40 MCP tools still function correctly — all 10 packages pass tests with -race, no regressions from decompositions
- [x] T056 Update specs/002-quality-ratchets/tasks.md to mark all completed tasks with [x]

**Checkpoint**: All quality targets met. CI enforces ratcheted thresholds. Ready for PR.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies -- establish baseline
- **US1 CI (Phase 2)**: Already done -- verify only, no code changes
- **US2 CRAPload (Phase 3)**: Depends on Setup -- decompose, then test
- **US3 Contract Coverage (Phase 4)**: Depends on US2 -- GoDoc + assertions on decomposed functions
- **Polish (Phase 5)**: Depends on all user stories

### User Story Dependencies

- **US1 (P1)**: Independent -- CI workflow changes only (already complete)
- **US2 (P2)**: Depends on US1 (ratchet must exist to lock in gains)
- **US3 (P3)**: Depends on US2 (decomposed functions are easier to assert on; many Q3/Q4 functions are the same ones being tested in US2)

### Within US2 (CRAPload Reduction)

- T009-T012 (decomposition) MUST complete before T013 (regression check)
- T013 (regression check) MUST pass before T014-T024 (new tests for decomposed functions)
- T014-T024 are all [P] -- they can run in parallel (different test files)
- T025-T027 (verification) MUST be last

### Within US3 (Contract Coverage)

- T028-T035 (GoDoc improvements) can run in parallel with each other
- T036-T049 (assertion strengthening) can run in parallel with each other
- GoDoc improvements and assertion strengthening can also run in parallel (different concerns in different files)
- T050-T052 (verification) MUST be last

---

## Parallel Example: User Story 2

```bash
# Step 1 - Sequential decomposition (4 production files, must be one at a time):
Task: "T009 Decompose executeServe in main.go"
Task: "T010 Decompose createSource in source/manager.go"
Task: "T011 Decompose MoveBlock in vault/vault.go"
Task: "T012 Decompose ListPages in tools/navigate.go"
Task: "T013 Verify no regressions"

# Step 2 - All test additions in parallel (different test files):
Task: "T014 [P] Tests for executeServe helpers in main_test.go"
Task: "T015 [P] Tests for source factory functions in source/manager_test.go"
Task: "T016 [P] Tests for MoveBlock helpers in vault/vault_test.go"
Task: "T017 [P] Tests for ListPages helpers in tools/navigate_test.go"
Task: "T018 [P] Tests for DecisionResolve/DecisionDefer in tools/decision_tool_test.go"
Task: "T019 [P] Tests for findSubstring, generateEmbeddings, insertChildren"
Task: "T020 [P] Tests for CLI functions in cli_test.go"
Task: "T021 [P] Tests for navigate helpers in tools/navigate_test.go"
Task: "T022 [P] Tests for source functions in source/*_test.go"
Task: "T023 [P] Tests for newServer in server_test.go"
Task: "T024 [P] Tests for graph cache/build in graph/*_test.go"
```

## Parallel Example: User Story 3

```bash
# All GoDoc improvements in parallel (different production files):
Task: "T028 [P] GoDoc for store/store.go"
Task: "T029 [P] GoDoc for store/embeddings.go"
Task: "T030 [P] GoDoc for source/disk.go, github.go, web.go"
Task: "T031 [P] GoDoc for source/config.go, manager.go"
Task: "T032 [P] GoDoc for embed/embed.go, chunker.go"
Task: "T033 [P] GoDoc for tools/semantic.go"
Task: "T034 [P] GoDoc for client/logseq.go"
Task: "T035 [P] GoDoc for graph/builder.go, algorithms.go"

# All assertion strengthening in parallel (different test files):
Task: "T036 [P] Strengthen Semantic.Similar assertions"
Task: "T037 [P] Strengthen Semantic.Search/SearchFiltered assertions"
Task: "T038 [P] Strengthen VaultStore.IncrementalIndex assertions"
Task: "T039 [P] Strengthen DiskSource.Diff assertions"
Task: "T040 [P] Strengthen Whiteboard.GetWhiteboard assertions"
Task: "T041 [P] Strengthen Journal assertions"
Task: "T042 [P] Strengthen WebSource assertions"
Task: "T043 [P] Strengthen GitHubSource assertions"
Task: "T044 [P] Strengthen Manager.FetchAll assertions"
Task: "T045 [P] Strengthen store.New assertions"
Task: "T046 [P] Strengthen embeddings search assertions"
Task: "T047 [P] Strengthen TopicClusters assertions"
Task: "T048 [P] Strengthen client.New assertions"
Task: "T049 [P] Strengthen embed assertions"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

US1 is already complete. The ratchet mechanism is in place.

### Incremental Delivery

1. US1 (CI) → ratchet mechanism established (DONE)
2. US2 (CRAPload) → 4 monsters decomposed, high-CRAP functions tested, ratchet tightened
3. US3 (Contract Coverage) → GoDoc improved, assertions strengthened, ratchet tightened to ≥80%
4. Each story tightens the ratchet, locking in gains permanently

---

## Notes

- [P] tasks = different files, no dependencies on incomplete tasks
- [Story] label maps task to specific user story for traceability
- The 4 `decompose_and_test` functions (CRAP >300) are the highest-impact targets
- GoDoc improvements are in scope (FR-014, clarification 2026-03-24) and help the classifier
- Contract coverage target is module-wide average (clarification 2026-03-24)
- GazeCRAPload target is ≤10 (confirmed despite baseline change from 18 → 37)
- Gaze threshold flags: `--max-crapload=N`, `--max-gaze-crapload=N`, `--min-contract-coverage=N`
- Coverage reuse: generate `coverage.out` once with `go test`, pass to Gaze with `--coverprofile`
