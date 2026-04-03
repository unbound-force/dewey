# Implementation Plan: Store Learning MCP Tool

**Branch**: `008-store-learning` | **Date**: 2026-04-03 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/008-store-learning/spec.md`

## Summary

Add a `dewey_store_learning` MCP tool (tool #41) that allows AI agents to persist natural language learnings with optional tags into Dewey's existing SQLite store. Learnings are stored as pages with `source_id = "learning"`, making them immediately searchable via the existing `dewey_semantic_search` and `dewey_semantic_search_filtered` tools. No schema changes are needed — the feature reuses the existing page/block/embedding pipeline with a new source type convention.

## Technical Context

**Language/Version**: Go 1.25 (per `go.mod`)
**Primary Dependencies**: `github.com/modelcontextprotocol/go-sdk/mcp` (MCP SDK), `modernc.org/sqlite` (pure-Go SQLite), `github.com/charmbracelet/log` (logging)
**Storage**: SQLite via `modernc.org/sqlite` — single database `.dewey/graph.db` (existing, no schema changes)
**Testing**: Standard library `testing` package with in-memory SQLite (`:memory:`) and mock embedder
**Target Platform**: darwin/linux x amd64/arm64 (cross-platform CLI/MCP server)
**Project Type**: CLI + MCP server
**Performance Goals**: Round-trip store-and-retrieve under 2 seconds (SC-001)
**Constraints**: No CGO, no external services required for core functionality, local-only processing
**Scale/Scope**: Single new MCP tool, 4 files modified/created

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

### I. Composability First — PASS

The `dewey_store_learning` tool is registered alongside the existing 40 MCP tools. It requires the persistent store (`store.Store`) to function — when the store is nil (in-memory mode), the tool returns a clear error message. This matches the existing pattern used by `dewey_semantic_search` and other store-dependent tools. Dewey remains independently installable; the learning tool is an additive capability that does not create new external dependencies.

### II. Autonomous Collaboration — PASS

The tool communicates exclusively through the MCP tool call interface with a structured JSON input schema (`information` + `tags`) and a structured JSON response (UUID on success, error message on failure). No runtime coupling, shared memory, or direct function calls. Agents discover the tool through the MCP tool registry.

### III. Observable Quality — PASS

Stored learnings include provenance metadata: `source_id = "learning"` distinguishes them from disk/GitHub/web sources. The `inferSourceType()` function in `store/embeddings.go` will extract "learning" from the source ID, making it visible in search results via the `source` field. The `health` tool will automatically report learning source statistics since it iterates `ListSources()`.

### IV. Testability — PASS

The tool implementation follows the existing `tools/semantic.go` pattern: dependencies (`embed.Embedder`, `store.Store`) are injected via constructor. Tests use in-memory SQLite (`:memory:`) and the existing `mockEmbedder` from `tools/semantic_test.go`. No external services required. Coverage strategy: test the happy path (store + retrieve), error paths (nil store, nil embedder, empty input), and the Ollama-unavailable graceful degradation path.

**Coverage Strategy**: Contract-level tests covering:
- Happy path: store learning, verify page/block/embedding created, verify searchable
- Error: empty `information` parameter
- Error: nil store (in-memory mode)
- Error: nil embedder (graceful degradation — store without embedding)
- Graceful degradation: embedder unavailable (store text, skip embedding, return informational message)
- Tags: verify tags stored as page properties and filterable via `has_tag`

## Project Structure

### Documentation (this feature)

```text
specs/008-store-learning/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── quickstart.md        # Phase 1 output
└── tasks.md             # Phase 2 output (/speckit.tasks command)
```

### Source Code (repository root)

```text
types/tools.go           # ADD StoreLearningInput struct (alongside existing input types)
tools/learning.go        # NEW — Learning tool handler (store learning implementation)
tools/learning_test.go   # NEW — Tests for learning tool
server.go                # ADD registerLearningTools() call + function
```

**Structure Decision**: This feature adds a single MCP tool following the established flat package layout. The `tools/learning.go` file follows the same pattern as `tools/semantic.go` — a struct with injected dependencies and handler methods. No new packages are needed. The input type goes in `types/tools.go` alongside all other tool input types.

## Complexity Tracking

No constitution violations. The implementation reuses existing infrastructure (store, embedder, page/block model) with no new abstractions or dependencies.
