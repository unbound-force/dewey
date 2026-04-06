# Implementation Plan: Structured Diagnostics

**Branch**: `009-structured-diagnostics` | **Date**: 2026-04-06 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `specs/009-structured-diagnostics/spec.md`

## Summary

Add structured diagnostic logging to Dewey so AI agents can diagnose MCP startup timeouts, lock conflicts, and silent file skips by reading the log file alone. Changes span 4 packages (~10 files, ~30 insertion points) and introduce: ISO 8601 timestamps with millisecond precision on all loggers, per-package component prefixes (`dewey`, `dewey/vault`, `dewey/source`, `dewey/ignore`), phase timing with elapsed durations on every startup phase, a "server ready" marker with transport/tool-count/startup-time, PID+command in lock files, DEBUG logging for 8 silent file skips, Ollama polling progress, and shutdown logging. No new packages, no new dependencies, no schema changes.

## Technical Context

**Language/Version**: Go 1.25 (per `go.mod`)
**Primary Dependencies**: `github.com/charmbracelet/log` (structured logging), `github.com/spf13/cobra` (CLI), `github.com/modelcontextprotocol/go-sdk/mcp` (MCP server), `modernc.org/sqlite` (persistence)
**Storage**: N/A (no storage changes — this feature modifies logging, not the SQLite store schema)
**Testing**: `go test -race -count=1 ./...` (standard library `testing` package only)
**Target Platform**: darwin/linux (amd64/arm64)
**Project Type**: CLI + MCP server
**Performance Goals**: Zero measurable overhead — `time.Now()` calls add <1μs per phase
**Constraints**: No new dependencies. No CGO. All existing 43 MCP tools must continue to work identically.
**Scale/Scope**: ~10 files modified, ~30 insertion points, 0 new files

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

### I. Composability First — PASS

This feature modifies only internal logging behavior. Dewey remains independently installable and usable without any other Unbound Force tool. No new external dependencies are introduced. The `ignore` package wiring (`SetLogOutput`, `SetLogLevel`) follows the same pattern already established by `vault` and `source` packages — no coupling is introduced.

### II. Autonomous Collaboration — PASS

All 43 MCP tools continue to communicate via structured JSON responses. Logging changes are internal to the server process and do not affect MCP tool input/output contracts. The "server ready" marker is a log line, not an MCP tool response — it does not create runtime coupling.

### III. Observable Quality — PASS

This feature *improves* observability. Phase timing, timestamps, and component prefixes make the system more auditable. The lock file PID content makes `.dewey/` artifacts more inspectable. The `health` and `dewey status` tools are unaffected. The `dewey doctor` command is enhanced with PID-based lock holder reporting (FR-008, FR-009).

### IV. Testability — PASS

All changes are testable in isolation:
- Logger configuration changes are verified by capturing output to `bytes.Buffer`
- Lock file PID write/read is tested with temp directories and `:memory:` stores
- `detectLockHolder()` is already a standalone function with existing tests
- Silent file skip logging is tested with `t.TempDir()` and permission manipulation
- No external services required (Ollama polling progress is tested via the existing `mockOllamaStarter` pattern)

**Coverage strategy**: Tests verify the contract surface — PID is written to lock file, PID is read back by `detectLockHolder()`, `SetLogOutput()` changes the logger output, DEBUG messages appear when level is set. Tests do NOT assert on timestamp values (non-deterministic) or exact log line formatting (fragile).

## Project Structure

### Documentation (this feature)

```text
specs/009-structured-diagnostics/
├── plan.md              # This file
├── research.md          # Phase 0 research findings
├── quickstart.md        # Phase 1 design summary
└── tasks.md             # Phase 2 output (/speckit.tasks command)
```

### Source Code (repository root)

```text
main.go                  # Logger timestamps+prefix, phase timing, server ready marker,
                         #   Ollama polling progress, shutdown log, ignore wiring
server.go                # newServer() returns tool count alongside *mcp.Server
cli.go                   # detectLockHolder() reads PID from lock file,
                         #   doctor output for PID display + stale lock detection
store/store.go           # Write PID+command to lock file after flock, DEBUG lock logs
vault/vault.go           # Logger prefix dewey/vault + timestamps, 3 silent skip DEBUG logs
vault/vault_store.go     # 2 silent skip DEBUG logs in walkVault()
source/source.go         # Logger prefix dewey/source + timestamps
source/disk.go           # 4 silent skip DEBUG logs in List() and walkDiskFiles()
ignore/ignore.go         # Logger prefix dewey/ignore + timestamps, add SetLogOutput()
```

**Structure Decision**: No new files or packages. All changes are modifications to existing files in existing packages. This is a cross-cutting logging enhancement, not a new feature module.

### Test Files Affected

```text
main_test.go             # Update TestEnsureOllama tests if log output assertions exist,
                         #   add TestServerReadyMarker if feasible
cli_test.go              # Update TestDoctorCmd tests for new PID-based lock holder output
store/store_test.go      # Add TestStore_LockFilePID for PID write verification
```

## Implementation Phases

### Phase 1: Logger Infrastructure (FR-001, FR-004, FR-005)

Update all 4 logger declarations with timestamps and distinct prefixes. Wire the `ignore` package to verbose/file-logging. This is the foundation — all subsequent phases depend on correct logger configuration.

**Changes**:
1. `main.go:31-33` — Add `ReportTimestamp: true`, `TimeFormat: "2006-01-02T15:04:05.000Z07:00"` to logger Options
2. `vault/vault.go:24-26` — Change prefix to `dewey/vault`, add timestamp options
3. `source/source.go:17-19` — Change prefix to `dewey/source`, add timestamp options
4. `ignore/ignore.go:21-23` — Change prefix to `dewey/ignore`, add timestamp options
5. `ignore/ignore.go` — Add `SetLogOutput(w io.Writer, level log.Level)` function (mirrors vault/source pattern)
6. `main.go:74-79` (PersistentPreRunE) — Add `ignore.SetLogLevel(log.DebugLevel)` call
7. `main.go:153-157` (setupFileLogging) — Update `log.Options` in `newLogger` to include timestamps + correct prefix; add `ignore.SetLogOutput(multi, level)` call
8. `vault/vault.go:37` (SetLogOutput) — Update Options to include `dewey/vault` prefix + timestamps
9. `source/source.go:29` (SetLogOutput) — Update Options to include `dewey/source` prefix + timestamps

**Verification**: `go build ./...` + `go test -race -count=1 ./...` — all existing tests pass with new prefixes/timestamps.

### Phase 2: Phase Timing (FR-002, FR-003, FR-013)

Add `time.Now()` / `time.Since()` timing to each startup phase and emit the "server ready" marker.

**Changes**:
1. `main.go` (executeServe) — Add `startupStart := time.Now()` at function entry
2. `main.go` (initObsidianBackend) — Add timing around: store open (`store.New`), Ollama check (`ensureOllama`), vault indexing (`indexVault`), external page load (`LoadExternalPages`), watcher start (`vc.Watch`)
3. `main.go` (executeServe) — After `newServer()`, emit `logger.Info("server ready", "transport", transport, "tools", toolCount, "startup", time.Since(startupStart).Round(time.Millisecond))`
4. `server.go` — Modify `newServer()` to return `(*mcp.Server, int)` where int is tool count
5. `main.go` (runServer) — Add `logger.Info("server stopped")` after `srv.Run()` returns in stdio path

**Transport determination**: `httpAddr` is available in `executeServe()` — if non-empty, transport is `"http"`, otherwise `"stdio"`.

**Tool count approach**: Add a counter in `newServer()` that increments with each `AddTool`/`mcp.AddTool` call. Return the count alongside the server. This avoids reflection and stays accurate as tools are added/removed.

**Verification**: `go build ./...` + `go test -race -count=1 ./...`

### Phase 3: Lock File PID (FR-006, FR-007, FR-008, FR-009)

Write PID+command to lock file, read it back in `detectLockHolder()`, update doctor output.

**Changes**:
1. `store/store.go:84-89` — After `Flock()` succeeds, add: seek to 0, truncate, write `fmt.Fprintf(lockFile, "%d %s\n", os.Getpid(), strings.Join(os.Args, " "))`
2. `store/store.go:84` — Add `logger.Debug("lock acquired", "pid", os.Getpid(), "path", lockPath)` (requires adding a package-level logger to store, or using the existing pattern)
3. `store/store.go:104-106` — Add `logger.Debug("lock released")` before unlock
4. `cli.go:818-834` (detectLockHolder) — Read lock file content with `io.ReadAll()`, parse PID line with `strings.SplitN(line, " ", 2)`, return `"PID 12345 (dewey serve --vault /path)"` when lock is held
5. `cli.go:818-834` (detectLockHolder) — When lock is NOT held but file has PID content, return stale lock info: `"stale lock (was PID 12345)"` with suggestion to remove
6. `cli.go:1400-1405` (doctor) — Update display to use new PID-based holder string

**store package logger**: The `store` package currently has no logger. Two options:
- **Option A**: Add a package-level `log.Logger` to `store/store.go` (consistent with vault/source/ignore pattern)
- **Option B**: Use `fmt.Fprintf` to the lock file only (no logging dependency)

**Decision**: Option A — add a package-level logger with prefix `dewey/store` and timestamps. This is consistent with the project pattern and enables future store-level diagnostics. The logger is only used for lock acquire/release DEBUG messages in this feature.

**Verification**: `go build ./...` + `go test -race -count=1 ./...` + new unit tests for PID write/read.

### Phase 4: Silent File Skips + Ollama Progress (FR-010, FR-011, FR-012)

Add DEBUG logging to all silent file skip locations and Ollama polling progress.

**Changes**:
1. `vault/vault.go:169-170` — `logger.Debug("skipping path", "path", path, "err", walkErr)` before `return nil`
2. `vault/vault.go:191-192` — `logger.Debug("skipping unreadable file", "path", path, "err", readErr)` before `return nil`
3. `vault/vault.go:339-340` — `logger.Debug("skipping path in watcher", "path", path, "err", err)` before `return nil`
4. `vault/vault_store.go:227-228` — `logger.Debug("skipping path", "path", path, "err", walkErr)` before `return nil`
5. `vault/vault_store.go:244-245` — `logger.Debug("skipping unreadable file", "path", path, "err", readErr)` before `return nil`
6. `source/disk.go:110-111` — `logger.Debug("skipping path", "path", path, "err", walkErr)` before `return nil`
7. `source/disk.go:135-136` — `logger.Debug("skipping unreadable file", "path", path, "err", err)` before `return nil`
8. `source/disk.go:217-218` — `logger.Debug("skipping path", "path", path, "err", walkErr)` before `return nil`
9. `source/disk.go:238-240` — `logger.Debug("skipping unreadable file", "path", path, "err", err)` before `return nil`
10. `main.go` (ensureOllama) — Add progress logging every 5 seconds during polling loop
11. `main.go` (executeServe or initObsidianBackend) — Log resolved vault path at startup (FR-012)

**Verification**: `go build ./...` + `go test -race -count=1 ./...`

### Phase 5: Test Updates + Final Validation

Update existing tests that may break due to prefix/timestamp changes, add new tests for PID lock file behavior.

**Changes**:
1. Update any tests in `cli_test.go` that assert on `detectLockHolder()` output strings (currently checks for `"try: lsof"`)
2. Update any tests that capture log output and check for `"dewey:"` prefix (now `"dewey/vault:"` etc.)
3. Add `TestStore_LockFilePID` — verify PID is written to lock file after `store.New()`
4. Add `TestDetectLockHolder_WithPID` — verify PID is read back correctly
5. Add `TestDetectLockHolder_StaleLock` — verify stale lock detection
6. Run full CI-equivalent checks: `go build ./...`, `go vet ./...`, `go test -race -count=1 ./...`

**Verification**: All CI checks pass. `gaze report` (if available) shows no regressions.

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Test breakage from prefix changes | Medium | Low | Tests use `strings.Contains()` for message matching, not exact prefix matching. Grep for `"dewey:"` in test assertions to find affected tests. |
| Lock file write failure | Low | Low | Write is best-effort — if `Fprintf` fails, the lock is still held (flock is the real lock). Log the write error at DEBUG level. |
| Ollama polling log spam | Low | Low | Progress logs are DEBUG-level (only visible with `--verbose`) and throttled to every 5 seconds. |
| `newServer()` signature change breaks callers | Low | Medium | Only 2 call sites (`executeServe` and `newServeCmd`). Both are in `main.go`. Update both simultaneously. |
| store package logger adds import | Low | Low | `charmbracelet/log` is already a project dependency. Adding it to `store` adds no new external dependency. |

## Complexity Tracking

> No constitution violations. All changes align with existing patterns.

| Aspect | Complexity | Justification |
|--------|-----------|---------------|
| 4 logger declarations | Low | One-line change per declaration (add 2 fields to Options struct) |
| Phase timing | Low | `time.Now()` + `time.Since()` — idiomatic Go, no abstraction needed |
| Lock file PID | Medium | Requires coordinating write (store) and read (cli) across packages. Tested with unit tests. |
| newServer() signature | Low | 2 call sites, both in main.go |
| 9 silent skip logs | Low | Mechanical — add one `logger.Debug()` line before each `return nil` |
| store package logger | Low | Follows established vault/source/ignore pattern |
