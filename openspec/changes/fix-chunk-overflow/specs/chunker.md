## MODIFIED Requirements

### Requirement: Chunk Size Limit

`PrepareChunk` MUST truncate content to 768 characters (was 1,000) to stay within the embedding model's 512-token context window at worst-case tokenization ratios.

Previously: 1,000 characters, which exceeded context window for code-heavy content.

#### Scenario: Code-heavy block
- **GIVEN** a block with 900 characters of Go source code
- **WHEN** `PrepareChunk` is called
- **THEN** the output is truncated to 768 characters (rune-based)

### Requirement: Accurate Chunk Length Logging

Embedding failure warnings MUST log the chunk length as **rune count**, not byte count.

Previously: `len(chunk)` logged byte count, which inflated the number for multi-byte content.

#### Scenario: Warning log accuracy
- **GIVEN** a chunk that fails embedding
- **WHEN** the warning is logged
- **THEN** `chunkLen` reflects the rune count matching `maxChunkChars`

### Requirement: Context Overflow Recovery

When the embedding model rejects input as too long, the system MUST retry with a truncated version (half the content) rather than skipping the block entirely.

#### Scenario: Retry on overflow
- **GIVEN** a chunk that triggers "context length" error from the embedding model
- **WHEN** `GenerateEmbeddings` processes the block
- **THEN** it retries with the chunk truncated to half its rune length
- **AND** if the retry succeeds, the embedding is stored
- **AND** if the retry also fails, the block is skipped with a warning
