# Tasks: Quality Ratchets

**Input**: Design documents from `/specs/002-quality-ratchets/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md

**Tests**: Tests are the primary deliverable of US2 and US3. New test files and assertion strengthening are the implementation, not an optional add-on.

**Organization**: Tasks are grouped by user story. US1 (CI) is independent. US2 (CRAPload) should be done before US3 (contract coverage) since decomposed functions are easier to assert on.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup

**Purpose**: Establish baseline metrics and install Gaze tooling

- [ ] T001 Run `go test -race -count=1 -coverprofile=coverage.out ./...` and record baseline coverage profile
- [ ] T002 Run `gaze report ./... --format=json --coverprofile=coverage.out > /dev/null` to verify Gaze runs cleanly on the codebase and record baseline metrics (CRAPload, GazeCRAPload, contract coverage)

**Checkpoint**: Baseline metrics recorded. Gaze runs cleanly. Ready for CI integration.

---

## Phase 2: User Story 1 -- CI Enforces Quality Thresholds (Priority: P1) MVP

**Goal**: Add Gaze quality checks to the CI pipeline with ratcheted thresholds. CI fails on quality regression.

**Independent Test**: Push a PR that adds an untested complex function. Verify CI fails with an actionable error message.

### Implementation for User Story 1

- [ ] T003 [US1] Update CI workflow to generate coverage profile: change `go test -race -count=1 ./...` to `go test -race -count=1 -coverprofile=coverage.out ./...` in .github/workflows/ci.yml
- [ ] T004 [US1] Add Gaze install step to CI workflow: `go install github.com/unbound-force/gaze/cmd/gaze@v1.4.6` after the Go setup step in .github/workflows/ci.yml
- [ ] T005 [US1] Add Gaze threshold gate step to CI workflow (runs on all PRs): `gaze report ./... --format=json --coverprofile=coverage.out --max-crapload=88 --max-gaze-crapload=18 --min-contract-coverage=70 > /dev/null` in .github/workflows/ci.yml
- [ ] T006 [US1] Verify CI workflow is valid YAML and that `go build`, `go test`, and `gaze report` steps are in correct dependency order in .github/workflows/ci.yml
- [ ] T007 [US1] Run `go test -race -count=1 -coverprofile=coverage.out ./...` locally, then run `gaze report ./... --format=json --coverprofile=coverage.out --max-crapload=88 --max-gaze-crapload=18 --min-contract-coverage=70 > /dev/null` to verify the threshold gate passes with baseline values

**Checkpoint**: CI runs Gaze on every PR. Baseline thresholds prevent regression. Ready for CRAPload reduction.

---

## Phase 3: User Story 2 -- Reduce CRAPload to B Grade (Priority: P2)

**Goal**: Reduce CRAPload from 88 (24.8%, grade D) to ≤53 (≤15%, grade B) by decomposing high-complexity functions and adding tests for untested functions.

**Independent Test**: Run `gaze crap ./... --format=json --coverprofile=coverage.out` and verify CRAPload ≤53.

### Decomposition (production code changes)

- [ ] T008 [US2] Decompose `executeServe` (complexity 25, CRAP 650) in main.go: extract backend initialization, store initialization, embedder initialization, and MCP server startup into separate functions with complexity ≤15 each
- [ ] T009 [US2] Decompose `createSource` (complexity 22, CRAP 506) in source/manager.go: extract per-type source factory functions (createDiskSource, createGitHubSource, createWebSource) with complexity ≤10 each
- [ ] T010 [US2] Run `go test -race -count=1 ./...` to verify decompositions introduce no behavioral regressions

### Shared Test Infrastructure

- [ ] T011 [US2] Create shared mock backend for tool tests: implement `mockBackend` struct satisfying `backend.Backend` interface with configurable page/block/link data in tools/mock_backend_test.go

### Add Tests for tools/ Package (highest CRAP scores)

- [ ] T012 [P] [US2] Write tests for Decision tool functions (`DecisionCheck`, `DecisionCreate`, `DecisionResolve`, `DecisionDefer`, `AnalysisHealth`) using mock backend in tools/decision_test.go
- [ ] T013 [P] [US2] Write tests for Journal tool functions (`JournalRange`, `JournalSearch`) using mock backend in tools/journal_test.go
- [ ] T014 [P] [US2] Write tests for Search tool functions (`Search`, `QueryProperties`, `QueryDatalog`, `FindByTag`) using mock backend in tools/search_test.go
- [ ] T015 [P] [US2] Write tests for Analyze tool functions (`GraphOverview`, `FindConnections`, `KnowledgeGaps`, `ListOrphans`, `TopicClusters`) using mock backend in tools/analyze_test.go
- [ ] T016 [P] [US2] Write tests for Write tool functions (`CreatePage`, `AppendBlocks`, `UpsertBlocks`, `UpdateBlock`, `DeleteBlock`, `MoveBlock`, `LinkPages`, `DeletePage`, `RenamePage`, `BulkUpdateProperties`) using mock backend in tools/write_test.go
- [ ] T017 [P] [US2] Write tests for Navigate tool functions (`GetPage`, `GetBlock`, `ListPages`, `GetLinks`, `GetReferences`, `Traverse`) using mock backend in tools/navigate_test.go
- [ ] T018 [P] [US2] Write tests for helper functions (`formatResults`, `jsonTextResult`, `errorResult`, etc.) in tools/helpers_test.go

### Add Tests for client/ Package

- [ ] T019 [P] [US2] Write tests for Logseq client (`(*Client).call`, `New`, `GetPage`, `Search`) using httptest mock server in client/logseq_test.go

### Add Tests for Decomposed Functions

- [ ] T020 [P] [US2] Write tests for decomposed `executeServe` helper functions (backend init, store init, embedder init) in main_test.go
- [ ] T021 [P] [US2] Write tests for decomposed source factory functions (createDiskSource, createGitHubSource, createWebSource) in source/manager_test.go

### Verification

- [ ] T022 [US2] Run `go test -race -count=1 -coverprofile=coverage.out ./...` to verify all new tests pass and no existing tests break
- [ ] T023 [US2] Run `gaze crap ./... --format=json --coverprofile=coverage.out` and verify CRAPload ≤53 (target: ≤15% of total functions)
- [ ] T024 [US2] Tighten CRAPload threshold in CI: update `--max-crapload=88` to the achieved value in .github/workflows/ci.yml

**Checkpoint**: CRAPload at B grade (≤53). CI ratchet tightened. Ready for contract coverage improvement.

---

## Phase 4: User Story 3 -- Improve Contract Coverage to B Grade (Priority: P3)

**Goal**: Improve contract coverage from 70.1% (grade C) to ≥80% (grade B) by strengthening test assertions to verify observable behavior, not just exercise code paths.

**Independent Test**: Run `gaze quality ./... --format=json --coverprofile=coverage.out` and verify average contract coverage ≥80%.

### Strengthen Assertions for Q4 (Dangerous) Functions

- [ ] T025 [P] [US3] Strengthen assertions for `(*Semantic).Similar` (GazeCRAP 165, 33% contract coverage): add assertions verifying similarity scores, result ordering, provenance metadata, and error returns in tools/semantic_test.go
- [ ] T026 [P] [US3] Strengthen assertions for `(*VaultStore).IncrementalIndex` (GazeCRAP 97, 40% contract coverage): add assertions verifying changed file detection, persistence of only changed pages, correct stats reporting in vault/vault_store_test.go
- [ ] T027 [P] [US3] Strengthen assertions for `(*DiskSource).Diff` (GazeCRAP 82, 33% contract coverage): add assertions verifying new/modified/deleted file detection, content hash comparison accuracy in source/disk_test.go
- [ ] T028 [P] [US3] Strengthen assertions for remaining Q4 functions identified by Gaze, verifying return values and side effects in their respective test files

### Strengthen Assertions for Q3 (Needs Tests) Functions

- [ ] T029 [P] [US3] Strengthen assertions for Q3 functions in tools/semantic_test.go: verify semantic search result structure, filter application, empty index behavior, error message content
- [ ] T030 [P] [US3] Strengthen assertions for Q3 functions in source/ test files: verify source manager orchestration outcomes, config parsing results, web crawl content extraction
- [ ] T031 [P] [US3] Strengthen assertions for Q3 functions in store/ test files: verify embedding cosine similarity ranking correctness, filtered search result accuracy, LIKE pattern escaping effectiveness

### Verification

- [ ] T032 [US3] Run `go test -race -count=1 -coverprofile=coverage.out ./...` to verify all strengthened tests pass
- [ ] T033 [US3] Run `gaze quality ./... --format=json --coverprofile=coverage.out` and verify average contract coverage ≥80%
- [ ] T034 [US3] Run `gaze crap ./... --format=json --coverprofile=coverage.out` and verify GazeCRAPload ≤10 and all Q4 functions eliminated
- [ ] T035 [US3] Tighten all thresholds in CI to achieved values: update `--max-crapload`, `--max-gaze-crapload`, `--min-contract-coverage` in .github/workflows/ci.yml

**Checkpoint**: Contract coverage at B grade (≥80%). GazeCRAPload ≤10. All Q4 functions eliminated. CI ratchet locked.

---

## Phase 5: Polish & Cross-Cutting Concerns

- [ ] T036 Run full `go build ./...`, `go test -race -count=1 ./...`, `go vet ./...` suite to verify clean build
- [ ] T037 Run `gaze report ./... --format=json --coverprofile=coverage.out --max-crapload=53 --max-gaze-crapload=10 --min-contract-coverage=80 > /dev/null` to verify final CI thresholds pass
- [ ] T038 Verify all 40 MCP tools still function correctly (backward compatibility gate)

**Checkpoint**: All quality targets met. CI enforces ratcheted thresholds. Ready for PR.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies -- establish baseline
- **US1 CI (Phase 2)**: Depends on Setup -- CI config only, no code changes
- **US2 CRAPload (Phase 3)**: Depends on US1 -- ratchet must exist before tightening
- **US3 Contract Coverage (Phase 4)**: Depends on US2 -- assert on decomposed functions
- **Polish (Phase 5)**: Depends on all user stories

### User Story Dependencies

- **US1 (P1)**: Independent -- CI workflow changes only
- **US2 (P2)**: Depends on US1 (ratchet must exist to lock in gains)
- **US3 (P3)**: Depends on US2 (decomposed functions are easier to assert on; many Q3/Q4 functions are the same ones being tested in US2)

### Within US2 (CRAPload Reduction)

- T008-T009 (decomposition) MUST complete before T010 (regression check)
- T011 (mock backend) MUST complete before T012-T018 (tool tests)
- T012-T021 are all [P] -- they can run in parallel (different test files)
- T022-T024 (verification) MUST be last

### Within US3 (Contract Coverage)

- T025-T031 are all [P] -- they can run in parallel (different test files)
- T032-T035 (verification) MUST be last

---

## Parallel Example: User Story 2

```bash
# Step 1 - Sequential decomposition:
Task: "T008 Decompose executeServe in main.go"
Task: "T009 Decompose createSource in source/manager.go"
Task: "T010 Verify no regressions"

# Step 2 - Mock backend (required by all tool tests):
Task: "T011 Create mock backend in tools/mock_backend_test.go"

# Step 3 - All test additions in parallel:
Task: "T012 [P] Decision tool tests"
Task: "T013 [P] Journal tool tests"
Task: "T014 [P] Search tool tests"
Task: "T015 [P] Analyze tool tests"
Task: "T016 [P] Write tool tests"
Task: "T017 [P] Navigate tool tests"
Task: "T018 [P] Helper function tests"
Task: "T019 [P] Logseq client tests"
Task: "T020 [P] Decomposed serve function tests"
Task: "T021 [P] Decomposed source factory tests"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Establish baseline
2. Complete Phase 2: Add Gaze to CI with current baselines
3. **STOP and VALIDATE**: Push a PR, verify CI runs Gaze and passes
4. The ratchet is now in place -- quality cannot regress

### Incremental Delivery

1. US1 (CI) → ratchet mechanism established
2. US2 (CRAPload) → 35+ functions drop below threshold, ratchet tightened
3. US3 (Contract Coverage) → assertions strengthened, ratchet tightened again
4. Each story tightens the ratchet, locking in gains permanently

---

## Notes

- [P] tasks = different files, no dependencies on incomplete tasks
- [Story] label maps task to specific user story for traceability
- The 4 `decompose_and_test` functions (CRAP >300) are the highest-impact targets
- The `tools/` package accounts for the majority of untested functions -- a shared mock backend makes this tractable
- Gaze threshold flags: `--max-crapload=N`, `--max-gaze-crapload=N`, `--min-contract-coverage=N`
- Coverage reuse: generate `coverage.out` once with `go test`, pass to Gaze with `--coverprofile`
