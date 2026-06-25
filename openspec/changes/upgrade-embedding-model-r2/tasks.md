<!--
  [P] marks tasks eligible for parallel execution.
  Add [P] when a task: (a) touches different files from
  other [P] tasks in the group, (b) has no dependency
  on prior tasks in the group, (c) can safely execute
  without ordering constraints.
  Do NOT add [P] when tasks modify the same file —
  parallel workers will cause merge conflicts.
  Tasks without [P] run sequentially first, then [P]
  tasks run in parallel.
-->

## 1. Add configurable chunk limit to ProviderConfig and config pipeline

- [x] 1.1 Add `MaxChunkChars int` field to `ProviderConfig` struct in `embed/provider.go`. Add `MaxChunkChars int` yaml tag `max_chunk_chars` to `configFile.Embedding` struct in `embed/config.go`. Update `ReadEmbeddingConfig()` to populate `MaxChunkChars` from config, env var `DEWEY_CHUNK_MAX_CHARS`, or default `12288`. Ensure the default is applied in both the `embedConfigFromEnv()` path AND the config-file path (when config doesn't specify `max_chunk_chars`). Validate: non-positive or non-numeric values MUST log a warning and fall back to `12288`.
  - **Files**: `embed/provider.go`, `embed/config.go`

- [x] 1.2 [P] Add tests for `MaxChunkChars` resolution in `embed/config_test.go`: env var override (`DEWEY_CHUNK_MAX_CHARS=2048`), config file value, default fallback (no config or env var), invalid non-numeric value (`DEWEY_CHUNK_MAX_CHARS=abc` logs warning, uses default), zero value (`DEWEY_CHUNK_MAX_CHARS=0` logs warning, uses default), negative value (`DEWEY_CHUNK_MAX_CHARS=-5` logs warning, uses default). Use `t.TempDir()` with written `config.yaml` files for config-file tests and `t.Setenv()` for env var tests, following the pattern in `TestReadEmbeddingConfig_ConfigYAMLWinsOverOllamaHost`.
  - **Files**: `embed/config_test.go`

## 2. Update PrepareChunk to accept configurable limit

- [x] 2.1 Change `PrepareChunk` signature from `PrepareChunk(pageName string, headingPath []string, content string) string` to `PrepareChunk(pageName string, headingPath []string, content string, maxChars int) string`. Remove the `maxChunkChars` package-level constant. Use the `maxChars` parameter for truncation. Update GoDoc comment to reflect the parameter.
  - **Files**: `embed/chunker.go`

- [x] 2.2 [P] Update all `PrepareChunk` and `GenerateEmbeddings` call sites. Add `maxChunkChars int` parameter to `GenerateEmbeddings` signature (the `Embedder` interface is NOT modified). Thread the parameter through `GenerateEmbeddings` -> `flattenBlocks` -> `PrepareChunk`. All callers of `GenerateEmbeddings` pass the configured value from `ProviderConfig.MaxChunkChars`. Production call sites (11 total): `main.go` (2 calls), `vault/vault_store.go` (1 call), `vault/index_documents.go` (1 call), `tools/lint.go` (1 call), `tools/learning.go` (1 call), `tools/curate.go` (1 call), `tools/compile.go` (2 calls), `vault/parse_export.go` (indirect via `flattenBlocks`). Test call sites (6 total): `vault/parse_export_test.go` (4 calls), `vault/vault_store_test.go` (2 calls). Verify: run `grep -rn 'GenerateEmbeddings(' --include='*.go'` and confirm every call site passes the configured `maxChunkChars`.
  - **Files**: `vault/parse_export.go`, `vault/vault_store.go`, `vault/index_documents.go`, `main.go`, `tools/lint.go`, `tools/learning.go`, `tools/curate.go`, `tools/compile.go`, `vault/parse_export_test.go`, `vault/vault_store_test.go`

- [x] 2.3 Update `embed/chunker_test.go`: all 9 existing tests must pass the `maxChars` parameter (use a test-local constant, e.g., `const testMaxChars = 768`). Update truncation test assertions that reference the removed `maxChunkChars` constant to use the test-local value. Add tests for different limit values: small limit (50), large limit (12288), exact boundary, and no-truncation case (content shorter than limit returns full content).
  - **Files**: `embed/chunker_test.go`

## 3. Update default model name

- [x] 3.1 Update the hardcoded default model in `embed/config.go:149` from `"granite-embedding:30m"` to the R2 model name. Check `https://ollama.com/library/granite-embedding` for an R2 variant. **Guard**: If R2 is not yet on Ollama, keep the current default and add a TODO comment with the target model name and a link to tracking issue (#53).
  - **Files**: `embed/config.go`

- [x] 3.2 [P] Update the `dewey init` config template in `cli.go` (around line 251) to use the new default model name. Include a comment noting the model can be overridden via `DEWEY_EMBEDDING_MODEL`.
  - **Files**: `cli.go`

## 4. Update doctor diagnostics

- [x] 4.1 Add legacy model advisory to `dewey doctor` in `cli.go` (Embedding Layer section, around line 1382). After the existing model availability check, compare the configured model against `"granite-embedding:30m"`. If it matches, display an informational note suggesting the user upgrade to the R2 model and run `dewey reindex`. Use `INFO` marker, not `FAIL`.
  - **Files**: `cli.go`

- [x] 4.2 [P] Add tests for the legacy model advisory in the doctor output. Test that `granite-embedding:30m` triggers the informational note and that other model names do not. Verify the note uses INFO marker, not FAIL. Extract the comparison logic into a testable helper function if needed.
  - **Files**: `cli.go` or `cli_test.go`

## 5. Verification and documentation

- [x] 5.1 Run `go build ./...` to verify compilation.
- [x] 5.2 Run `go vet ./...` to verify static analysis.
- [x] 5.3 Run `go test -race -count=1 ./...` to verify all tests pass. Run `gaze report ./... --coverprofile=coverage.out --max-crapload=48 --max-gaze-crapload=18 --min-contract-coverage=70` if gaze is available and verify no regressions.
- [x] 5.4 Verify constitution alignment: Composability (configurable defaults, graceful degradation preserved), Observable Quality (doctor reports new model status), Testability (chunker testable with arbitrary limits, no external service deps).
- [x] 5.5 Update `AGENTS.md` to document the new `DEWEY_CHUNK_MAX_CHARS` env var, `embedding.max_chunk_chars` config field, updated default embedding model, and updated default chunk limit. Update `README.md`: add `DEWEY_CHUNK_MAX_CHARS` to the Environment Variables table, add `max_chunk_chars` to Provider Configuration examples, update `ollama pull` example and model references if the default model changed.
- [x] 5.6 Create a GitHub issue in `unbound-force/website` documenting: new `DEWEY_CHUNK_MAX_CHARS` env var, new `embedding.max_chunk_chars` config field, changed default model name (if applicable), and `dewey doctor` legacy model advisory. Reference affected website pages: Environment Variables table, Provider Configuration section, Semantic Search Setup section, Doctor section. Use format: `docs: sync dewey embedding model upgrade documentation`.
- [x] 5.7 If the default model name changed, update `.specify/memory/constitution.md` line 156 (default model name parenthetical) as a PATCH version bump.
<!-- spec-review: passed -->
<!-- code-review: passed -->
