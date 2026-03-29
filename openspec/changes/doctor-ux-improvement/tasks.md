# Tasks: doctor-ux-improvement

## 1. Add Helper Infrastructure

- [x] 1.1 Add pass/warn/fail counter struct and helper to `cli.go` — `doctorCounter` with `pass`, `warn`, `fail` fields and `printCheck(w, marker, name, description)` method that auto-counts and formats with consistent 20-char name column
- [x] 1.2 Add `humanSize(bytes int64) string` helper that converts bytes to human-readable format (KB, MB, GB)
- [x] 1.3 Add `printSummaryBox(w, counter)` function that prints the `uf doctor` style summary box with emoji counters

## 2. Rewrite Section Output

- [x] 2.1 Rewrite Environment section — use `printCheck` for vault path and dewey binary, remove pid (implementation detail)
- [x] 2.2 Rewrite Workspace section — use `printCheck` for .dewey/, config.yaml, sources.yaml, dewey.log with human-readable sizes and paths in parens
- [x] 2.3 Rewrite Database section — use `printCheck` for graph.db (show human size + page count), remove WAL/SHM display, remove lock check (moved to MCP Server), query embedding count from the open store handle
- [x] 2.4 Rewrite Sources in Database section — use `printCheck` for each source with page count and last-fetched time
- [x] 2.5 Rewrite Embedding Layer section — show endpoint/model as section header context (not as checks), use `printCheck` for Ollama reachability and model availability
- [x] 2.6 Rewrite MCP Server section — use `printCheck` for serve process (consolidate lock check here) and opencode.json
- [x] 2.7 Add summary box at the end via `printSummaryBox`

## 3. Update Tests

- [x] 3.1 Update `TestDoctorCmd_WithStore` in `cli_test.go` — assertions match new `[PASS]` format with 20-char names
- [x] 3.2 Update `TestDoctorCmd_NoInit` in `cli_test.go` — assertion matches new `[FAIL]` format

## 4. Verification

- [x] 4.1 Run `go build ./...`, `go vet ./...`, `go test -race -count=1 ./...` — all must pass
- [x] 4.2 Manual test: run `dewey doctor --vault .` and visually compare output alignment against `uf doctor` from `../unbound-force`
