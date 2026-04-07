## 1. Reduce Chunk Size Limit

- [x] 1.1 In `embed/chunker.go`: change `maxChunkChars` from `1000` to `768`
- [x] 1.2 Update the comment explaining the rationale — reference 1.5 chars/token worst case for code content

## 2. Fix Chunk Length Logging

- [x] 2.1 In `vault/parse_export.go` line 141: change `"chunkLen", len(chunk)` to `"chunkLen", len([]rune(chunk))` so the warning reports rune count matching `maxChunkChars`

## 3. Add Context Overflow Retry

- [x] 3.1 In `vault/parse_export.go` `GenerateEmbeddings()`: when `embedder.Embed()` returns an error containing "context length", retry once with the chunk truncated to half its rune length
- [x] 3.2 Log the retry attempt at DEBUG level: `"retrying embedding with truncated chunk"` with original and truncated lengths

## 4. Update Tests

- [x] 4.1 In `embed/chunker_test.go`: update the truncation test to expect 768 chars instead of 1000 (tests use `maxChunkChars` constant directly — auto-updated)
- [x] 4.2 In `vault/vault_store_test.go`: add `TestGenerateEmbeddings_RetryOnContextOverflow` — mock embedder that fails with "context length" on first call, succeeds on second with shorter input. Verify embedding is stored.
- [x] 4.3 In `vault/vault_store_test.go`: add `TestGenerateEmbeddings_RetryFailsBoth` — mock embedder that fails both calls. Verify warning logged and block skipped.

## 5. Verification

- [x] 5.1 Run `go build ./...` and `go vet ./...`
- [x] 5.2 Run `go test -race -count=1 ./...` — all tests pass
