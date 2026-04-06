# Tasks: Structured Diagnostics

**Input**: Design documents from `specs/009-structured-diagnostics/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, quickstart.md

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3, US4)
- Include exact file paths in descriptions

## User Story Mapping

| Story | Title | Priority | Spec FRs |
|-------|-------|----------|----------|
| US1 | Diagnose MCP Startup Timeout | P1 | FR-001, FR-002, FR-003, FR-011, FR-012, FR-013 |
| US2 | Identify Lock File Holder | P2 | FR-006, FR-007, FR-008, FR-009 |
| US3 | Trace Which Package Emitted a Log | P3 | FR-004, FR-005 |
| US4 | Surface Silent File Skips | P4 | FR-010 |

---

## Phase 1: Logger Infrastructure (FR-001, FR-004, FR-005)

**Purpose**: Update all 4 logger declarations with ISO 8601 timestamps and distinct per-package prefixes. Wire the `ignore` package to verbose/file-logging. This is the foundation — all subsequent phases depend on correct logger configuration.

**⚠️ CRITICAL**: No other phase can begin until this phase is complete.

- [x] T001 [P] [US3] Update main logger in `main.go:31-33` — add `ReportTimestamp: true`, `TimeFormat: "2006-01-02T15:04:05.000Z07:00"` to `log.Options`. Prefix stays `"dewey"`.
- [x] T002 [P] [US3] Update vault logger in `vault/vault.go:24-26` — change prefix to `"dewey/vault"`, add `ReportTimestamp: true` and ISO 8601 `TimeFormat`. Also update `SetLogOutput()` at line 37 to include prefix `"dewey/vault"` and timestamp options.
- [x] T003 [P] [US3] Update source logger in `source/source.go:17-19` — change prefix to `"dewey/source"`, add `ReportTimestamp: true` and ISO 8601 `TimeFormat`. Also update `SetLogOutput()` at line 29 to include prefix `"dewey/source"` and timestamp options.
- [x] T004 [P] [US3] Update ignore logger in `ignore/ignore.go:21-23` — change prefix to `"dewey/ignore"`, add `ReportTimestamp: true` and ISO 8601 `TimeFormat`. Add new `SetLogOutput(w io.Writer, level log.Level)` function mirroring the vault/source pattern.
- [x] T005 [US3] Wire ignore package to verbose/file-logging in `main.go` — add `ignore.SetLogLevel(log.DebugLevel)` in `PersistentPreRunE` verbose block. Add `ignore.SetLogOutput(multi, level)` in `setupFileLogging()` at line 157. Update `newLogger` in `setupFileLogging()` to include timestamp options and correct prefix.

**Checkpoint**: `go build ./...` passes. All 4 loggers emit ISO 8601 timestamps and distinct prefixes. `go test -race -count=1 ./...` passes (no test assertions broken by prefix changes).

---

## Phase 2: Phase Timing + Server Ready + Shutdown (FR-002, FR-003, FR-012, FR-013)

**Purpose**: Add `time.Now()` / `time.Since()` timing to each startup phase, emit the "server ready" marker with transport/tool-count/startup-time, log vault path, and add shutdown logging.

- [x] T006 [US1] Modify `newServer()` in `server.go:40` to return `(*mcp.Server, int)` — add a tool counter that increments with each `AddTool`/`mcp.AddTool` call across all `registerXxxTools` functions. Update `return srv` at line 111 to `return srv, toolCount`. Update the call site in `main.go:233` to capture both values: `srv, toolCount := newServer(...)`.
- [x] T007 [US1] Add phase timing to `initObsidianBackend()` in `main.go:410-554` — add `phaseStart := time.Now()` before each phase (store open, Ollama check, vault indexing, external page load, watcher start) and add `"elapsed", time.Since(phaseStart).Round(time.Millisecond)` as a structured field to each phase's existing completion log line. Log resolved vault path (`logger.Info("vault path resolved", "path", vp)`) after `resolveVaultPath()` at line 411 (FR-012).
- [x] T008 [US1] Add startup timing and "server ready" marker in `executeServe()` in `main.go:196-239` — add `startupStart := time.Now()` at function entry. After `newServer()` returns, emit `logger.Info("server ready", "transport", transport, "tools", toolCount, "startup", time.Since(startupStart).Round(time.Millisecond))` where transport is `"http"` if `httpAddr != ""`, else `"stdio"`.
- [x] T009 [US1] Add shutdown logging in `runServer()` in `main.go:610-646` — add `logger.Info("server stopped")` after `srv.Run()` returns in the stdio path (line 644). HTTP path already has shutdown logging.

**Checkpoint**: `go build ./...` passes. `go test -race -count=1 ./...` passes. Starting `dewey serve --verbose` shows phase elapsed times, vault path, server ready marker, and shutdown log.

---

## Phase 3: Lock File PID (FR-006, FR-007, FR-008, FR-009)

**Purpose**: Write PID+command to lock file after flock acquisition, read it back in `detectLockHolder()`, update doctor output for PID display and stale lock detection.

- [x] T010 [US2] Add package-level logger to `store/store.go` — add `var logger = log.NewWithOptions(os.Stderr, log.Options{Prefix: "dewey/store", ReportTimestamp: true, TimeFormat: "2006-01-02T15:04:05.000Z07:00"})` and import `"github.com/charmbracelet/log"`. Add `SetLogLevel()` and `SetLogOutput()` functions matching the vault/source/ignore pattern. Wire in `main.go` (`PersistentPreRunE` and `setupFileLogging()`).
- [x] T011 [US2] Write PID+command to lock file in `store/store.go:84-89` — after `Flock()` succeeds, add: seek to 0, truncate, write `fmt.Fprintf(lockFile, "%d %s\n", os.Getpid(), strings.Join(os.Args, " "))`. Add `logger.Debug("lock acquired", "pid", os.Getpid(), "path", lockPath)`. Add `logger.Debug("lock released")` before unlock in `Close()` at line 105. Import `"strings"`.
- [x] T012 [US2] Update `detectLockHolder()` in `cli.go:818-834` — read lock file content with `io.ReadAll()`, parse PID line with `strings.SplitN(line, " ", 2)`. When lock IS held: return `"PID {pid} ({command})"`. When lock is NOT held but file has PID content: return stale lock info. When lock is NOT held and no PID content: return empty string (existing behavior).
- [x] T013 [US2] Update doctor output in `cli.go:1394-1408` — when lock is held, display `"running (PID 12345, dewey serve)"` instead of the raw holder string. When stale lock detected, use `WARN` status with suggestion to remove the lock file: `"stale lock (was PID 12345) — remove: rm {lockPath}"`.

**Checkpoint**: `go build ./...` passes. `go test -race -count=1 ./...` passes. Lock file contains PID after `dewey serve` starts. `dewey doctor` shows PID-based lock holder info.

---

## Phase 4: Silent File Skips + Ollama Polling (FR-010, FR-011)

**Purpose**: Add DEBUG logging to all silent file skip locations and Ollama polling progress. These are mechanical additions — one `logger.Debug()` line before each `return nil`.

- [x] T014 [P] [US4] Add silent skip DEBUG logs in `vault/vault.go` — add `logger.Debug("skipping path", "path", path, "err", walkErr)` before `return nil` at walk error (~line 169) and `logger.Debug("skipping unreadable file", "path", path, "err", readErr)` before `return nil` at ReadFile error (~line 191). Add same pattern in watcher walk error (~line 339).
- [x] T015 [P] [US4] Add silent skip DEBUG logs in `vault/vault_store.go` — add `logger.Debug("skipping path", "path", path, "err", walkErr)` before `return nil` at walk error (~line 227) and `logger.Debug("skipping unreadable file", "path", path, "err", readErr)` before `return nil` at ReadFile error (~line 244).
- [x] T016 [P] [US4] Add silent skip DEBUG logs in `source/disk.go` — add `logger.Debug("skipping path", "path", path, "err", walkErr)` and `logger.Debug("skipping unreadable file", "path", path, "err", err)` at each silent skip location in `List()` (~lines 110-136) and `walkDiskFiles()` (~lines 217-240).
- [x] T017 [US1] Add Ollama polling progress in `ensureOllama()` in `main.go:384-396` — track `start := time.Now()` and `lastLog := start` before the polling loop. Inside the loop, after the health check fails, log progress every 5 seconds: `logger.Debug("waiting for Ollama", "elapsed", time.Since(start).Round(time.Millisecond), "timeout", maxWait)`.

**Checkpoint**: `go build ./...` passes. `go test -race -count=1 ./...` passes. Running `dewey serve --verbose` on a vault with unreadable files shows DEBUG skip messages.

---

## Phase 5: Test Updates

**Purpose**: Update existing tests that break due to `newServer()` signature change. Add new tests for PID lock file behavior. Verify no test assertions are broken by prefix/timestamp changes.

- [x] T018 [US1] Update all `newServer()` call sites in `server_test.go` — change `srv := newServer(...)` to `srv, _ := newServer(...)` at all 15 call sites (lines 151, 162, 288, 345, 395, 420, 451, 480, 508, 526, 587, 654, 709, 753, 773). Add one test that verifies the tool count is > 0.
- [x] T019 [US2] Add `TestStore_LockFilePID` in `store/store_test.go` — create a temp dir, open a store with `store.New(dbPath)`, read the lock file content, verify it contains the current PID and command. Close the store and verify the lock file is released.
- [x] T020 [US2] Add `TestDetectLockHolder_WithPID` and `TestDetectLockHolder_StaleLock` in `cli_test.go` — test that `detectLockHolder()` returns PID info when lock is held (write PID to lock file, hold flock, call function). Test stale lock detection (write PID to lock file, release flock, call function).
- [x] T021 [US3] Scan all `*_test.go` files for assertions matching on `"dewey:"` prefix or exact log line formats. Update any affected assertions to account for new prefixes (`"dewey/vault:"`, `"dewey/source:"`, `"dewey/ignore:"`, `"dewey/store:"`) and timestamps. Use `strings.Contains()` for message matching, not exact prefix matching.

**Checkpoint**: `go test -race -count=1 ./...` passes with zero failures. All new tests verify contract surface (PID written/read, tool count > 0), not implementation details.

---

## Phase 6: Verification

**Purpose**: Run full CI-equivalent checks and validate documentation impact.

- [x] T022 Run CI-equivalent checks: `go build ./...`, `go vet ./...`, `go test -race -count=1 ./...`. If `gaze` is installed, run `gaze report ./... --coverprofile=coverage.out --max-crapload=48 --max-gaze-crapload=18 --min-contract-coverage=70`. All must pass.
- [x] T023 Documentation validation — update `AGENTS.md` if any new patterns are introduced (store package logger). Verify GoDoc comments on all new exported functions (`ignore.SetLogOutput`, `store.SetLogLevel`, `store.SetLogOutput`). No README changes needed (no new CLI flags or commands).

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Logger Infrastructure)**: No dependencies — start immediately. BLOCKS all other phases.
- **Phase 2 (Phase Timing)**: Depends on Phase 1 (needs timestamps on loggers). T006 (newServer signature) is independent of Phase 1 and can start early.
- **Phase 3 (Lock File PID)**: Depends on Phase 1 (needs store logger with timestamps). T012-T013 (cli.go changes) are independent of Phase 1.
- **Phase 4 (Silent Skips)**: Depends on Phase 1 (needs per-package loggers). T017 (Ollama polling) is independent of Phase 1.
- **Phase 5 (Test Updates)**: Depends on Phases 1-4 (tests validate all changes). T018 depends on T006 specifically.
- **Phase 6 (Verification)**: Depends on Phase 5 (all tests must pass first).

### Parallel Opportunities

Within Phase 1: T001, T002, T003, T004 can all run in parallel (different files). T005 depends on T001+T004.

Within Phase 4: T014, T015, T016 can all run in parallel (different files). T017 is independent.

Phases 2, 3, 4 can run in parallel after Phase 1 completes (they touch different files):
- Phase 2 touches `server.go` + `main.go` (executeServe, initObsidianBackend, runServer)
- Phase 3 touches `store/store.go` + `cli.go`
- Phase 4 touches `vault/vault.go`, `vault/vault_store.go`, `source/disk.go`, `main.go` (ensureOllama only)

⚠️ **Conflict**: Phase 2 (T007, T008) and Phase 4 (T017) both modify `main.go`. T017 modifies `ensureOllama()` while T007/T008 modify `initObsidianBackend()`/`executeServe()` — different functions, low conflict risk, but should be sequenced if working in the same session.

---

## Implementation Strategy

### Recommended Order (Single Developer)

1. Phase 1: T001→T004 (parallel), then T005
2. Phase 2: T006, T007, T008, T009
3. Phase 3: T010, T011, T012, T013
4. Phase 4: T014→T016 (parallel), T017
5. Phase 5: T018, T019, T020, T021
6. Phase 6: T022, T023

### MVP First (US1 Only)

1. Phase 1 (all) — logger infrastructure is shared
2. Phase 2 (T006-T009) — phase timing + server ready
3. Phase 5 (T018 only) — fix newServer test breakage
4. Phase 6 — verify

This delivers timestamp-based startup diagnosis without lock PID or silent skip features.
<!-- spec-review: passed -->
