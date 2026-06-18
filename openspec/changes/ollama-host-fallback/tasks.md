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

## 1. Default endpoint constant and endpoint resolver

- [x] 1.1 Add `DefaultOllamaEndpoint` constant and `ResolveOllamaEndpoint()` helper to `embed/config.go`. The helper implements the three-tier fallback: `DEWEY_EMBEDDING_ENDPOINT` > `OLLAMA_HOST` > `DefaultOllamaEndpoint`. Normalize `OLLAMA_HOST` values without a scheme by prepending `http://`. Update `ReadEmbeddingConfig()` and `embedConfigFromEnv()` to use the helper and constant.
- [x] 1.2 Add unit tests in `embed/config_test.go` for `ResolveOllamaEndpoint()`: DEWEY_EMBEDDING_ENDPOINT set, OLLAMA_HOST set, both set (precedence), neither set (default), OLLAMA_HOST without scheme (normalization), OLLAMA_HOST with HTTPS (preserved), OLLAMA_HOST empty string (treated as unset), config.yaml endpoint vs OLLAMA_HOST (config wins).
- [x] 1.3 Update `embed/provider.go` `NewEmbedderFromConfig()` to use `DefaultOllamaEndpoint` constant instead of hardcoded string.

## 2. Graceful degradation on missing model

- [x] 2.1 [P] In `main.go` `initObsidianBackend()`, replace the hard error at line 646-648 with a warning log and continue without embeddings (set `embedder = nil`). The warning must include model name, endpoint, and `ollama pull` instructions.
- [x] 2.2 [P] In `cli.go` `createIndexEmbedder()`, replace the hard error at line 950-952 with a warning log and return `nil, nil` (matching the existing `OllamaUnavailable` graceful degradation path).
- [x] 2.3 Add/update tests in `main_test.go` for the missing-model graceful degradation path in `initObsidianBackend`. Verify that: (a) `initObsidianBackend` returns a non-nil backend and no error, (b) the embedder is nil in the returned configuration (proving keyword-only mode), (c) log output contains the model name, endpoint, and `ollama pull` instructions.
- [x] 2.4 Add/update tests in `cli_test.go` (or appropriate test file) for the missing-model graceful degradation path in `createIndexEmbedder`. Verify that: (a) `nil, nil` is returned (no error, no embedder), (b) log output contains the model name, endpoint, and `ollama pull` instructions.

## 3. Doctor endpoint consistency

- [x] 3.1 In `cli.go` doctor command (~line 1350), replace the direct `os.Getenv("DEWEY_EMBEDDING_ENDPOINT")` and `os.Getenv("DEWEY_EMBEDDING_MODEL")` reads with a call to `embed.ReadEmbeddingConfig(deweyDir)`. Use the returned config's `Endpoint` and `Model` fields. Remove the hardcoded fallback to `http://localhost:11434`.

## 4. llm/config.go constant update

- [x] 4.1 [P] Update `llm/config.go` `ReadSynthesisConfig()` (line 84-86) and `synthConfigFromEnv()` (line 116-118) to use `embed.DefaultOllamaEndpoint` instead of hardcoded `http://localhost:11434`. Note: `OLLAMA_HOST` fallback for synthesis is out of scope (separate issue).
- [x] 4.2 [P] Update `llm/config_test.go` (if any tests assert on the default endpoint string) to use `embed.DefaultOllamaEndpoint`.
- [x] 4.3 [P] Update `llm/provider.go` `NewSynthesizerFromConfig()` to use `embed.DefaultOllamaEndpoint` constant instead of hardcoded `http://localhost:11434`.

## 5. Verification

- [x] 5.1 Run `go build ./...` — verify clean build.
- [x] 5.2 Run `go vet ./...` — verify no warnings.
- [x] 5.3 Run `go test -race -count=1 ./...` — verify all tests pass.
- [x] 5.4 Verify no remaining hardcoded `http://localhost:11434` in Go source files outside of the constant definition and test assertions.
- [x] 5.5 Verify constitution alignment: Composability First (Dewey starts in more environments), Observable Quality (doctor reports correct endpoint), Testability (all new paths have unit tests).
- [x] 5.6 Assess documentation impact: update `AGENTS.md` Provider Configuration and env var references to document `OLLAMA_HOST` fallback.
- [x] 5.7 Create GitHub issue in `unbound-force/website` documenting the `OLLAMA_HOST` env var fallback and graceful degradation behavior change. Reference existing website issue #113 (pluggable providers) and #114 (provider configuration tutorial) as related context.
<!-- spec-review: passed -->
<!-- code-review: passed -->
