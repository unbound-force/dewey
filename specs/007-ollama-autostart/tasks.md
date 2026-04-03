# Tasks: Ollama Auto-Start

**Input**: Design documents from `/specs/007-ollama-autostart/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, quickstart.md

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup — Types & Interface (Blocking Prerequisites)

**Purpose**: Define the `OllamaState` type and `ollamaStarter` interface that all subsequent phases depend on.

**⚠️ CRITICAL**: No implementation work can begin until this phase is complete.

- [x] T001 [P] [US1] Define `OllamaState` type (iota enum: `OllamaExternal`, `OllamaManaged`, `OllamaUnavailable`) with `String()` method in `main.go`. Place above `initObsidianBackend()`. GoDoc comments on type and each constant per quickstart.md.
- [x] T002 [P] [US1] Define `ollamaStarter` interface (`Start() error`) and `execOllamaStarter` struct in `main.go`. The production implementation uses `exec.Command("ollama", "serve")` with `SysProcAttr{Setpgid: true}` and nil stdout/stderr per research.md R3.

**Checkpoint**: Types compile. `go build ./...` passes. No behavioral changes yet.

---

## Phase 2: Core Functions (US1 — Zero-Config Semantic Search)

**Purpose**: Implement the three core functions that drive the Ollama state machine.

- [x] T003 [P] [US1] Implement `isLocalEndpoint(endpoint string) bool` in `main.go`. Parse URL, check hostname against `localhost`, `127.0.0.1`, `::1`. Return `false` on parse error. Pure function, no I/O. (FR-008)
- [x] T004 [P] [US1] Implement `ollamaHealthCheck(endpoint string) bool` in `main.go`. `GET /api/tags` with 2-second `http.Client` timeout. Return `true` only on `200 OK`. No error logging — caller decides. (FR-001)
- [x] T005 [US1] Implement `ensureOllama(endpoint string, autoStart bool, starter ollamaStarter) (OllamaState, error)` in `main.go`. State machine per research.md R2: (1) health check → `OllamaExternal` if healthy, (2) if `!autoStart` or `!isLocalEndpoint` → `OllamaUnavailable`, (3) `exec.LookPath("ollama")` → `OllamaUnavailable` if not found, (4) `starter.Start()` → poll health every 500ms up to 30s → `OllamaManaged` or `OllamaUnavailable`. (FR-001, FR-002, FR-003, FR-008, FR-010)

**Checkpoint**: Core functions compile. `go build ./...` passes. Functions are not yet wired into serve/index paths.

---

## Phase 3: Integration — Wire into Serve & Index (US1, US2, US3)

**Purpose**: Connect `ensureOllama()` into the existing `initObsidianBackend()` and `createIndexEmbedder()` call sites, replacing hard errors with auto-start + graceful degradation.

- [x] T006 [US1] Modify `initObsidianBackend()` in `main.go` (lines 365-376): Insert `ensureOllama(embedEndpoint, true, &execOllamaStarter{})` call before embedder creation. When `OllamaUnavailable`, log info and skip embedder (keyword-only mode). When `External` or `Managed`, create `OllamaEmbedder` and check `Available()`. Preserve hard error when model is not pulled. (FR-002, FR-005, FR-006, FR-007)
- [x] T007 [US1] Modify `createIndexEmbedder()` in `cli.go` (lines 840-862): Same pattern as T006 — call `ensureOllama(embedEndpoint, true, &execOllamaStarter{})` before creating embedder. Return `nil, nil` when `OllamaUnavailable` (graceful degradation for `dewey index`/`dewey reindex`).
- [x] T008 [US4] Verify subprocess detachment: Confirm that `execOllamaStarter.Start()` uses `cmd.Start()` (not `cmd.Run()`), `Setpgid: true`, and does not store the `*exec.Cmd` — Dewey does not track or kill the subprocess on exit. Add a code comment citing FR-004. (FR-004)

**Checkpoint**: `go build ./...` passes. `dewey serve` with Ollama stopped should attempt auto-start. `dewey serve` with `--no-embeddings` should skip Ollama entirely.

---

## Phase 4: Doctor — Report Ollama State (US1, US2, US3)

**Purpose**: Enhance `dewey doctor` to report the Ollama state without auto-starting.

- [x] T009 [US1] Modify `runDoctorChecks()` in `cli.go` (lines 1335-1355): Replace the current ollama reachability check with `ensureOllama(embedEndpoint, false, nil)` (autoStart=false). Report state: `External` → "running (external)", `Unavailable` with binary in PATH → "not running (start with: ollama serve)", `Unavailable` without binary → "not installed (install: brew install ollama)". (FR-009)
- [x] T010 [US1] Update doctor output format: When Ollama state is `External`, show PASS. When `Unavailable` with binary present, show WARN with fix hint. When `Unavailable` without binary, show INFO (not FAIL — it's a valid configuration per composability principle). Preserve existing model availability and embedding count checks.

**Checkpoint**: `go build ./...` passes. `dewey doctor` shows enhanced Ollama state. No auto-start triggered by doctor.

---

## Phase 5: Tests — Unit Tests for All New Functions

**Purpose**: Cover all branches of the Ollama state machine and helper functions.

- [x] T011 [P] [US1] Add `TestOllamaState_String` in `main_test.go`: Verify `String()` returns `"external"`, `"managed"`, `"unavailable"` for each state, and `"unknown"` for an out-of-range value.
- [x] T012 [P] [US1] Add `TestIsLocalEndpoint` in `main_test.go`: Table-driven test covering `localhost`, `127.0.0.1`, `::1` (true), remote hosts like `gpu-server`, `192.168.1.100` (false), malformed URLs (false), and empty string (false).
- [x] T013 [P] [US1] Add `TestOllamaHealthCheck_Healthy` and `TestOllamaHealthCheck_Unreachable` in `main_test.go`: Use `httptest.NewServer` returning 200 for healthy. Use a closed listener URL for unreachable. Verify boolean return values.
- [x] T014 [P] [US2] Add `TestEnsureOllama_AlreadyRunning` in `main_test.go`: Use `httptest.NewServer` returning 200. Verify returns `OllamaExternal` and nil error. Verify mock starter's `Start()` is NOT called (use a mock that records calls).
- [x] T015 [P] [US3] Add `TestEnsureOllama_BinaryNotFound` in `main_test.go`: Use a non-listening endpoint. Temporarily manipulate PATH to exclude `ollama` (or test with a non-existent binary name). Verify returns `OllamaUnavailable`.
- [x] T016 [P] [US1] Add `TestEnsureOllama_RemoteEndpoint` in `main_test.go`: Use a non-local endpoint (e.g., `http://gpu-server:11434`). Verify returns `OllamaUnavailable` and no start attempt (FR-008).
- [x] T017 [US1] Add `TestEnsureOllama_StartSuccess` in `main_test.go`: Use mock `ollamaStarter` that returns nil. Use `httptest.NewServer` that starts returning 200 after the first poll. Verify returns `OllamaManaged`.
- [x] T018 [US1] Add `TestEnsureOllama_AutoStartDisabled` in `main_test.go`: Pass `autoStart=false` with a local endpoint and binary available. Verify returns `OllamaUnavailable` and no start attempt.

**Checkpoint**: `go test -race -count=1 ./...` passes. All new functions have test coverage. No external dependencies required.

---

## Phase 6: Doctor Test Updates

**Purpose**: Update existing doctor test assertions to match the enhanced Ollama state output.

- [x] T019 Update `TestRunDoctorChecks` assertions in `cli_test.go` (around line 2619): Adjust "Embedding Layer" section assertions to match the new state-based output format (e.g., "running (external)" or "not installed" instead of the old "running" / "not reachable" labels).

**Checkpoint**: `go test -race -count=1 ./...` passes including updated doctor tests.

---

## Phase 7: Verification — CI Parity Gate

**Purpose**: Replicate CI checks locally before declaring implementation complete.

- [x] T020 Run full CI parity gate: `go build ./...` && `go vet ./...` && `go test -race -count=1 ./...`. All must pass. Verify no regressions in existing tests. Check that `TestInitObsidianBackend_HardError_WhenOllamaUnavailable` still passes (behavior changed from hard error to graceful degradation — test may need updating).
- [x] T021 Documentation validation: Update `AGENTS.md` Active Technologies section if needed (verify `os/exec`, `net/http` are listed for 007). Verify GoDoc comments on all new exported symbols (`OllamaState`, `OllamaExternal`, `OllamaManaged`, `OllamaUnavailable`). No README changes needed (no new CLI flags or commands).

**Checkpoint**: All CI-equivalent checks pass locally. Documentation is current. Feature is ready for `/review-council`.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies — can start immediately. T001 and T002 are parallel (different code sections).
- **Phase 2 (Core)**: Depends on Phase 1 (uses `OllamaState` type and `ollamaStarter` interface). T003 and T004 are parallel (independent pure functions). T005 depends on T001–T004.
- **Phase 3 (Integration)**: Depends on Phase 2 (uses `ensureOllama()`). T006 and T007 are sequential (T007 follows same pattern as T006). T008 is parallel with T006/T007 (review-only task).
- **Phase 4 (Doctor)**: Depends on Phase 2 (uses `ensureOllama()` with `autoStart=false`). Can run in parallel with Phase 3.
- **Phase 5 (Tests)**: Depends on Phase 2 (tests the core functions). T011–T016 are parallel (independent test functions). T017–T018 depend on mock setup patterns from T014. Can run in parallel with Phases 3–4.
- **Phase 6 (Doctor Tests)**: Depends on Phase 4 (tests the doctor output changes).
- **Phase 7 (Verification)**: Depends on all previous phases.

### Parallel Opportunities

```
Phase 1: T001 ─┐
               ├─ parallel (different code sections in main.go)
         T002 ─┘

Phase 2: T003 ─┐
               ├─ parallel (independent functions)
         T004 ─┘
         T005 ── sequential (depends on T001-T004)

Phase 3: T006 → T007 (sequential, same pattern)
         T008 ── parallel with T006/T007

Phase 4: T009 → T010 (sequential, same function)
         Can run parallel with Phase 3

Phase 5: T011, T012, T013, T014, T015, T016 ── all parallel
         T017, T018 ── sequential after mock pattern established

Phase 6: T019 ── depends on Phase 4

Phase 7: T020 → T021 (sequential, final gate)
```

### User Story Mapping

| Story | Tasks | What it delivers |
|-------|-------|-----------------|
| **US1** (P1) Zero-Config Semantic Search | T001–T006, T009–T013, T016–T018, T020–T021 | Auto-start Ollama when not running |
| **US2** (P2) Respect External Ollama | T004, T006, T014 | Detect and use existing Ollama |
| **US3** (P3) Graceful Degradation | T006, T009–T010, T015 | Keyword-only mode when unavailable |
| **US4** (P4) Leave Ollama Running | T002, T008 | Detached subprocess, no cleanup |

---

## Notes

- [P] tasks = different files or independent functions, no dependencies
- [Story] label maps task to specific user story for traceability
- Total: **21 tasks** across 7 phases
- Scope: ~100 lines production code (`main.go`, `cli.go`), ~150 lines test code (`main_test.go`, `cli_test.go`)
- No new packages, no new dependencies, no storage changes
- The `--no-embeddings` bypass is automatic (existing `if noEmbeddings` branch in both `initObsidianBackend()` and `createIndexEmbedder()`)
- `dewey doctor` uses `ensureOllama(autoStart=false)` — it reports state but never starts Ollama
<!-- spec-review: passed -->
