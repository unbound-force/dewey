# Research: Dewey Core Implementation

**Branch**: `001-core-implementation` | **Date**: 2026-03-22

## Research Summary

All Technical Context items are resolved. Five research areas were investigated to inform the implementation plan.

## Decision 1: SQLite Library (Pure-Go)

**Decision**: Use `modernc.org/sqlite` v1.47.0.

**Rationale**: This is the most mature pure-Go SQLite implementation (2,562 importers, v1 stable API, Tailscale-backed). It transpiles the entire SQLite C source to Go via `ccgo`, passing SQLite's own test suite. Supports virtual tables (for future extensibility), custom functions (for cosine similarity if needed), WAL journal mode, and foreign keys. No CGO required, satisfying the constitution's mandate.

**Alternatives considered**:
- `mattn/go-sqlite3`: Rejected because it requires CGO. Constitution line 145-146: "CGO dependencies MUST be avoided unless no pure-Go alternative exists." Since `modernc.org/sqlite` exists and is mature, CGO is not justified.
- `github.com/ncruces/go-sqlite3` v0.33.0: Pure-Go via WebAssembly transpilation. Pre-v1, smaller community (88 importers). Good technology but higher risk for a foundational dependency. Could revisit if `modernc.org/sqlite` presents issues.

## Decision 2: Vector Search Strategy

**Decision**: Brute-force cosine similarity computed in Go code. Store embeddings as BLOBs in SQLite.

**Rationale**: Dewey indexes hundreds to low thousands of documents, producing <10k embedding vectors. At 384 dimensions (granite-embedding:30m), brute-force cosine similarity over 10k vectors takes ~5ms in Go -- well under the <100ms query budget. This approach requires zero additional dependencies, keeps the architecture simple, and is trivially testable with fixture data.

**Alternatives considered**:
- `sqlite-vec` extension: Excellent technology (7.3k GitHub stars) but Go bindings require CGO (`mattn/go-sqlite3`). Cannot be loaded via `modernc.org/sqlite` without reimplementing the extension logic as a Go virtual table. Over-engineered for <10k vectors.
- `sqlite-vss`: Deprecated by its author in favor of `sqlite-vec`. Same CGO constraint.
- Separate Go vector index (HNSW, vek): Adds architectural complexity for marginal performance gain at our scale. Could be introduced behind the same interface if scale exceeds 100k vectors.

## Decision 3: Embedding API (Ollama)

**Decision**: Use Ollama's HTTP API at `POST http://localhost:11434/api/embed`.

**Rationale**: Ollama is already referenced in the design paper and constitution as the embedding provider. The `/api/embed` endpoint accepts a model name and text input, returns float64 embedding vectors. Supports batch embedding (array of strings). No SDK dependency needed -- standard `net/http` calls.

**Key API details**:
- Embed: `POST /api/embed` with `{"model": "granite-embedding:30m", "input": "text"}`
- Response: `{"embeddings": [[0.123, -0.456, ...]], "total_duration": 123456789}`
- Model check: `GET /api/tags` returns list of available models
- Error patterns: connection refused (Ollama not running), model not found (need `ollama pull`), timeout (model loading on first request)

**Alternatives considered**: None. The design paper and constitution specify Ollama. An `Embedder` interface allows swapping the backend in the future without changing callers.

## Decision 4: Chunking Strategy

**Decision**: Block-level embedding. Each heading section (H1-H6 delimited block) = one embedding chunk.

**Rationale**: The existing `vault/markdown.go` already parses Markdown into heading-based `BlockEntity` structures. These are the natural semantic units: a heading section like "## Installation" with its body content is typically 50-500 tokens, fitting within the Granite model's 512-token context window. Each block has a stable UUID, enabling incremental re-embedding when content changes. For embedding context, the heading hierarchy path is prepended (e.g., "setup.md > Installation > From Source\n\n...").

**Alternatives considered**:
- Page-level embedding: Rejected because most pages exceed the 512-token context window, causing truncation and information loss. Also produces less precise search results.
- Hybrid (both page and block level): Rejected as unnecessary duplication. If block-level search returns a hit, the page context is trivially available from metadata. Can be added later if needed.
- Fixed-size overlapping windows: Rejected because it ignores document structure and would require a separate chunking pass independent of the existing block parser.

## Decision 5: HTML-to-Text for Web Crawl

**Decision**: Use `github.com/k3a/html2text` v1.4.0.

**Rationale**: Pure Go (zero non-standard dependencies), stable API (author considers it feature-complete), handles the key cases: strips `<head>`/`<script>`/`<style>`, converts `<a>` to href text, decodes HTML entities, formats lists. At 153 importers, it's well-adopted. The API is a single function call: `html2text.HTML2Text(htmlString)`.

**Alternatives considered**:
- `github.com/JohannesKaufmann/html-to-markdown` v2: Converts HTML to Markdown, not plain text. Heavier (depends on `goquery`/`net/html`). Over-specified for our use case where we need plain text for embedding.
- Manual implementation using `golang.org/x/net/html`: Possible but reinvents what `k3a/html2text` already does well. No benefit.

## Dependency Summary

| Dependency | Import Path | Version | License | Purpose |
|------------|-------------|---------|---------|---------|
| SQLite | `modernc.org/sqlite` | v1.47.0 | BSD-3 | Persistent graph + embedding storage |
| HTML-to-text | `github.com/k3a/html2text` | v1.4.0 | MIT | Web crawl source HTML conversion |
| CLI framework | `github.com/spf13/cobra` | latest | Apache-2.0 | CLI command routing (convention pack CS-009) |
| Structured logging | `github.com/charmbracelet/log` | latest | MIT | Application logging (convention pack CS-008) |

Total new dependencies: **4**. All pure Go, all permissively licensed, all well-maintained. Cobra and charmbracelet/log are added per review council decision to comply with Go convention pack MUST rules (CS-008, CS-009) rather than overriding them.

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| `modernc.org/sqlite` performance insufficient | Low | Medium | Benchmarked acceptable for our scale. WAL mode for concurrent reads. Can optimize queries or add caching if needed. |
| Brute-force vector search too slow at scale | Low | Low | 5ms for 10k vectors. If scale exceeds 100k, introduce HNSW behind the same `Embedder` interface. |
| Ollama API changes between versions | Low | Medium | Pin to documented `/api/embed` endpoint. The `Embedder` interface abstracts the API. |
| `modernc.org/sqlite` binary size increase (~20MB) | Medium | Low | Acceptable for a CLI/server binary. GoReleaser handles cross-platform builds regardless. |
| Web crawl source blocked by robots.txt or CAPTCHAs | Medium | Low | Graceful degradation: report warning, continue with other sources. Rate limiting and `robots.txt` compliance reduce blocking risk. |
