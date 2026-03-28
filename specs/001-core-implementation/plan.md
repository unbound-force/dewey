# Implementation Plan: Dewey Core Implementation

**Branch**: `001-core-implementation` | **Date**: 2026-03-22 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/001-core-implementation/spec.md`

## Summary

Extend Dewey (a hard fork of graphthulhu) with persistent storage, vector-based semantic search, pluggable content sources, and CLI management commands. The persistence layer uses SQLite (pure-Go, no CGO) to store the knowledge graph index and vector embeddings in a single database (`.dewey/graph.db`). Semantic search uses locally-run Granite embedding models via Ollama's HTTP API. Content sources follow a pluggable interface supporting local disk (existing), GitHub API, and web crawl. The CLI is refactored from `flag.FlagSet` to cobra, and logging is migrated from `fmt.Fprintf` to `charmbracelet/log`, per convention pack compliance. All 37 existing MCP tools remain backward compatible.

## Technical Context

**Language/Version**: Go 1.25 (per `go.mod`)
**Primary Dependencies**: `modernc.org/sqlite` v1.47.0 (pure-Go SQLite), `github.com/k3a/html2text` v1.4.0 (HTML-to-text for web crawl), `github.com/modelcontextprotocol/go-sdk` v1.2.0 (existing MCP SDK), `github.com/spf13/cobra` (CLI framework), `github.com/charmbracelet/log` (structured logging)
**Storage**: SQLite via `modernc.org/sqlite` -- single database `.dewey/graph.db` containing the knowledge graph index (pages, blocks, links) and vector embeddings
**Testing**: `go test -race ./...` -- existing test suite (vault, tools, parser, graph, types packages) plus new tests for store, embedding, source, and CLI packages
**Target Platform**: macOS (darwin/arm64, darwin/amd64), Linux (linux/amd64, linux/arm64). Windows is not supported.
**Project Type**: MCP server + CLI tool
**Performance Goals**: <2s session startup with persisted index and <10 changed files; <100ms per MCP tool query; <5ms vector similarity search for <10k embeddings
**Constraints**: No CGO (pure Go only); no data leaves developer's machine; all state in `.dewey/` directory; backward compatible with all 37 existing MCP tools; GitHub tokens never logged or persisted; web crawl restricted to http/https with same-domain-only redirects
**Scale/Scope**: Hundreds to low thousands of documents per repository; <10k embedding vectors; 3-5 configured external sources typical

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

### I. Composability First -- PASS

Dewey remains independently installable. The `.dewey/` directory is optional -- without it, Dewey functions identically to graphthulhu (in-memory index only). Semantic search degrades gracefully when Ollama is unavailable. Content sources are optional and configured per-repository. No other Unbound Force tool is required.

### II. Autonomous Collaboration -- PASS

All new capabilities (semantic search, status reporting) are exposed exclusively through MCP tools. No runtime coupling, shared memory, or direct function calls. The 3 new MCP tools (`dewey_semantic_search`, `dewey_similar`, `dewey_semantic_search_filtered`) follow the same registration pattern as existing tools in `server.go`.

### III. Observable Quality -- PASS

Every search result includes provenance metadata including origin_url for external sources (FR-011). The `health` MCP tool and `dewey status` CLI command report index state, source freshness, and embedding coverage (FR-023). The `.dewey/` directory is inspectable at rest. Structured logging via charmbracelet/log provides observability.

### IV. Testability -- PASS

SQLite uses `modernc.org/sqlite` (pure Go, no external services). Embedding integration is testable via interface mocking -- the Ollama client is behind an `Embedder` interface. Content sources use fixture data and mock HTTP servers. CLI commands are testable via cobra's built-in testing support. All tests pass with `go test -race ./...` on a clean checkout.

**Pre-design gate result: ALL PASS.**

## Project Structure

### Documentation (this feature)

```text
specs/001-core-implementation/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── contracts/           # Phase 1 output (MCP tool schemas)
│   ├── mcp-tools.md     # New MCP tool contracts
│   └── cli-commands.md  # CLI command contracts
├── checklists/
│   └── requirements.md  # Spec quality checklist
└── tasks.md             # Phase 2 output (/speckit.tasks command)
```

### Source Code (repository root)

```text
# Existing packages (preserved, extended)
main.go                   # Entry point -- refactored to cobra root command
cli.go                    # CLI subcommands -- migrated to cobra commands
server.go                 # MCP server -- register 3 new semantic search tools
backend/backend.go        # Backend interface (unchanged)
vault/                    # Obsidian vault backend -- refactor to use store
tools/                    # MCP tool implementations -- add semantic search tools
types/                    # Shared types -- extend with embedding types
parser/                   # Content parser (unchanged)
graph/                    # Graph algorithms (unchanged)
client/                   # Logseq client (unchanged)

# New packages
store/                    # Persistence layer
├── store.go              # SQLite graph store (pages, blocks, links)
├── store_test.go         # Store tests with in-memory SQLite
├── embeddings.go         # Embedding storage and vector similarity
├── embeddings_test.go    # Embedding tests with fixture vectors
└── migrate.go            # Schema migration management

embed/                    # Embedding generation
├── embed.go              # Embedder interface + Ollama implementation
├── embed_test.go         # Tests with mock HTTP server
├── chunker.go            # Block-to-chunk preparation (heading context)
└── chunker_test.go       # Chunking tests

source/                   # Content source plugins
├── source.go             # Source interface definition
├── disk.go               # Local disk source (refactored from vault)
├── disk_test.go
├── github.go             # GitHub API source (token precedence chain)
├── github_test.go
├── web.go                # Web crawl source (URL validation, same-domain)
├── web_test.go
├── manager.go            # Source orchestration (refresh, failures)
├── manager_test.go
└── config.go             # Source configuration parsing
```

**Structure Decision**: Extend the existing flat package layout with 3 new packages (`store/`, `embed/`, `source/`). This follows the existing codebase pattern where each concern has its own package (vault, tools, graph, parser). No directory restructuring of existing code. Note: this deviates from convention pack AP-007 (which mandates `internal/`/`cmd/` layout) because the project inherits graphthulhu's flat structure. See `go-custom.md` for documented override.

## Complexity Tracking

| Complexity | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| CLI refactor from flag to cobra | Convention pack CS-009 compliance + better testability | Keeping flag-based CLI would violate MUST rule and miss testable CLI pattern benefits |
| Logging migration to charmbracelet/log | Convention pack CS-008 compliance + structured logging for observability | Keeping fmt.Fprintf would violate MUST rule and miss structured logging benefits |

Note: The flat package layout (AP-007 deviation) is retained because restructuring the entire codebase to `internal/`/`cmd/` is a separate concern from the core implementation. This deviation is documented in `go-custom.md`.

## Security Considerations

### GitHub Token Handling

- **Token precedence**: `GITHUB_TOKEN` or `GH_TOKEN` env var → `gh auth token` subprocess → unauthenticated (60 req/hr)
- Tokens MUST NOT be logged, persisted to `.dewey/`, or stored in any plaintext file
- Required scope: read-only (no write access needed)
- When `gh` CLI is not installed and no env var is set, operate in unauthenticated mode with warning

### Web Crawl Safety

- Restrict to `http://` and `https://` schemes only (reject `file://`, `ftp://`, etc.)
- Maximum response body size: 1MB per page
- Maximum pages per source: 100
- Follow redirects only within the same domain
- Respect `robots.txt` directives
- Enforce configurable rate limits (default: 1s between requests)

### SQL Injection Prevention

- All store operations MUST use parameterized queries (prepared statements)
- User-derived content (page names, issue titles, crawled page titles) is never interpolated into SQL strings

## Verification Strategy

1. **Backward compatibility**: Run existing `go test -race ./...` after every change to ensure all 37 MCP tools produce identical results.
2. **Persistence**: Test store package with in-memory SQLite. Test incremental update logic with fixture vaults.
3. **Vector search**: Test with pre-computed fixture embeddings (no Ollama needed). Test Ollama client against mock HTTP server.
4. **Content sources**: Test with fixture data. Test GitHub source with recorded HTTP responses. Test web crawl with local HTTP server serving test HTML.
5. **CLI**: Test cobra commands with temporary directories and fixture configurations.
6. **Integration**: End-to-end test: init → index → serve → query (semantic + keyword) → verify results.
7. **Performance**: Benchmark test for incremental startup (<2s for 200-file vault with <10 changes).
