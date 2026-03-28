# Implementation Plan: Unified Content Serve

**Branch**: `004-unified-content-serve` | **Date**: 2026-03-28 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/004-unified-content-serve/spec.md`

## Summary

Close the gap between `dewey index` (which fetches content from GitHub, web, and disk sources into `graph.db`) and `dewey serve` (which only queries local `.md` files). The approach: (1) upgrade `dewey index` to parse documents into blocks, links, and embeddings (not just page metadata), (2) teach the vault to load external-source pages from `graph.db` on startup alongside disk-parsed pages, and (3) add write guards so external content is read-only via MCP tools.

## Technical Context

**Language/Version**: Go 1.25 (per `go.mod`)
**Primary Dependencies**: `modernc.org/sqlite` (pure-Go SQLite), `github.com/modelcontextprotocol/go-sdk` (MCP), `github.com/spf13/cobra` (CLI), `github.com/charmbracelet/log` (logging), `github.com/k3a/html2text` (web crawl)
**Storage**: SQLite via `modernc.org/sqlite` — single database `.dewey/graph.db` containing pages, blocks, links, embeddings, sources, metadata tables
**Testing**: Standard library `testing` package, `-race -count=1`, in-memory SQLite for store tests, `httptest` for HTTP client tests, `t.TempDir()` for filesystem tests
**Target Platform**: macOS (darwin arm64/amd64), Linux (amd64/arm64) — CLI + MCP server
**Project Type**: CLI + MCP server (hybrid)
**Performance Goals**: Startup with 1,000 external pages adds no more than 2 seconds (SC-006). Write rejections under 100ms (SC-003).
**Constraints**: No CGO. No data leaves the developer's machine. All state in `.dewey/`. Backward compatibility with 37 graphthulhu MCP tools.
**Scale/Scope**: Designed for up to 5,000 external pages in-memory. Log warning above that threshold.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

### I. Composability First — PASS

Dewey remains independently installable. External content sources are optional configuration — `dewey serve` without `dewey index` continues to work identically to today (vault-only mode). No new hard dependencies on other Unbound Force tools. Ollama remains optional (graceful degradation for embeddings).

### II. Autonomous Collaboration — PASS

All indexed content (including external sources) is accessible exclusively through MCP tool calls with structured JSON responses. No runtime coupling, shared memory, or direct function calls introduced. The vault loads external pages into its existing in-memory index and serves them through the same `backend.Backend` interface.

### III. Observable Quality — PASS

External-source pages carry source provenance (source ID, source document ID, fetch timestamp). FR-010 requires `dewey status` to report per-source page counts. FR-014 requires structured logging at phase boundaries. The `health` tool continues to report index state including external page counts.

### IV. Testability — PASS

All new code is testable in isolation:
- Block parsing uses string-based functions (`parseMarkdownBlocks`, `parseFrontmatter`) — no disk I/O required for testing
- Store operations testable with in-memory SQLite (`:memory:`)
- Vault loading from store testable by pre-populating an in-memory store, then calling `LoadExternalPages()`
- Write guards testable by setting `readOnly` flag on a `cachedPage` and verifying error returns
- No new external service dependencies (Ollama embedding is optional, existing mock pattern applies)

**Coverage Strategy**: Each workstream requires tests exercising the contract. Test categories:
- Unit: block tree reconstruction, page namespace generation, write guard checks
- Integration: round-trip `persistPage → loadFromStore → queryViaMCP`, concurrent WAL access
- Regression: all 37 graphthulhu tools produce identical results with external pages loaded

### Upstream Stewardship — PASS

No changes to the 37 inherited MCP tool contracts. External pages are additive — they appear in results alongside local pages. An existing graphthulhu-compatible MCP client sees the same results for local vault queries (FR-011).

## Project Structure

### Documentation (this feature)

```text
specs/004-unified-content-serve/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
│   └── mcp-tools.md     # MCP tool contract changes
└── tasks.md             # Phase 2 output (/speckit.tasks)
```

### Source Code (repository root)

```text
main.go              # Startup orchestration — add LoadExternalPages() call after indexVault()
cli.go               # dewey index — upgrade indexDocuments() to persist blocks/links/embeddings
server.go            # MCP server setup — no changes (tools already use backend.Backend)
backend/             # Backend interface — no changes
vault/
├── vault.go         # Add sourceID/readOnly fields to cachedPage, add write guards
├── vault_store.go   # Implement LoadExternalPages(), block tree reconstruction
├── parse_export.go  # NEW: exported ParseDocument() wrapping parseMarkdownBlocks + parseFrontmatter
├── index.go         # No changes (buildBacklinks works on any cachedPage)
└── search_index.go  # No changes (BuildFrom works on any cachedPage)
store/
└── store.go         # Add ListPagesExcludingSource(), DeletePagesBySource(), ListPagesBySource()
tools/               # No changes — tools program against backend.Backend
source/              # No changes — source fetching already works
embed/               # No changes — embedder interface already exists
types/               # No changes
parser/              # No changes — parser.Parse() already extracts wikilinks
```

**Structure Decision**: Flat package layout preserved. Two new items: `vault/parse_export.go` (exported parsing entry point) and new store methods. All changes are within existing packages — no new packages created.

## Verification Strategy

### Quality Gates

New code introduced by this spec MUST meet the current CI-enforced Gaze thresholds (derived from `.github/workflows/ci.yml`):
- `--max-crapload=15` — no new function may exceed this CRAP score
- `--max-gaze-crapload=34` — aggregate GazeCRAPload must not regress
- Contract coverage: all new exported functions MUST have contract-level test assertions

Note: Spec 002 (Quality Ratchets) is partially complete — contract coverage is at 60.6% (target 80% not yet met). New code from this spec must not further depress contract coverage.

### Backward Compatibility Verification

FR-011 (37 inherited graphthulhu tools produce identical results for local vault content) will be verified by:
1. Running the full existing test suite (`go test -race -count=1 ./...`) with external pages loaded in the vault
2. All existing tool tests pass without modification — the tests exercise local vault content
3. Any test that begins failing after loading external pages indicates a backward compatibility regression

### Test Categories

- **Unit**: Block tree reconstruction (`reconstructBlockTree`), page namespace generation, write guard rejection, `ParseDocument` output correctness
- **Integration**: Round-trip `indexDocuments → LoadExternalPages → MCP tool query`, concurrent WAL access from separate goroutines
- **Regression**: All 37 graphthulhu tools + 3 semantic search tools produce correct results with external pages loaded alongside local pages

### Re-Index Strategy

When a document's content hash changes during `dewey index`, the system uses a **replace strategy**: delete all existing blocks, links, and embeddings for that page, then re-insert the newly parsed content. This ensures consistency and avoids stale block/link artifacts.

## Complexity Tracking

No constitution violations to justify. All changes follow existing patterns:
- Block parsing reuses existing `parseMarkdownBlocks` and `parseFrontmatter`
- Store operations follow existing `InsertBlock`/`InsertLink` patterns
- Write guards are simple boolean checks on `cachedPage.readOnly`
- WAL mode is already enabled (per research R1) — no code changes needed for FR-012
