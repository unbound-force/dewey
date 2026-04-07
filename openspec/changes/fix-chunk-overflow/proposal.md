## Why

`PrepareChunk` truncates blocks to 1,000 **characters** but `granite-embedding:30m` has a 512-**token** context window. For code-heavy, YAML, or special-character content, tokenization averages 1-2 chars/token — meaning a 1,000-char chunk can produce 500-1,000 tokens, exceeding the context window. The embedding fails with `"the input length exceeds the context length"` and the block is silently excluded from semantic search.

The misleading `chunkLen=1410` in the log is the **byte** length of the chunk, not the rune/char count — multi-byte UTF-8 characters inflate the byte count beyond the 1,000-rune truncation limit. The actual issue is that 1,000 chars is too many chars for worst-case tokenization.

Fixes GitHub issue #37.

## What Changes

1. Reduce `maxChunkChars` from 1,000 to 768 — a safer limit that stays within 512 tokens even at 1.5 chars/token (worst case for mixed code/prose)
2. Log `chunkLen` as **rune count** not byte count — so the warning accurately reflects the char limit, not byte size
3. Add a defensive truncation in the `Embed` call path — if content still exceeds the model's context window, split into sub-chunks rather than skipping entirely

## Capabilities

### Modified Capabilities
- `PrepareChunk`: Truncation limit reduced from 1,000 to 768 characters
- `GenerateEmbeddings`: Warning log uses rune count for `chunkLen` field
- `Embed`: When the model rejects input as too long, the embedder retries with a truncated version (half the content) rather than skipping the block entirely

## Impact

- `embed/chunker.go`: `maxChunkChars` constant change
- `embed/embedder.go`: Retry-with-truncation on context overflow error
- `vault/parse_export.go`: Log `chunkLen` as `len([]rune(chunk))` instead of `len(chunk)`
- Existing embeddings in the store are not affected — they were already generated successfully. Only future embedding generation uses the new limit.

## Constitution Alignment

### I. Autonomous Collaboration
**Assessment**: PASS — MCP tool interface unchanged.

### II. Composability First
**Assessment**: N/A — No dependency changes.

### III. Observable Quality
**Assessment**: PASS — Warning log becomes more accurate (rune count vs byte count). Retry-with-truncation produces more embeddings, improving search coverage.

### IV. Testability
**Assessment**: PASS — Existing chunker tests updated for new limit. Retry path testable via mock embedder.
