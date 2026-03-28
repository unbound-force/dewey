# Tasks: ollama-hard-error

## 1. Add --no-embeddings Flag

- [x] 1.1 Add `noEmbeddings bool` flag to the root command in `main.go` (alongside `--vault`, `--backend`, etc.) and pass it through `executeServe()` to `initObsidianBackend()`
- [x] 1.2 Add `--no-embeddings` flag to `newServeCmd()` in `main.go` and pass it through to `executeServe()`
- [x] 1.3 Add `--no-embeddings` flag to `newIndexCmd()` in `cli.go` and pass it through to `createIndexEmbedder()`

## 2. Change Embedding Unavailability to Hard Error

- [x] 2.1 In `initObsidianBackend()` in `main.go`: when `noEmbeddings` is false and `embedder.Available()` returns false, return an error with an actionable message containing the model name, endpoint, `ollama pull` command, and `--no-embeddings` hint
- [x] 2.2 In `initObsidianBackend()` in `main.go`: when `noEmbeddings` is true, skip embedder creation entirely (set embedder to nil) and log INFO that embeddings are disabled
- [x] 2.3 In `createIndexEmbedder()` in `cli.go`: accept a `noEmbeddings` parameter. When true, return nil. When false, check availability and return a hard error if unavailable (same message format as serve)
- [x] 2.4 Update the index command's `RunE` in `cli.go` to handle the error from `createIndexEmbedder()` — if it returns an error, exit with that error instead of proceeding

## 3. Add dewey doctor Command

- [x] 3.1 Create `newDoctorCmd()` in `cli.go` with a `--vault` flag (same resolution as search: flag or `OBSIDIAN_VAULT_PATH` env var, resolve to absolute path)
- [x] 3.2 Implement check 1: `.dewey/` directory exists at vault path. Print pass/fail with `dewey init` as fix
- [x] 3.3 Implement check 2: `graph.db` exists and has pages. Open store, call `CountPagesBySource()` or `ListPages()`. Print pass/fail with `dewey index` as fix
- [x] 3.4 Implement check 3: Ollama reachable. Create embedder, call `Available()` — but first check if the endpoint is reachable at all (HTTP GET to `/api/tags`). Print pass/fail with install/start instructions
- [x] 3.5 Implement check 4: Embedding model available. Use `embedder.Available()`. Print pass/fail with `ollama pull <model>` as fix
- [x] 3.6 Register `newDoctorCmd()` in `newRootCmd()` in `main.go`

## 4. Tests

- [x] 4.1 Write test for `--no-embeddings` on serve: verify serve starts without error when Ollama is unavailable and `--no-embeddings` is set
- [x] 4.2 Write test for hard error on serve: verify serve fails with actionable error when Ollama is unavailable and `--no-embeddings` is NOT set
- [x] 4.3 Write test for `newDoctorCmd()`: create temp vault with `.dewey/` and `graph.db`, verify doctor reports pass for init and store checks (Ollama checks will show fail in test env, which is expected)
- [x] 4.4 Write test for doctor with missing `.dewey/`: verify doctor reports fail with `dewey init` fix

## 5. Verification

- [x] 5.1 Run `go build ./...`, `go vet ./...`, `go test -race -count=1 ./...` — all must pass
- [x] 5.2 Manual verification: run `dewey doctor --vault .` and verify output shows pass/fail for each check
- [x] 5.3 Manual verification: run `dewey serve --vault . --no-embeddings` and verify it starts without error
