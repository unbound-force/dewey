# Implementation Plan: Code Source Indexing & Manifest Generation

**Branch**: `010-code-source-index` | **Date**: 2026-04-06 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `specs/010-code-source-index/spec.md`

## Summary

Add a new `type: code` content source that indexes Go source code using language-aware AST chunking, and a `dewey manifest` CLI command that generates a project interface summary. The code source uses `go/parser` + `go/ast` (stdlib) to extract exported declarations, Cobra CLI commands, and MCP tool registrations as searchable blocks. A new `chunker/` package provides the pluggable language-aware parsing interface, with Go as the initial implementation. The code source integrates with the existing indexing pipeline — chunked declarations are formatted as markdown documents, flowing through `vault.ParseDocument()` → `PersistBlocks()` → `GenerateEmbeddings()` unchanged.

## Technical Context

**Language/Version**: Go 1.25 (per `go.mod`)
**Primary Dependencies**: `go/parser`, `go/ast`, `go/token`, `go/format` (all stdlib); existing `source`, `store`, `vault`, `embed`, `ignore` packages
**Storage**: SQLite via `modernc.org/sqlite` — same `.dewey/graph.db` database, no schema changes
**Testing**: Standard library `testing` package with `t.TempDir()` fixtures, in-memory SQLite for store tests
**Target Platform**: darwin/linux (amd64/arm64)
**Project Type**: CLI + MCP server
**Performance Goals**: Index a 100-file Go repo in <10 seconds excluding embedding generation (SC-004)
**Constraints**: No CGO, no external dependencies beyond stdlib for AST parsing, local-only processing
**Scale/Scope**: New `chunker/` package (3 files), new `source/code.go`, modifications to `source/config.go`, `source/manager.go`, `cli.go`

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

### I. Composability First — ✅ PASS

Code source indexing is an optional feature. Dewey continues to work without any code sources configured. The `dewey manifest` command works standalone — it doesn't require `dewey serve` or any other tool. The chunker package has zero external dependencies (stdlib only). No new hard dependencies are introduced.

### II. Autonomous Collaboration — ✅ PASS

Code source content is indexed into the same knowledge graph as all other sources. Agents discover code declarations through the existing MCP tools (`semantic_search`, `search`, `get_page`) — no new MCP tools are needed. The code source communicates through the standard `Source` interface, not through internal APIs.

### III. Observable Quality — ✅ PASS

Indexed code blocks include provenance metadata (source ID, source type "code", fetch timestamp). The `dewey status` command reports code source page counts and last-fetched times. The `dewey doctor` command validates code source configuration. Manifest files include generation timestamps. All existing quality reporting continues to work.

### IV. Testability — ✅ PASS

- `chunker/` package: Testable with string inputs (Go source code literals) — no filesystem, no network, no external services
- `source/code.go`: Testable with `t.TempDir()` fixtures containing `.go` files — same pattern as `source/disk_test.go`
- `cli.go` manifest command: Testable with `t.TempDir()` fixtures — same pattern as existing CLI tests
- All tests pass with `go test ./...` on a clean checkout
- Coverage strategy: Contract tests for chunker output (block count, heading format, content inclusion), integration tests for code source → document pipeline

**Post-Phase 1 Re-check**: All four principles remain PASS. The design uses stdlib-only AST parsing (Composability), standard Source interface (Autonomous Collaboration), existing provenance pipeline (Observable Quality), and fixture-based testing (Testability).

## Project Structure

### Documentation (this feature)

```text
specs/010-code-source-index/
├── plan.md                          # This file
├── spec.md                          # Feature specification
├── research.md                      # Phase 0 research findings
├── quickstart.md                    # Phase 1 quickstart guide
├── contracts/                       # Phase 1 interface contracts
│   ├── chunker-interface.md         # Chunker interface + Block type + Registry
│   ├── go-chunker.md                # Go language chunker behavior
│   ├── code-source.md               # CodeSource implementation
│   ├── config-validation.md         # Config validation for type: code
│   └── manifest-command.md          # dewey manifest CLI command
├── checklists/                      # Requirement quality checklists
└── tasks.md                         # Task breakdown (created by /speckit.tasks)
```

### Source Code (repository root)

```text
chunker/                    # NEW: Language-aware code chunking package
├── chunker.go              # Chunker interface, Block type, Registry
├── go_chunker.go           # Go implementation (go/parser + go/ast)
└── go_chunker_test.go      # Contract tests for Go chunker

source/
├── code.go                 # NEW: CodeSource implementing Source interface
├── code_test.go            # NEW: CodeSource tests with t.TempDir() fixtures
├── config.go               # MODIFIED: Add type: code validation
├── config_test.go          # MODIFIED: Add code source validation tests
├── manager.go              # MODIFIED: Add case "code" to createSource()
├── manager_test.go         # MODIFIED: Add code source manager tests
├── source.go               # UNCHANGED
├── disk.go                 # UNCHANGED (reference implementation)
├── github.go               # UNCHANGED
└── web.go                  # UNCHANGED

cli.go                      # MODIFIED: Add newManifestCmd()
cli_test.go                 # MODIFIED: Add manifest command tests
AGENTS.md                   # MODIFIED: Document new source type and command
```

**Structure Decision**: Flat package layout consistent with existing architecture. The new `chunker/` package is the only new directory — it's a separate package because it has a distinct responsibility (language-aware parsing) and will grow with additional language implementations. The `CodeSource` lives in the existing `source/` package alongside `DiskSource`, `GitHubSource`, and `WebSource`.

## Implementation Phases

### Phase 1: Chunker Package (Foundation)

Create the `chunker/` package with the interface, Go implementation, and registry. This is the foundation — everything else depends on it.

**Files**: `chunker/chunker.go`, `chunker/go_chunker.go`, `chunker/go_chunker_test.go`

**Key decisions**:
- `Block` type is chunker-specific (not `types.BlockEntity`) to avoid coupling the chunker to the indexing pipeline
- Registry uses `init()` registration pattern for automatic language registration
- Go chunker uses `go/parser.ParseFile()` with `parser.ParseComments` flag
- Function signatures extracted by formatting `ast.FuncDecl` without `Body` field
- Cobra detection walks `ast.Inspect` looking for `ast.CompositeLit` with `cobra.Command` type
- MCP detection walks `ast.Inspect` looking for `ast.CallExpr` with `mcp.AddTool` selector

**Test strategy**: Table-driven tests with Go source code string literals as input. Test each block kind independently. Test error cases (syntax errors, empty files, no exports).

### Phase 2: Code Source (Integration)

Create `CodeSource` in `source/code.go` and integrate with config validation and manager.

**Files**: `source/code.go`, `source/code_test.go`, `source/config.go`, `source/manager.go`

**Key decisions**:
- `CodeSource.List()` formats chunker blocks as markdown documents — one document per source file
- Document content uses markdown headings (`## func Name`) for each declaration
- Reuses `DiskSource` patterns: ignore matcher, recursive walk, hash-based change detection
- Config validation requires `path` and `languages` fields
- Manager `createSource()` gets a new `case "code":` branch

**Test strategy**: `t.TempDir()` fixtures with `.go` files. Test List() output (document count, content format). Test config validation (valid, missing fields). Test manager integration (createSource returns CodeSource).

### Phase 3: Manifest Command (CLI)

Add `dewey manifest` CLI command that uses the chunker to generate `.dewey/manifest.md`.

**Files**: `cli.go`, `cli_test.go`

**Key decisions**:
- Manifest command is standalone — doesn't require `dewey init` or a running server
- Creates `.dewey/` directory if needed
- Uses the same Go chunker as the code source (FR-011)
- Output is plain markdown with tables for commands/tools and sections for packages
- Idempotent — running twice produces the same output

**Test strategy**: `t.TempDir()` with sample `.go` files containing Cobra commands and MCP tools. Verify manifest content includes expected sections and entries.

### Phase 4: Documentation & Agent Context

Update AGENTS.md with new source type, command, and package documentation.

**Files**: `AGENTS.md`

## Coverage Strategy

| Package | Target | Approach |
|---------|--------|----------|
| `chunker/` | ≥80% | Contract tests: each block kind, error cases, edge cases |
| `source/code.go` | ≥70% | Integration tests: List(), Fetch(), Diff(), Meta() |
| `source/config.go` (additions) | ≥90% | Unit tests: validation for type: code |
| `cli.go` (manifest) | ≥70% | Integration tests: manifest generation with fixtures |

**CRAP score target**: All new functions < 30 CRAPload. The Go chunker's `ChunkFile` method is the highest-risk function — decompose into per-declaration-type extractors to keep complexity manageable.

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Go AST API complexity | Medium | Medium | Prototype chunker first, validate with Dewey's own source code |
| Cobra detection false positives | Low | Low | Only match `cobra.Command` composite literals, not arbitrary structs |
| Large file performance | Low | Medium | AST parsing is O(n) in file size; each declaration is independent |
| Embedding quality for code | Medium | Medium | Doc-comment-first content format; natural language embeds well |
| Chunker interface too rigid | Low | High | Keep interface minimal (4 methods); extend via options, not interface changes |

## Complexity Tracking

> No constitution violations requiring justification. All new code uses stdlib dependencies, follows existing patterns, and introduces no architectural complexity beyond the new `chunker/` package.

## Dependencies

```text
Phase 1 (chunker/) → Phase 2 (source/code.go) → Phase 3 (manifest command)
                                                → Phase 4 (documentation)
```

Phase 1 has no dependencies on existing code changes. Phase 2 depends on Phase 1 (imports `chunker/`). Phase 3 depends on Phase 1 (imports `chunker/`) but is independent of Phase 2. Phase 4 depends on all prior phases being complete.
