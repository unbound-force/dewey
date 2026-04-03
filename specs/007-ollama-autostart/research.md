# Research: Ollama Auto-Start

**Branch**: `007-ollama-autostart` | **Date**: 2026-04-03

## R1: Current Ollama Integration Points

### Where Ollama is checked today

**`main.go:initObsidianBackend()`** (lines 353-376):
The Obsidian backend initialization reads `DEWEY_EMBEDDING_ENDPOINT` and `DEWEY_EMBEDDING_MODEL` environment variables (defaulting to `http://localhost:11434` and `granite-embedding:30m`). When `--no-embeddings` is false, it creates an `OllamaEmbedder` and calls `embedder.Available()`. If the model is not available, it returns a hard error with instructions to `ollama pull` or use `--no-embeddings`.

**Problem**: The current code assumes Ollama is either running or not. It never attempts to start Ollama. If Ollama is installed but not running, the user gets a confusing error about the model not being available, when the real issue is that Ollama itself isn't running.

**`main.go:createIndexEmbedder()`** (lines 840-862):
Used by `dewey index` and `dewey reindex`. Same pattern — creates an embedder, checks `Available()`, returns hard error if unavailable.

**`cli.go:runDoctorChecks()`** (lines 1323-1377):
The Embedding Layer section checks Ollama reachability via `GET /api/tags` and reports PASS/FAIL. Currently does not distinguish between "Ollama not installed" and "Ollama not running".

**`embed/embed.go:OllamaEmbedder`**:
The `Available()` method calls `checkModelAvailable()` which does `GET /api/tags` and checks if the model name is in the response. The `checkModelAvailable()` method returns `false` for both "connection refused" (Ollama not running) and "model not in list" (Ollama running but model not pulled). These are distinct failure modes that the auto-start feature needs to distinguish.

### Key insight: health check vs. model check

The auto-start feature needs TWO checks:
1. **Health check** (`GET /api/tags` → 200): Is Ollama running?
2. **Model check** (model name in tags response): Is the embedding model pulled?

The existing `OllamaEmbedder.Available()` conflates both. The auto-start logic needs to separate them: if the health check fails (connection refused), try to start Ollama. If the health check passes but the model check fails, that's a different error (user needs to `ollama pull`).

## R2: Ollama State Machine

Three terminal states for the Ollama lifecycle:

```
                    ┌─────────────┐
                    │   Unknown   │
                    └──────┬──────┘
                           │
                    health check
                    GET /api/tags
                           │
              ┌────────────┼────────────┐
              │            │            │
         200 OK     conn refused   conn refused
              │            │            │
              ▼            │            │
        ┌──────────┐  ollama in    ollama NOT
        │ External │  PATH?        in PATH
        └──────────┘       │            │
                           ▼            ▼
                    ┌──────────┐  ┌─────────────┐
                    │ start    │  │ Unavailable  │
                    │ ollama   │  └─────────────┘
                    │ serve    │
                    └────┬─────┘
                         │
                    poll health
                    every 500ms
                    up to 30s
                         │
                    ┌────┴────┐
                    │         │
               200 OK    timeout
                    │         │
                    ▼         ▼
             ┌──────────┐  ┌─────────────┐
             │ Managed  │  │ Unavailable  │
             └──────────┘  │ (start fail) │
                           └─────────────┘
```

**State definitions**:
- **External**: Ollama was already running before Dewey started. Dewey uses it as-is. On Dewey exit, Ollama continues running (no action needed).
- **Managed**: Dewey started Ollama as a detached subprocess. On Dewey exit, Ollama continues running (detached process, no signal forwarding).
- **Unavailable**: Ollama is not installed (binary not in PATH) and not running at the endpoint. Dewey operates in keyword-only mode.

## R3: Subprocess Detachment Strategy

**Requirement**: The auto-started Ollama process must outlive Dewey (FR-004).

**Approach**: Use `os/exec.Command("ollama", "serve")` with `SysProcAttr` to prevent signal forwarding:

```go
cmd := exec.Command("ollama", "serve")
cmd.SysProcAttr = &syscall.SysProcAttr{
    Setpgid: true, // Create new process group — parent signals don't propagate
}
cmd.Stdout = nil // Discard stdout (Ollama logs to stderr)
cmd.Stderr = nil // Discard stderr (Ollama manages its own logs)
```

**Why `Setpgid: true`**: When the parent process (dewey) receives SIGINT or SIGTERM, the signal is sent to the process group. By putting the child in its own process group, it won't receive the parent's signals.

**Why not `Setsid: true`**: `Setsid` creates a new session, which is more aggressive than needed. `Setpgid` is sufficient to prevent signal propagation and is more portable.

**Why discard stdout/stderr**: Ollama manages its own logging. Mixing Ollama's output with Dewey's structured logging would create noise. The user can check Ollama's logs separately.

**Platform note**: `SysProcAttr.Setpgid` works on both macOS (darwin) and Linux. Windows is not a target platform for Dewey (per release workflow: darwin/linux only).

## R4: Health Check Polling

**Endpoint**: `GET /api/tags` at the configured endpoint (default `http://localhost:11434`).

**Why `/api/tags`**: This is the same endpoint used by `OllamaEmbedder.checkModelAvailable()`. A 200 response confirms Ollama is fully initialized and ready to serve requests. Using `/api/tags` instead of a simpler endpoint (like `/`) ensures the model registry is loaded.

**Polling parameters**:
- **Interval**: 500ms between checks
- **Timeout**: 30 seconds total (FR-010)
- **First check**: Immediately after starting the subprocess (no initial delay)

**Implementation**: Simple loop with `time.Ticker` and `time.After`:

```go
ticker := time.NewTicker(500 * time.Millisecond)
defer ticker.Stop()
timeout := time.After(30 * time.Second)

for {
    select {
    case <-timeout:
        return fmt.Errorf("ollama did not become ready within 30s")
    case <-ticker.C:
        if ollamaHealthCheck(endpoint) {
            return nil
        }
    }
}
```

## R5: Local Endpoint Detection

**Requirement**: Auto-start MUST only attempt to start a local subprocess when the endpoint is `localhost` or `127.0.0.1` (FR-008).

**Implementation**: Parse the endpoint URL and check the hostname:

```go
func isLocalEndpoint(endpoint string) bool {
    u, err := url.Parse(endpoint)
    if err != nil {
        return false
    }
    host := u.Hostname()
    return host == "localhost" || host == "127.0.0.1" || host == "::1"
}
```

**Why include `::1`**: IPv6 loopback. Unlikely but correct.

**Behavior for remote endpoints**: If the endpoint is not local (e.g., `http://gpu-server:11434`), Dewey only performs the health check. If the remote Ollama is not reachable, Dewey falls back to keyword-only mode with a log message. No subprocess is started.

## R6: Integration Points

### `initObsidianBackend()` modification

The auto-start logic inserts between the environment variable resolution and the `embedder.Available()` check:

```
Current flow:
  1. Read DEWEY_EMBEDDING_ENDPOINT, DEWEY_EMBEDDING_MODEL
  2. Create OllamaEmbedder
  3. Check embedder.Available() → hard error if false

New flow:
  1. Read DEWEY_EMBEDDING_ENDPOINT, DEWEY_EMBEDDING_MODEL
  2. ensureOllama(endpoint) → returns OllamaState
  3. If Unavailable → log info, skip embedder (keyword-only mode)
  4. If External or Managed → create OllamaEmbedder, check Available()
```

**Key change**: The hard error on `!embedder.Available()` becomes a soft fallback when Ollama is unavailable (not installed). The hard error is preserved when Ollama IS running but the model is not pulled — that's a fixable configuration issue, not a missing dependency.

### `createIndexEmbedder()` modification

Same pattern as `initObsidianBackend()`. The `dewey index` and `dewey reindex` commands should also attempt to start Ollama if it's not running. This ensures `dewey index` works without manually starting Ollama first.

### `runDoctorChecks()` modification

The Embedding Layer section currently reports "ollama running" or "ollama not reachable". With auto-start, it should report the Ollama state:
- **External**: "running (external)" — Ollama was already running
- **Managed**: "running (managed by dewey)" — Dewey started it
- **Unavailable**: "not available (not installed)" — binary not in PATH

Since `dewey doctor` is a diagnostic command (not `dewey serve`), it should NOT auto-start Ollama. It should only report the current state. The state detection logic (health check + binary lookup) is shared, but the auto-start action is serve-only.

## R7: `--no-embeddings` Bypass

When `--no-embeddings` is set (FR-007), the entire Ollama lifecycle is skipped:
- No health check
- No binary lookup
- No subprocess start
- No state logging beyond "embeddings disabled via --no-embeddings"

This is already the behavior in `initObsidianBackend()` (line 366-367). The auto-start logic is placed inside the `else` branch, so the bypass is automatic.

## R8: Error Handling Strategy

| Scenario | Behavior | Log Level |
|----------|----------|-----------|
| Ollama already running | Use it, log state | Info |
| Ollama started successfully | Use it, log state | Info |
| Ollama binary not found | Keyword-only mode | Info |
| Ollama binary found but start fails | Keyword-only mode, log error details | Warn |
| Ollama started but health check times out | Keyword-only mode, log timeout | Warn |
| Ollama running but model not pulled | Hard error with `ollama pull` instructions | Error (fatal) |
| Remote endpoint not reachable | Keyword-only mode | Info |

**Design decision**: "Ollama not installed" is Info, not Warn, because it's a valid configuration (the user may not want semantic search). "Start failed" and "timeout" are Warn because the user has Ollama installed and likely expects it to work.

## R9: Testability Analysis

### What can be tested without Ollama

1. **State machine logic**: Given health check result + binary availability → expected state. Pure function, no I/O.
2. **Health check HTTP**: Use `httptest.NewServer` to simulate Ollama responses (200, connection refused, timeout).
3. **Local endpoint detection**: Pure function on URL string.
4. **`--no-embeddings` bypass**: Verify `ensureOllama` is not called when flag is set.

### What requires integration testing

1. **Actual subprocess start**: Requires `ollama` binary in PATH. Guard with `testing.Short()` or skip if binary not available.
2. **Signal detachment**: Requires starting a real subprocess and killing the parent. This is an OS-level behavior that's well-tested by Go's `os/exec` package — we trust the stdlib.

### Test approach

- Unit tests for `isLocalEndpoint()`, `ollamaHealthCheck()` (via httptest), and `ensureOllama()` state transitions.
- The `ensureOllama()` function will accept an `ollamaStarter` interface parameter for dependency injection, allowing tests to mock the subprocess start without actually spawning processes.
- No integration tests that require a real Ollama binary — those are covered by the acceptance scenarios in manual testing.

## R10: Backward Compatibility

### Existing behavior preserved

- `--no-embeddings` flag: unchanged behavior
- `DEWEY_EMBEDDING_ENDPOINT` / `DEWEY_EMBEDDING_MODEL` env vars: unchanged
- All 37 MCP tools: unchanged input/output contracts
- `dewey doctor` output: enhanced (more detail) but same structure
- `dewey index` / `dewey reindex`: same behavior when Ollama is running; new auto-start when not running

### Behavior change (intentional)

- **Before**: `dewey serve` with Ollama not running → hard error
- **After**: `dewey serve` with Ollama not running → auto-start attempt → either success (full semantic search) or graceful fallback (keyword-only mode)

This is a strictly better user experience. The hard error is replaced with either success or a graceful degradation. No existing workflow is broken.
