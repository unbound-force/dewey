# Quickstart: Ollama Auto-Start

**Branch**: `007-ollama-autostart` | **Date**: 2026-04-03

## Overview

This feature adds Ollama lifecycle management to `dewey serve`, `dewey index`, and `dewey reindex`. When Ollama is installed but not running, Dewey starts it automatically. When Ollama is already running, Dewey uses it. When Ollama is not installed, Dewey falls back to keyword-only mode.

## New Types

### `OllamaState` (in `main.go`)

```go
// OllamaState represents the detected state of the Ollama server
// relative to the current Dewey process.
type OllamaState int

const (
    // OllamaExternal means Ollama was already running before Dewey started.
    // Dewey uses it as-is and does not manage its lifecycle.
    OllamaExternal OllamaState = iota

    // OllamaManaged means Dewey started Ollama as a detached subprocess.
    // The subprocess outlives Dewey — no cleanup on exit.
    OllamaManaged

    // OllamaUnavailable means Ollama is not installed and not running.
    // Dewey operates in keyword-only mode.
    OllamaUnavailable
)

// String returns a human-readable label for the Ollama state.
func (s OllamaState) String() string {
    switch s {
    case OllamaExternal:
        return "external"
    case OllamaManaged:
        return "managed"
    case OllamaUnavailable:
        return "unavailable"
    default:
        return "unknown"
    }
}
```

## New Functions

### `ensureOllama()` — State Machine Entry Point

```go
// ensureOllama detects the Ollama server state and optionally starts it.
// Returns the detected state. When autoStart is true and Ollama is not
// running but the binary is available at a local endpoint, starts
// `ollama serve` as a detached subprocess and waits for it to become ready.
//
// The starter parameter abstracts subprocess creation for testability.
// Pass nil to use the default os/exec implementation.
func ensureOllama(endpoint string, autoStart bool, starter ollamaStarter) (OllamaState, error)
```

**Flow**:
1. Call `ollamaHealthCheck(endpoint)` — if healthy, return `OllamaExternal`
2. If not healthy and `!autoStart`, check binary → return `OllamaUnavailable`
3. If not healthy and `autoStart`:
   a. Check `isLocalEndpoint(endpoint)` — if remote, return `OllamaUnavailable`
   b. Check `exec.LookPath("ollama")` — if not found, return `OllamaUnavailable`
   c. Start subprocess via `starter.Start()`
   d. Poll health check every 500ms for up to 30s
   e. If healthy within timeout, return `OllamaManaged`
   f. If timeout, return `OllamaUnavailable` with warning

### `ollamaHealthCheck()` — HTTP Health Probe

```go
// ollamaHealthCheck checks if Ollama is running at the given endpoint
// by making a GET request to /api/tags with a 2-second timeout.
// Returns true if the response status is 200 OK.
func ollamaHealthCheck(endpoint string) bool
```

### `isLocalEndpoint()` — Endpoint Locality Check

```go
// isLocalEndpoint returns true if the endpoint URL points to localhost,
// 127.0.0.1, or ::1. Auto-start only attempts to spawn a subprocess
// for local endpoints (FR-008).
func isLocalEndpoint(endpoint string) bool
```

### `ollamaStarter` — Testability Interface

```go
// ollamaStarter abstracts Ollama subprocess creation for testability.
// The default implementation uses os/exec; tests inject a mock.
type ollamaStarter interface {
    Start() error
}

// execOllamaStarter is the production implementation that starts
// `ollama serve` as a detached subprocess.
type execOllamaStarter struct{}

func (e *execOllamaStarter) Start() error {
    cmd := exec.Command("ollama", "serve")
    cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
    cmd.Stdout = nil
    cmd.Stderr = nil
    return cmd.Start()
}
```

## Modified Functions

### `initObsidianBackend()` in `main.go`

**Before** (lines 365-376):
```go
if noEmbeddings {
    logger.Info("embeddings disabled via --no-embeddings")
} else {
    embedder = embed.NewOllamaEmbedder(embedEndpoint, embedModel)
    if !embedder.Available() {
        return nil, nil, nil, fmt.Errorf("embedding model %q not available...")
    }
    // ...
}
```

**After**:
```go
if noEmbeddings {
    logger.Info("embeddings disabled via --no-embeddings")
} else {
    // Ensure Ollama is running (auto-start if needed).
    ollamaState, err := ensureOllama(embedEndpoint, true, &execOllamaStarter{})
    if err != nil {
        logger.Warn("ollama auto-start failed, continuing without embeddings", "err", err)
    }
    logger.Info("ollama state", "state", ollamaState, "endpoint", embedEndpoint)

    if ollamaState == OllamaUnavailable {
        logger.Info("semantic search unavailable — ollama not installed",
            "install", "brew install ollama")
    } else {
        embedder = embed.NewOllamaEmbedder(embedEndpoint, embedModel)
        if !embedder.Available() {
            return nil, nil, nil, fmt.Errorf("embedding model %q not available at %s\n\nTo fix:\n  ollama pull %s\n\nTo skip embeddings:\n  dewey serve --no-embeddings",
                embedModel, embedEndpoint, embedModel)
        }
        logger.Info("embedding model available", "model", embedModel)
        srvOpts = append(srvOpts, WithEmbedder(embedder))
    }
}
```

**Key change**: The hard error when Ollama is not running is replaced by auto-start. The hard error when the model is not pulled is preserved (that's a fixable config issue).

### `createIndexEmbedder()` in `cli.go`

Same pattern as `initObsidianBackend()` — call `ensureOllama()` before creating the embedder. When Ollama is unavailable, return `nil` (no embedder) instead of a hard error, matching the graceful degradation behavior.

### `runDoctorChecks()` in `cli.go`

The Embedding Layer section is enhanced to report the Ollama state. `dewey doctor` does NOT auto-start Ollama — it only reports the current state using the same detection logic (health check + binary lookup) but with `autoStart: false`.

## Integration Sequence

```
dewey serve
  │
  ├── resolveBackendType() → "obsidian"
  ├── initObsidianBackend()
  │     ├── resolveVaultPath()
  │     ├── open persistent store
  │     ├── read DEWEY_EMBEDDING_ENDPOINT, DEWEY_EMBEDDING_MODEL
  │     ├── if --no-embeddings → skip all Ollama logic
  │     ├── ensureOllama(endpoint, autoStart=true)
  │     │     ├── ollamaHealthCheck(endpoint)
  │     │     │     └── GET /api/tags → 200? → OllamaExternal
  │     │     ├── isLocalEndpoint(endpoint)?
  │     │     ├── exec.LookPath("ollama")?
  │     │     ├── starter.Start() → ollama serve (detached)
  │     │     └── poll health check → OllamaManaged | OllamaUnavailable
  │     ├── if OllamaUnavailable → keyword-only mode (no embedder)
  │     ├── if External|Managed → create OllamaEmbedder
  │     │     └── embedder.Available() → model check
  │     ├── create vault, index, watch
  │     └── return backend
  ├── newServer(backend)
  └── runServer(ctx, srv)
```

## Test Strategy

| Test | Type | What it verifies |
|------|------|-----------------|
| `TestOllamaHealthCheck_Healthy` | Unit (httptest) | Returns true when server responds 200 |
| `TestOllamaHealthCheck_Unreachable` | Unit | Returns false when connection refused |
| `TestIsLocalEndpoint_Localhost` | Unit | Returns true for localhost variants |
| `TestIsLocalEndpoint_Remote` | Unit | Returns false for non-local hosts |
| `TestEnsureOllama_AlreadyRunning` | Unit (httptest) | Returns OllamaExternal, no start attempt |
| `TestEnsureOllama_StartSuccess` | Unit (mock starter) | Returns OllamaManaged after mock start + health |
| `TestEnsureOllama_BinaryNotFound` | Unit | Returns OllamaUnavailable when LookPath fails |
| `TestEnsureOllama_RemoteEndpoint` | Unit | Returns OllamaUnavailable, no start attempt |
| `TestEnsureOllama_StartTimeout` | Unit (mock starter) | Returns OllamaUnavailable after timeout |
| `TestEnsureOllama_AutoStartDisabled` | Unit | Returns OllamaUnavailable when autoStart=false |
| `TestOllamaState_String` | Unit | String() returns expected labels |

**Coverage strategy**: All branches of the `ensureOllama()` state machine are covered. The `ollamaStarter` interface enables testing without real subprocess spawning. Health check tests use `httptest.NewServer`. No external dependencies required.
