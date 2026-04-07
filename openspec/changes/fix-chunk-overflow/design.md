## Context

`granite-embedding:30m` has a 512-token context window. The chunker truncates to 1,000 characters, but code/YAML tokenizes at 1-2 chars/token, so 1,000 chars can produce up to 1,000 tokens — 2x the context limit.

## Goals / Non-Goals

### Goals
- Reduce chunk size to stay within 512 tokens for worst-case content
- Log accurate chunk sizes (rune count, not byte count)
- Recover from context overflow errors instead of skipping blocks

### Non-Goals
- Token-level truncation (would require a tokenizer, adds complexity)
- Changing the embedding model
- Re-embedding existing content (only affects future embeddings)

## Decisions

**D1: Reduce maxChunkChars to 768.** At the worst observed tokenization ratio of 1.5 chars/token, 768 chars = 512 tokens. This is conservative enough for code-heavy content while still providing meaningful context for the embedding model.

**D2: Log rune count, not byte count.** Change `"chunkLen", len(chunk)` to `"chunkLen", len([]rune(chunk))` in `parse_export.go`. This makes the warning directly comparable to `maxChunkChars`.

**D3: Retry with truncation on overflow.** When `Embed()` returns a context-length error, `GenerateEmbeddings` retries with the chunk truncated to half its length. This is a best-effort recovery — a shorter chunk is better than no embedding at all. Maximum one retry to avoid infinite loops.

## Risks / Trade-offs

**Trade-off: Shorter chunks = less context per embedding.** Reducing from 1,000 to 768 chars means embeddings capture ~23% less text. This may slightly reduce semantic search relevance for long blocks. The improvement in coverage (more blocks get embeddings) should outweigh this.

**Risk: Retry masking real errors.** The retry only triggers on the specific "context length" error string. Other embedding errors (network, model not found) are not retried.
