## Context

Dewey's embedding pipeline uses `granite-embedding:30m` with a 768-character chunk limit (derived from the model's 512-token context window). The chunker (`embed/chunker.go`) truncates all block content to this limit before embedding, which means semantic search only indexes small fragments of longer blocks.

The Granite Embedding R2 small model offers 8192-token context (16x improvement) with the same 384-dimensional output, enabling much larger chunks without any schema or storage changes. The proposal (proposal.md) confirmed constitution alignment -- this change maintains composability (configurable defaults, graceful degradation) and testability (pure Go, no external services at test time).

## Goals / Non-Goals

### Goals
- Update the default embedding model name to the R2 equivalent
- Increase the chunk character limit to leverage R2's larger context window
- Make the chunk limit configurable so users with different models get appropriate chunking
- Maintain full backward compatibility with `granite-embedding:30m`
- Update doctor diagnostics to report the new default

### Non-Goals
- Auto-detecting model context window from Ollama's `/api/show` endpoint (future enhancement -- requires HTTP call during chunking, adds latency and coupling)
- Supporting multiple concurrent embedding models (existing `model_id` primary key already handles this at the storage layer)
- Automatic migration of existing embeddings (users must run `dewey reindex` manually)
- Adding R2 large or multilingual variants as defaults

## Decisions

### D1: Configurable chunk limit via ProviderConfig

The `maxChunkChars` constant in `embed/chunker.go` becomes a configurable value. The configuration flows through `ProviderConfig` (adding a `MaxChunkChars` field) and is resolved via the existing precedence chain:

1. `DEWEY_CHUNK_MAX_CHARS` env var (highest precedence)
2. `embedding.max_chunk_chars` in per-vault `config.yaml`
3. `embedding.max_chunk_chars` in global `config.yaml`
4. Default: `12288` (for R2's 8192-token context at ~1.5 chars/token)

**Rationale**: The chunk limit is model-dependent. Hardcoding it couples the chunker to a specific model. Making it configurable via the existing config pipeline (Composability First) allows users with different models -- including the old `granite-embedding:30m` -- to set appropriate limits. The `ProviderConfig` struct already carries model metadata, so this is a natural extension.

**Alternative rejected**: Querying `/api/show` at runtime to auto-detect context window. This adds an HTTP call to the hot path, creates a hard dependency on Ollama availability during chunking (violates graceful degradation), and doesn't work for Vertex AI embeddings.

### D2: Default model name update

The hardcoded default in `embed/config.go:149` changes from `"granite-embedding:30m"` to the R2 model name. The `cli.go` init template (`dewey init`) updates to match.

**Rationale**: New installations should get the best available model by default. Existing users with `granite-embedding:30m` in their `config.yaml` or `DEWEY_EMBEDDING_MODEL` env var are unaffected -- their explicit configuration takes precedence over the default.

**Guard**: The R2 model must be available on Ollama before this change ships. If it's not yet available, the default stays as `granite-embedding:30m` and only the configurable chunk limit ships. The issue notes R2 is not yet on Ollama's library.

### D3: PrepareChunk signature change

`PrepareChunk` gains a `maxChars int` parameter instead of using the package-level constant. Callers pass the configured limit from their `ProviderConfig`.

**Rationale**: This eliminates global state (no package-level constant governing behavior), makes the function testable with arbitrary limits, and follows the project convention of preferring dependency injection over global state.

**Call sites**: The function is called from `vault/parse_export.go` during document indexing. The `maxChars` value flows as an explicit `maxChunkChars int` parameter through the call chain: callers of `GenerateEmbeddings` pass the configured value from `ProviderConfig.MaxChunkChars`, `GenerateEmbeddings` passes it to `flattenBlocks`, and `flattenBlocks` passes it to `PrepareChunk`. The `Embedder` interface is NOT modified.

**Affected call sites** (11 production, 6 test):
- `main.go` (2 calls)
- `vault/vault_store.go` (1 call)
- `vault/index_documents.go` (1 call)
- `vault/parse_export.go` (indirect via `flattenBlocks`)
- `tools/lint.go` (1 call)
- `tools/learning.go` (1 call)
- `tools/curate.go` (1 call)
- `tools/compile.go` (2 calls)
- `vault/parse_export_test.go` (4 test calls)
- `vault/vault_store_test.go` (2 test calls)

### D4: Doctor warning for legacy model

`dewey doctor` compares the configured model against the new default and emits an informational note (not a failure) if the user is still on `granite-embedding:30m`, suggesting they upgrade and reindex.

**Rationale**: This is advisory, not blocking. Users may intentionally stay on the old model (e.g., resource-constrained environments). Observable Quality principle -- the doctor surfaces actionable information without enforcing behavior.

## Risks / Trade-offs

### R1: R2 model not yet on Ollama

The R2 model is not yet available via `ollama pull`. If it's still unavailable when this change is ready, we ship the configurable chunk limit with the old default model name. The model name update becomes a follow-up one-line change.

**Mitigation**: The design decouples the two changes. The chunk limit is independently valuable (users can set it via env var today). The model name default is a single constant.

### R2: Larger chunks increase embedding time

Embedding 12288-character chunks takes longer per block than 768-character chunks. For large vaults, initial indexing could be noticeably slower.

**Mitigation**: R2's encoding speed is nearly identical to the original (199 vs 198 docs/sec per the benchmarks). The longer context is processed in a single model call, not multiple calls. Users who need faster indexing can reduce `max_chunk_chars` via config.

### R3: Existing embeddings become stale

After upgrading the model or chunk limit, old embeddings (generated with 768-char chunks) are still served until `dewey reindex` is run. Semantic search quality may be inconsistent during this window.

**Mitigation**: The store uses `model_id` in the embedding primary key, so old-model embeddings are automatically replaced during reindex. Doctor output should suggest reindexing if the configured model differs from the model stored in existing embeddings.
