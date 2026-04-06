# Quickstart: Structured Diagnostics

**Branch**: `009-structured-diagnostics` | **Date**: 2026-04-06

## What Changes

This feature adds structured diagnostic logging to Dewey so AI agents can diagnose startup timeouts, lock conflicts, and silent failures by reading the log file alone. No new packages, no new dependencies, no schema changes.

## Files Modified

| File | Changes |
|------|---------|
| `main.go` | Logger timestamp+prefix, phase timing in `initObsidianBackend`/`indexVault`/`executeServe`, server ready marker, Ollama polling progress, shutdown log, ignore package wiring |
| `store/store.go` | Write PID+command to lock file after flock acquisition, DEBUG log on lock acquire/release |
| `cli.go` | Read PID from lock file in `detectLockHolder()`, update doctor output for PID display and stale lock detection |
| `vault/vault.go` | Logger prefix `dewey/vault` + timestamps, DEBUG logs for 2 silent file skips in `Load()` |
| `vault/vault_store.go` | DEBUG logs for 2 silent file skips in `walkVault()` |
| `source/source.go` | Logger prefix `dewey/source` + timestamps |
| `source/disk.go` | DEBUG logs for 2-3 silent file skips in `List()`/`Diff()` |
| `ignore/ignore.go` | Logger prefix `dewey/ignore` + timestamps, add `SetLogOutput()` function |
| `server.go` | Return tool count from `newServer()` |

## Key Design Decisions

1. **Timestamps on all loggers**: `ReportTimestamp: true` + `TimeFormat: "2006-01-02T15:04:05.000Z07:00"` in all four `log.NewWithOptions()` calls and in `SetLogOutput()` functions. Timestamps appear regardless of TTY detection — log files are the primary consumer.

2. **Phase timing via time.Since()**: Each startup phase gets `phaseStart := time.Now()` before and `"elapsed", time.Since(phaseStart).Round(time.Millisecond)` added to its existing completion log line. No new log messages — just adding a field to existing ones.

3. **Server ready marker**: New `logger.Info("server ready", "transport", transport, "tools", toolCount, "startup", totalElapsed)` line in `executeServe()` after `newServer()` but before `runServer()`.

4. **Lock file PID**: `fmt.Fprintf(lockFile, "%d %s\n", os.Getpid(), strings.Join(os.Args, " "))` after flock acquisition in `store.New()`. Read back in `detectLockHolder()` with `io.ReadAll()` + `strings.SplitN()`.

5. **ignore wiring**: Add `ignore.SetLogOutput()` (new function) + call in `setupFileLogging()`. Add `ignore.SetLogLevel()` call in `PersistentPreRunE`.

6. **Silent file skips**: `logger.Debug("skipping file", "path", path, "err", err)` at each of 7 locations. Only visible with `--verbose`.

7. **Tool count**: Modify `newServer()` signature to return `(*mcp.Server, int)`. Count tools during registration.

## Testing Strategy

This feature is primarily a logging enhancement — most changes add structured fields to existing log calls or add new DEBUG-level log lines. Testing focuses on:

1. **Lock file PID write/read**: Unit test that `store.New()` writes PID to lock file and `detectLockHolder()` reads it back correctly. Uses `:memory:` store with temp dir for lock file.

2. **detectLockHolder stale detection**: Unit test that when no process holds the lock but a PID line exists, the function returns stale lock info.

3. **SetLogOutput/SetLogLevel for ignore**: Verify the ignore package logger respects level and output writer changes (mirrors existing vault/source tests if any).

4. **Existing test preservation**: All 43 MCP tools must continue to work. Logger prefix changes may affect tests that capture log output via `bytes.Buffer` — these need prefix updates.

5. **No timestamp assertions in tests**: Tests should not assert on timestamp values (non-deterministic). Tests that check log output should use `strings.Contains()` for the message portion, not exact line matching.

## Verification

```bash
# Build
go build ./...

# All tests pass
go test -race -count=1 ./...

# Static analysis
go vet ./...

# Manual verification: start dewey and check log output
dewey serve --verbose --vault /path/to/vault 2>&1 | head -20
# Expected: ISO 8601 timestamps, dewey/vault prefix, phase elapsed times, server ready line
```
