# Implementation Plan: Ollama Auto-Start

**Branch**: `007-ollama-autostart` | **Date**: 2026-04-03 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/007-ollama-autostart/spec.md`

## Summary

When `dewey serve` starts and Ollama is installed but not running, Dewey will automatically start `ollama serve` as a detached subprocess, wait for it to become healthy, and then proceed with full semantic search capabilities. If Ollama is already running, Dewey uses it. If Ollama is not installed, Dewey falls back to keyword-only mode. The auto-started Ollama process is left running when Dewey exits.

The implementation is ~100 lines of Go in `main.go` plus tests, using an Ollama state machine (External / Managed / Unavailable) driven by health checks and binary detection.

## Technical Context

**Language/Version**: Go 1.25 (per `go.mod`)
**Primary Dependencies**: `os/exec` (subprocess), `net/http` (health check), `github.com/charmbracelet/log` (logging), `github.com/spf13/cobra` (CLI)
**Storage**: N/A (no storage changes)
**Testing**: `go test -race -count=1 ./...` with standard library `testing` package
**Target Platform**: macOS (darwin/amd64, darwin/arm64), Linux (linux/amd64, linux/arm64)
**Project Type**: CLI / MCP server
**Performance Goals**: Ollama health check polling at 500ms intervals; total startup timeout 30s; no measurable impact on `dewey serve` startup when Ollama is already running
**Constraints**: No CGO. No new dependencies. Subprocess must be detached (outlives parent). Local-only processing.
**Scale/Scope**: ~100 lines production code in `main.go`, ~150 lines test code in `main_test.go`

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

### Pre-Design Check

| Principle | Verdict | Rationale |
|-----------|---------|-----------|
| **I. Composability First** | PASS | Dewey continues to work without Ollama — auto-start is additive. When Ollama is unavailable, Dewey falls back to keyword-only mode (FR-006). The `--no-embeddings` flag bypasses the entire Ollama lifecycle (FR-007). No new external dependencies are introduced. |
| **II. Autonomous Collaboration** | PASS | No changes to MCP tool contracts. The auto-start logic is internal to `dewey serve` startup — agents interact via the same MCP tools. No runtime coupling or shared memory. |
| **III. Observable Quality** | PASS | Ollama state (External/Managed/Unavailable) is logged at startup. `dewey doctor` reports the state in the Embedding Layer section (FR-009). Provenance metadata on query results is unchanged. |
| **IV. Testability** | PASS | The Ollama health check is an HTTP call testable via `httptest`. Subprocess spawning is testable via `exec.LookPath` mocking (interface injection). The state machine is a pure function of health-check result + binary availability. No external services required for tests. |

**Gate result: PASS** — all four principles satisfied. Proceeding to Phase 0.

## Project Structure

### Documentation (this feature)

```text
specs/007-ollama-autostart/
├── plan.md              # This file
├── research.md          # Phase 0: codebase analysis and design decisions
├── quickstart.md        # Phase 1: implementation guide
└── tasks.md             # Phase 2 output (/speckit.tasks command)
```

### Source Code (repository root)

```text
main.go                  # OllamaState type, ensureOllama(), ollamaHealthCheck(),
                         #   startOllamaSubprocess(), isLocalEndpoint()
                         # Modified: initObsidianBackend(), createIndexEmbedder()
main_test.go             # Tests for ensureOllama state machine, health check,
                         #   endpoint detection, --no-embeddings bypass
cli.go                   # Modified: runDoctorChecks() Embedding Layer section
                         #   to report External/Managed/Unavailable state
embed/embed.go           # No changes (Embedder interface is sufficient)
```

**Structure Decision**: This feature modifies existing files only — no new packages or directories. The Ollama lifecycle management lives in `main.go` alongside the existing `initObsidianBackend()` and `createIndexEmbedder()` functions because it is serve-time orchestration, not reusable library code. Tests go in `main_test.go` alongside the existing test file.

### Post-Design Check

*Re-evaluated after Phase 1 design (research.md + quickstart.md).*

| Principle | Verdict | Post-Design Notes |
|-----------|---------|-------------------|
| **I. Composability First** | PASS | Confirmed: `ensureOllama()` returns `OllamaUnavailable` when Ollama is not installed, and `initObsidianBackend()` gracefully skips embedder creation. All 37 MCP tools continue to work in keyword-only mode. The `ollamaStarter` interface adds no external dependencies. |
| **II. Autonomous Collaboration** | PASS | Confirmed: No MCP tool contract changes. The `OllamaState` type is internal to `main.go` — not exposed via MCP tools or the `backend.Backend` interface. |
| **III. Observable Quality** | PASS | Confirmed: `ensureOllama()` logs the detected state at Info level. `dewey doctor` reports the state in the Embedding Layer section. The `OllamaState.String()` method provides human-readable labels for logging. |
| **IV. Testability** | PASS | Confirmed: The `ollamaStarter` interface enables mock injection for subprocess tests. `ollamaHealthCheck()` is testable via `httptest`. `isLocalEndpoint()` is a pure function. All 11 planned tests run without external services. Coverage strategy documented in quickstart.md. |

**Post-design gate result: PASS** — design is constitution-compliant.

## Complexity Tracking

> No constitution violations. No complexity justifications needed.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| *(none)* | — | — |
