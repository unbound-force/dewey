## Why

Dewey's default embedding model (`granite-embedding:30m`, Dec 2024) has a 512-token context window, forcing the chunker to truncate inputs to 768 characters. This means semantic search only sees small fragments of each block, losing significant context for longer documents, code blocks, and spec artifacts.

IBM released the Granite Embedding R2 family (Aug 2025) with ModernBERT architecture, 16x longer context windows (8192 vs 512 tokens), improved retrieval scores across all benchmarks, and better code retrieval -- directly relevant since Dewey indexes code via the `code` source type.

The R2 small model is a near-drop-in replacement: same 384-dimensional embeddings (no schema changes), similar parameter count (47M vs 30M), same Apache 2.0 license, and comparable encoding speed.

Related: [unbound-force/dewey#53](https://github.com/unbound-force/dewey/issues/53)

## What Changes

1. **Default model name**: Update from `granite-embedding:30m` to R2 equivalent in `embed/config.go` default and `cli.go` init template
2. **Chunker context window**: Increase `maxChunkChars` from 768 to ~12288 to leverage R2's 8192-token context, dramatically improving semantic search quality
3. **Configurable chunk limit**: Make `maxChunkChars` model-aware via a new config field or env var (`DEWEY_CHUNK_MAX_CHARS`), so users with different models get appropriate chunking
4. **Doctor output**: Report the new model name and warn if the old model is still configured
5. **Migration documentation**: Document the upgrade path for existing users

## Capabilities

### New Capabilities
- `configurable-chunk-limit`: Allow users to override the chunk character limit via `DEWEY_CHUNK_MAX_CHARS` env var or `embedding.max_chunk_chars` config field, supporting models with different context windows

### Modified Capabilities
- `embedding-defaults`: Default model changes from `granite-embedding:30m` to the R2 equivalent; default chunk limit increases from 768 to 12288
- `semantic-search`: Quality improvement from embedding larger text chunks (up to 16x more context per block)
- `doctor-diagnostics`: Reports R2 model name; warns when using the legacy model with a suggestion to upgrade

### Removed Capabilities
- None

## Impact

- **Files modified**: `embed/config.go`, `embed/provider.go`, `embed/chunker.go`, `embed/chunker_test.go`, `embed/config_test.go`, `cli.go` (init template, doctor output), `vault/parse_export.go`, `vault/parse_export_test.go`, `vault/vault_store.go`, `vault/vault_store_test.go`, `vault/index_documents.go`, `main.go`, `tools/lint.go`, `tools/learning.go`, `tools/curate.go`, `tools/compile.go`, `README.md`, `AGENTS.md`
- **Backward compatible**: Existing `granite-embedding:30m` installations continue to work. The old model name is still valid -- only the default changes.
- **No schema changes**: R2 produces 384-dimensional embeddings, same as the current model. No database migration needed.
- **Reindex required**: Users upgrading to R2 must run `dewey reindex` to regenerate embeddings with the new model. Old embeddings are cleaned up automatically (store uses `model_id` in primary key).
- **No new dependencies**: All changes are in existing packages.

## Constitution Alignment

Assessed against the Dewey project constitution (v1.4.0).

### I. Composability First

**Assessment**: PASS

Dewey remains independently installable. The model default change is a configuration value -- users can override it via env var or config.yaml. Graceful degradation is preserved: if the R2 model is not pulled, Dewey logs a warning and continues in keyword-only mode. No new mandatory dependencies are introduced.

### II. Autonomous Collaboration

**Assessment**: PASS

This change modifies embedding configuration defaults and chunking behavior. It does not affect artifact-based communication between heroes or MCP tool interfaces. All 50 MCP tools continue to produce identical output formats.

### III. Observable Quality

**Assessment**: PASS

The `dewey doctor` command continues to report embedding model status with machine-parseable output. The chunker change improves provenance quality by embedding more context per block, making semantic search results more accurate. The configurable chunk limit is observable via config inspection.

### IV. Testability

**Assessment**: PASS

The chunker is tested in isolation via `embed/chunker_test.go` (9 existing tests). Config resolution is tested via `embed/config_test.go` and `embed/provider_test.go`. All changes are to pure Go code with no external service dependencies at test time. The configurable chunk limit can be tested with in-memory config.
