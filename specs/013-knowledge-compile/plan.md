# Implementation Plan: Knowledge Compilation & Temporal Intelligence

**Branch**: `013-knowledge-compile` | **Date**: 2026-04-10 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/013-knowledge-compile/spec.md`

## Summary

Transform Dewey's flat learning store into an event-sourced knowledge system with temporal awareness, LLM-powered compilation, quality linting, and trust-tier contamination separation. Raw learnings are the append-only event log; compiled articles are the materialized view rebuilt from learnings. The `store_learning` API gains a required `tag` parameter and `{tag}-{sequence}` identity. Three new MCP tools (`compile`, `lint`, `promote`) and three new CLI commands provide the user-facing interface. Schema migration adds `tier` and `category` columns to the pages table (ISO 8601 `created_at` is derived from the existing Unix ms column at the tool response layer).

## Technical Context

**Language/Version**: Go 1.25 (per `go.mod`)
**Primary Dependencies**: `modernc.org/sqlite` (pure-Go SQLite), `github.com/modelcontextprotocol/go-sdk` (MCP SDK), `github.com/spf13/cobra` (CLI), `github.com/charmbracelet/log` (logging), `github.com/unbound-force/dewey/embed` (Ollama embeddings)
**Storage**: SQLite via `modernc.org/sqlite` — single database `.uf/dewey/graph.db` with schema migration from v1 → v2 (2 new columns on `pages` table: `tier`, `category`)
**Testing**: Standard library `testing` package, in-memory SQLite (`:memory:`), `httptest` for HTTP mocks
**Target Platform**: macOS/Linux (darwin/linux × amd64/arm64)
**Project Type**: CLI + MCP server
**Performance Goals**: `store_learning` < 1s (SC-001), `dewey compile` < 60s for 40 learnings (SC-003)
**Constraints**: No CGO, no data leaves developer machine, compiled articles are ephemeral (rebuilt from learnings)
**Scale/Scope**: ~6 new files, ~10 modified files, 3 new MCP tools, 3 new CLI commands

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

### I. Composability First — PASS

Dewey remains independently installable. The compile step uses a configurable LLM provider with graceful degradation: when no LLM is available, `dewey compile` returns a clear error but all other tools continue working. The `store_learning` API change is backward-compatible via default tag `"general"` when `tag` is omitted. No new external dependencies are introduced.

### II. Autonomous Collaboration — PASS

All new capabilities are exposed as MCP tools (`compile`, `lint`, `promote`) with documented input schemas and structured JSON responses. The `/unleash` integration is a documentation change to the agent instruction file, not a runtime coupling. The compile tool delegates LLM synthesis through a well-defined interface, not through shared memory or direct function calls.

### III. Observable Quality — PASS

Every learning gets `created_at` timestamp and `{tag}-{sequence}` identity for provenance. Semantic search results include `created_at`, `category`, and `tier` metadata. Compiled articles include history sections attributing each fact to its source learning. The `lint` tool provides auditable quality metrics (stale decisions, uncompiled learnings, embedding gaps). The `health` tool continues to report index state.

### IV. Testability — PASS

All new packages are testable in isolation:
- Schema migration tested with in-memory SQLite
- `store_learning` changes tested with existing mock patterns
- Compile tool tested with injected LLM interface (mock for tests)
- Lint tool tested with pre-populated store fixtures
- Promote tool tested with in-memory store
- No external services required for tests (LLM interface mocked)

**Coverage strategy**: Contract-level tests for all new public functions. Existing tests must continue passing. CRAP score targets: < 48 (CI threshold). The LLM synthesis interface is mocked in tests — no Ollama or external model required.

## Project Structure

### Documentation (this feature)

```text
specs/013-knowledge-compile/
├── plan.md              # This file
├── research.md          # Phase 0: codebase analysis, design decisions
├── quickstart.md        # Phase 1: architecture overview, key decisions
├── contracts/           # Phase 1: API contracts for new/modified interfaces
│   ├── store-learning.md    # Modified store_learning MCP tool contract
│   ├── compile-tool.md      # New compile MCP tool contract
│   ├── lint-tool.md         # New lint MCP tool contract
│   ├── promote-tool.md      # New promote MCP tool contract
│   ├── schema-migration.md  # Schema v1 → v2 migration contract
│   └── llm-interface.md     # LLM synthesis interface contract
├── checklists/
│   └── requirements.md  # Requirement quality checklist
└── tasks.md             # Phase 2 output (/speckit.tasks command)
```

### Source Code (repository root)

```text
store/
├── store.go             # MODIFY: Add Tier, Category, CreatedAtISO fields to Page struct
├── migrate.go           # MODIFY: Schema v1 → v2 migration (3 new columns)
└── embeddings.go        # MODIFY: Add Tier field to SimilarityResult, tier filter to SearchFilters

tools/
├── learning.go          # MODIFY: tag/category params, {tag}-{sequence} identity, tier/created_at
├── learning_test.go     # MODIFY: Update tests for new API
├── semantic.go          # MODIFY: Add created_at + tier to search result metadata
├── semantic_test.go     # MODIFY: Update tests for new metadata fields
├── compile.go           # NEW: Compile MCP tool handler
├── compile_test.go      # NEW: Tests for compile tool
├── lint.go              # NEW: Lint MCP tool handler
├── lint_test.go         # NEW: Tests for lint tool
├── promote.go           # NEW: Promote MCP tool handler
└── promote_test.go      # NEW: Tests for promote tool

types/
└── tools.go             # MODIFY: Update StoreLearningInput, add CompileInput, LintInput, PromoteInput

llm/
├── llm.go               # NEW: LLM synthesis interface + Ollama implementation
└── llm_test.go          # NEW: Tests for LLM interface

server.go                # MODIFY: Register 3 new tools (compile, lint, promote)
cli.go                   # MODIFY: Add dewey compile, dewey lint, dewey promote commands
main.go                  # MODIFY: Wire LLM provider from config
```

**Structure Decision**: Follows existing flat package layout. New `llm/` package for the LLM synthesis interface (separate from `embed/` which is embedding-only). All tool handlers in `tools/` following established patterns. No new top-level directories beyond `llm/`.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| New `llm/` package | LLM synthesis is a distinct concern from embedding generation (`embed/`). Embedding produces vectors; synthesis produces natural language text. Mixing them violates Single Responsibility. | Putting synthesis in `embed/` would conflate two different Ollama API endpoints (`/api/embed` vs `/api/generate`) with different request/response shapes, error modes, and configuration. |
| `store_learning` breaking API change | The spec requires `tag` (singular, required) replacing `tags` (plural, optional). This is a deliberate breaking change per FR-001. | Keeping `tags` and adding `tag` as an alias would create confusion about which field to use and complicate the `{tag}-{sequence}` identity generation. |
<!-- scaffolded by unbound vdev -->
