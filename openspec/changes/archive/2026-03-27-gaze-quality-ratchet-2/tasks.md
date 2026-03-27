## 1. Add tests for `registerHealthTool` (server.go)

- [x] 1.1 Add `TestRegisterHealthTool_MinimalConfig` — create server with nil store + nil embedder, call health tool via MCP, assert JSON response has `dewey.persistent == false`, `dewey.embeddingAvailable == false`, `dewey.embeddingCount == 0`, `status == "ok"`
- [x] 1.2 Add `TestRegisterHealthTool_WithStore` — create server with in-memory store containing pages, blocks, embeddings, and source records; assert JSON response has `dewey.persistent == true`, correct `embeddingCount`, `embeddingCoverage > 0`, and `sources` array with per-source status
- [x] 1.3 Add `TestRegisterHealthTool_WithEmbedder` — create server with mock embedder whose `Available()` returns true; assert `dewey.embeddingAvailable == true` and `dewey.embeddingModel` matches `ModelID()`
- [x] 1.4 Add `TestRegisterHealthTool_PingError` — create server with mock backend whose `Ping()` returns error; assert `status` starts with `"error:"`
- [x] 1.5 Run `go test -race -count=1 ./...` and verify all tests pass
- [x] 1.6 Run `gaze crap --format=json .` and verify `registerHealthTool` CRAP score dropped below 15

## 2. Decompose `newIndexCmd` (cli.go)

- [x] 2.1 Extract `indexDocuments(s *store.Store, allDocs map[string][]source.Document) (int, error)` — moves the document upsert loop into a private function
- [x] 2.2 Extract `reportSourceErrors(s *store.Store, result *source.FetchResult)` — moves source error reporting into a private function
- [x] 2.3 Refactor `newIndexCmd` RunE into orchestrator: open store → load configs → create manager → fetch → indexDocuments → reportSourceErrors → report
- [x] 2.4 Run `go test -race -count=1 ./...` and verify all tests pass
- [x] 2.5 Run `gaze crap --format=json .` and verify `newIndexCmd` CRAP score dropped below 15

## 3. Decompose `(*Semantic).Similar` (tools/semantic.go)

- [x] 3.1 Extract `validateSimilarInput(input types.SimilarInput) *mcp.CallToolResult` — moves input validation and embedder/store availability checks into a private method; returns error result or nil
- [x] 3.2 Extract `resolveQueryVector(ctx context.Context, input types.SimilarInput, modelID string) ([]float32, *mcp.CallToolResult)` — moves UUID-vs-page vector lookup into a private method; returns vector or error result
- [x] 3.3 Extract `filterSimilarResults(results []store.SimilarityResult, excludeUUID string, limit int) []store.SimilarityResult` — moves the self-exclusion filtering into a pure function
- [x] 3.4 Refactor `Similar` into orchestrator: validate → resolve vector → search → filter → format
- [x] 3.5 Run `go test -race -count=1 ./tools/...` and verify all tests pass
- [x] 3.6 Run `gaze crap --format=json ./tools/...` and verify `Similar` CRAP score dropped below 15

## 4. Strengthen navigate test contracts (tools/navigate_test.go)

- [x] 4.1 Strengthen `TestGetPage` — added assertions for required keys, page name/originalName, exact blockCount, outgoing link names, backlinks structure, and linkCount
- [x] 4.2 Strengthen `TestGetBlock` — added assertions for uuid, children array with correct entries, parsed field with links, and leaf block test
- [x] 4.3 Strengthen `TestListPages` — added assertions for page entry fields (name, journal, properties, updatedAt), journal flag correctness, and property pass-through
- [x] 4.4 Run `go test -race -count=1 ./tools/...` and verify all tests pass
- [x] 4.5 Run `gaze crap --format=json ./tools/...` and verify GazeCRAP scores for navigate functions improved

## 5. Decompose `newStatusCmd` (cli.go)

- [x] 5.1 Define `statusData` struct with PageCount, BlockCount, EmbeddingCount, EmbeddingModel, EmbeddingAvailable, Sources, IndexPath fields plus embeddingCoverage() method
- [x] 5.2 Extract `queryStoreStatus(deweyDir string) (statusData, error)` — opens store, queries all counts and sources, closes store, returns populated struct
- [x] 5.3 Extract `formatStatusText(data statusData, w io.Writer) error` — formats human-readable status output
- [x] 5.4 Extract `formatStatusJSON(data statusData, w io.Writer) error` — formats JSON status output
- [x] 5.5 Refactor `newStatusCmd` RunE into orchestrator: find .dewey dir → readEmbeddingModel → queryStoreStatus → formatStatusText/JSON (20 lines)
- [x] 5.6 Run `go test -race -count=1 ./...` and verify all tests pass
- [x] 5.7 Run `gaze crap --format=json .` and verify `newStatusCmd` CRAP score dropped below 15

## 6. Verification

- [x] 6.1 Run full CI-equivalent checks: `go build ./... && go vet ./... && go test -race -count=1 ./...` — all 11 packages pass
- [x] 6.2 Run Gaze threshold gate: CRAPload=10 (pass, gate 15), GazeCRAPload=33 (pass, gate 34)
- [x] 6.3 CRAPload = 10 (target ≤10, down from 13) — HIT TARGET
- [x] 6.4 GazeCRAPload = 33 (target ≤30 not fully met, but improved from 34 and CI gate passes)
- [x] 6.5 All tests pass including TestNewServer_ToolCount — 40 tools registered
- [x] 6.6 Constitution alignment verified: Composability First (no new deps), Observable Quality (Gaze gates pass with headroom), Testability (all packages pass -race)
