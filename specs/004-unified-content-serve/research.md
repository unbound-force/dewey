# Research: Unified Content Serve

**Date**: 2026-03-28
**Spec**: [spec.md](spec.md)

## R1: SQLite WAL Mode for Concurrent Access

**Decision**: WAL mode is already enabled — no new work required for FR-012.

**Rationale**: The store's `New()` function (`store/store.go:61`) already sets `PRAGMA journal_mode=WAL` along with `PRAGMA foreign_keys=ON` and `PRAGMA busy_timeout=5000`. The store also uses `syscall.Flock` for file-level locking on non-memory databases. FR-012 is satisfied by existing code.

**Alternatives considered**: None needed — the desired behavior already exists.

**Impact on plan**: Remove FR-012 from the implementation task list. Document as pre-existing in the plan. The spec requirement remains valid as a constraint statement but requires no code changes.

## R2: Exporting Parsing Functions from Vault Package

**Decision**: Create a new exported `ParseDocument()` function in `vault/parse_export.go` that wraps `parseFrontmatter()` and `parseMarkdownBlocks()`.

**Rationale**: Both `parseMarkdownBlocks(filepath, body string)` (`vault/markdown.go:18`) and `parseFrontmatter(content string)` (`vault/frontmatter.go:12`) are package-private. They operate on strings (no disk I/O) and are safe to expose. The `parseFile()` method on `*Client` also depends on `os.FileInfo`, but only for `info.ModTime()` — a minimal `FileInfo` implementation suffices. However, it's cleaner to bypass `parseFile()` entirely and call the two underlying functions directly, accepting a timestamp parameter instead of `os.FileInfo`.

**Alternatives considered**:
- Export `parseFile` directly — rejected because it requires a `*Client` receiver (needs vault config for journal detection) and `os.FileInfo` (unnecessary coupling).
- Move parsing to a shared package — rejected because it would break the existing vault package boundary for minimal gain.

## R3: Store Methods Needed for Source-Level Operations

**Decision**: Add three new store methods: `ListPagesExcludingSource()`, `DeletePagesBySource()`, and `ListPagesBySource()`.

**Rationale**: The store currently has only `CountPagesBySource()` (`store/store.go:624`). The implementation needs:
- `ListPagesExcludingSource(sourceID string)` — for `LoadExternalPages()` to load all non-local pages
- `DeletePagesBySource(sourceID string)` — for orphan auto-purge (FR-013). Uses `DELETE FROM pages WHERE source_id = ?` with CASCADE to blocks/links
- `ListPagesBySource(sourceID string)` — for `dewey status` per-source reporting (FR-010)

No existing delete methods support bulk source-based deletion. `DeletePage(name)` works per-page with CASCADE (`store/store.go:274`), but iterating and deleting one-by-one is inefficient for large source purges.

**Alternatives considered**:
- Loop `DeletePage()` per page — rejected for performance reasons with large sources (N+1 queries)
- Add `source_id` filter to existing `ListPages()` — rejected to keep the existing method's contract stable

## R4: Embedder Availability During `dewey index`

**Decision**: Create an embedder in `dewey index` using the same environment variables as `dewey serve`, with graceful fallback when Ollama is unavailable.

**Rationale**: Currently `dewey index` (`cli.go:553-620`) does not create an embedder. The embedder is only instantiated during `dewey serve` in `main.go:212`. To generate embeddings during indexing (FR-003), the index command needs to:
1. Create an `embed.OllamaEmbedder` with the same `DEWEY_EMBED_ENDPOINT` / `DEWEY_EMBED_MODEL` environment variables
2. Call `embedder.Available()` to check if Ollama is running
3. If available, generate embeddings per-block after parsing
4. If unavailable, log a warning and skip embeddings (graceful degradation, same pattern as serve)

**Alternatives considered**:
- Skip embeddings in `dewey index` entirely — rejected because it defeats the purpose of FR-003 (semantic search across all sources)
- Require Ollama during indexing — rejected because it violates the graceful degradation principle

## R5: Block Tree Reconstruction from Flat Store Data

**Decision**: Implement a `reconstructBlockTree(flat []*store.Block) []types.BlockEntity` function in `vault/vault_store.go`.

**Rationale**: The store persists blocks as flat rows with `parent_uuid` and `position` columns. The vault expects nested `[]types.BlockEntity` with `Children` populated. The reconstruction algorithm:
1. Create map: `uuid → *types.BlockEntity`
2. Iterate flat list, convert each `store.Block` to `types.BlockEntity`
3. For blocks with `ParentUUID.Valid`, append to parent's `Children`
4. Roots (null parent) form the returned slice
5. Children ordered by `Position`

This is ~30 lines and operates purely on data structures (no I/O). Existing `VaultStore.persistBlocks()` (`vault/vault_store.go:508-530`) already writes the tree in the inverse direction, so round-trip fidelity is guaranteed by the schema.

**Alternatives considered**:
- Store blocks as a serialized JSON tree — rejected because it would require a schema migration and break the existing flat block model used by the vault's disk path
- Use `store.GetBlocksByPage()` and re-parse content — rejected because it would require storing raw page content (which the store doesn't have)

## R6: External Page Namespace Strategy

**Decision**: Prefix external page names with `{sourceID}/{documentID}` (e.g., `github-myorg/issues/42`, `web-docs/api-reference`).

**Rationale**: The `source.Document` struct has `SourceID` (e.g., `"github-myorg"`) and `ID` (e.g., `"issues/42"` or a file path). Combining them creates a unique, human-readable namespace. The vault uses lowercase page names as map keys, so `strings.ToLower(sourceID + "/" + docID)` ensures uniqueness. This matches how the store already tracks pages via the `(source_id, source_doc_id)` unique constraint (`store/migrate.go:75`).

**Alternatives considered**:
- Use document title as page name — rejected because titles are not unique (multiple GitHub repos could have issues with the same title)
- Use a hash-based prefix — rejected because it's not human-readable and makes debugging harder
- Use only source_doc_id without source prefix — rejected because doc IDs from different sources could collide (e.g., a GitHub file path and a disk file path)

## R7: Write Guard Implementation Strategy

**Decision**: Add `sourceID string` and `readOnly bool` fields to `cachedPage` struct. Check `readOnly` at the top of each write method.

**Rationale**: The vault has 9 write methods: `CreatePage`, `AppendBlockInPage`, `PrependBlockInPage`, `InsertBlock`, `UpdateBlock`, `RemoveBlock`, `MoveBlock`, `DeletePage`, `RenamePage`. Of these, 8 need write guards (`CreatePage` is unaffected — it creates new pages, not modifying external ones). Each method currently assumes a local `.md` file exists. Adding a guard check at the top of each is simpler than modifying the backend interface (which would require changes to the Logseq client too).

Pages loaded from external sources are marked `readOnly: true` during `LoadExternalPages()`. Pages from disk and pages created via MCP tools are `readOnly: false`.

**Alternatives considered**:
- Add a `ReadOnlyBackend` wrapper that wraps `backend.Backend` — rejected because it adds a layer of indirection for all operations, not just writes
- Check `sourceID != "disk-local"` in each write method — rejected because it's less explicit and doesn't support future writable store-backed pages
