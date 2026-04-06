# Research: Structured Diagnostics

**Branch**: `009-structured-diagnostics` | **Date**: 2026-04-06

## R1: charmbracelet/log Timestamp Configuration

The `log.Options` struct supports `ReportTimestamp: true` and `TimeFormat: string`. Setting `TimeFormat: "2006-01-02T15:04:05.000Z07:00"` produces ISO 8601 with millisecond precision. This applies to `log.NewWithOptions()` — all four logger declarations use this constructor already.

**Finding**: No new dependencies needed. The existing `log.Options` struct already supports timestamps. The `Prefix` field already exists and is set to `"dewey"` in all four declarations — changing it to `"dewey/vault"`, `"dewey/source"`, `"dewey/ignore"` is a one-line change per declaration.

**Impact on SetLogOutput**: The `vault.SetLogOutput()` and `source.SetLogOutput()` functions create new loggers with `log.NewWithOptions()`. These must also include `ReportTimestamp: true` and the ISO 8601 `TimeFormat`, plus the correct per-package prefix. The `ignore` package needs a new `SetLogOutput()` function matching the vault/source pattern.

## R2: Phase Timing with time.Since()

Go's `time.Now()` + `time.Since(start)` is the idiomatic way to measure elapsed time. The `time.Duration` type formats as `"1.234s"` by default, which is human-readable. Using `elapsed.Round(time.Millisecond)` gives clean output like `"28.312s"`.

**Finding**: The `initObsidianBackend()` function already has sequential phases (store open, Ollama check, index, external pages, watcher). Each phase has an existing `logger.Info()` call at completion. Adding `"elapsed", time.Since(phaseStart)` as a structured field requires only:
1. `phaseStart := time.Now()` before each phase
2. `"elapsed", time.Since(phaseStart).Round(time.Millisecond)` added to the existing log call

The total startup time is measured from the top of `executeServe()` to just before `runServer()`.

## R3: Lock File PID Format

The lock file at `.dewey/.dewey.lock` is created by `store.New()` using `os.OpenFile()` + `syscall.Flock()`. Currently, the file is empty — it's used only for the advisory lock.

**Finding**: Writing `fmt.Fprintf(lockFile, "%d %s\n", os.Getpid(), strings.Join(os.Args, " "))` after acquiring the lock is safe because:
- The file is opened with `O_RDWR` (already)
- The write happens after `Flock()` succeeds, so no race
- The content is small (< 1 KB) — no buffering concerns
- On next acquisition, the file is truncated implicitly by seeking to 0 and writing

**Reading the PID**: `detectLockHolder()` in `cli.go` already opens the lock file. Adding `io.ReadAll()` + `strings.SplitN(line, " ", 2)` extracts the PID and command. The function currently returns `"lock file ... is held (try: lsof ...)"` — this changes to `"PID 12345 (dewey serve --vault /path)"`.

**Stale lock detection**: When `Flock()` succeeds (no holder), but the file contains a PID line, the lock is stale. `detectLockHolder()` can return this information for `dewey doctor`.

## R4: Server Ready Marker

The "server ready" log line must appear after all initialization but before `srv.Run()`. In `executeServe()`, this is between `initObsidianBackend()` return and `runServer()` call (line 233-239).

**Finding**: The tool count is available from `newServer()` but not currently exposed. The `mcp.Server` type from `go-sdk` has a `ListTools()` method that returns registered tools. We can call `srv.ListTools()` to get the count.

**Alternative**: Count tools at registration time in `newServer()` by tracking additions. However, `ListTools()` is cleaner and doesn't require modifying the registration flow.

**Verification needed**: Check if `mcp.Server` exposes `ListTools()` or equivalent. If not, we count tools manually in `newServer()` and return the count alongside the server.

## R5: ignore Package Wiring

The `ignore` package already has `SetLogLevel()` but lacks `SetLogOutput()`. The vault and source packages both follow the same pattern:

```go
func SetLogOutput(w io.Writer, level log.Level) {
    newLogger := log.NewWithOptions(w, log.Options{Prefix: "dewey/ignore", Level: level})
    *logger = *newLogger
}
```

**Wiring points**:
1. `setupFileLogging()` in `main.go`: Add `ignore.SetLogOutput(multi, level)` after vault and source calls
2. `PersistentPreRunE` in `main.go`: Add `ignore.SetLogLevel(log.DebugLevel)` in the verbose block

## R6: Silent File Skip Locations

Seven locations silently discard filesystem errors with `return nil`:

| # | File | Line | Context |
|---|------|------|---------|
| 1 | `vault/vault.go` | 169 | `Load()` walk error |
| 2 | `vault/vault.go` | 191 | `Load()` ReadFile error |
| 3 | `vault/vault_store.go` | 227 | `walkVault()` walk error |
| 4 | `vault/vault_store.go` | 244 | `walkVault()` ReadFile error |
| 5 | `source/disk.go` | 111 | `List()` walk error |
| 6 | `source/disk.go` | 136 | `List()` ReadFile error |
| 7 | `source/disk.go` | ~170 | `Diff()` walk error (to verify) |

Each needs a `logger.Debug("skipping file", "path", path, "err", walkErr)` or equivalent.

## R7: Ollama Polling Progress

The polling loop in `ensureOllama()` (main.go:384-396) currently sleeps 500ms per iteration with no logging. Adding a progress log every 5 seconds requires tracking elapsed time:

```go
start := time.Now()
lastLog := start
for time.Now().Before(deadline) {
    time.Sleep(pollInterval)
    if ollamaHealthCheck(endpoint) { ... }
    if time.Since(lastLog) >= 5*time.Second {
        logger.Debug("waiting for Ollama", "elapsed", time.Since(start).Round(time.Millisecond), "timeout", maxWait)
        lastLog = time.Now()
    }
}
```

## R8: Shutdown Logging

The stdio transport path in `runServer()` (main.go:640-645) has no shutdown log. The HTTP path already has `logger.Info("shutting down HTTP server")`. Adding `logger.Info("server stopped")` after `srv.Run()` returns covers the stdio path.

## R9: Tool Count from mcp.Server

The `mcp.Server` from `go-sdk` v1.2.0 does not expose a public `ListTools()` method that returns a count directly. However, `newServer()` in `server.go` registers all tools — we can count them by adding a counter or by returning the count from `newServer()`.

**Decision**: Modify `newServer()` to return `(*mcp.Server, int)` where the int is the tool count. This is cleaner than reflection or post-hoc counting. The count is incremented in each `registerXxxTools()` function or tracked via a wrapper.

**Simpler alternative**: Since `newServer()` already calls `mcp.AddTool()` and `srv.AddTool()`, we can count the calls statically. Current count: 43 tools (from `grep -c` above). But this is fragile — better to count dynamically.

**Simplest alternative**: Pass the count as a return value from `newServer()` by counting `AddTool` calls within it. Each `registerXxxTools` function returns the number of tools it registered.
