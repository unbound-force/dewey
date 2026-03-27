## Context

The first quality ratchet (PR #5) decomposed `handleEvent`, `newServer`, and `IncrementalIndex`, reducing CRAPload from 15 to 13. However, extracting `registerHealthTool` from `newServer` created a new worst-offender at CRAP 67.5 (complexity 8, only 2.4% coverage). The GazeCRAPload sits at 34 — exactly at the CI gate boundary — leaving no headroom.

This second ratchet targets the next tier of offenders: the health tool test gap, two CLI commands with complexity >19, the highest-complexity function in the project (`Similar` at 22), and Q3 navigate functions with adequate line coverage but weak contract assertions.

Per the proposal's constitution alignment: N/A for Autonomous Collaboration, PASS for Composability First, Observable Quality, and Testability.

## Goals / Non-Goals

### Goals
- Reduce CRAPload from 13 to <=10 by adding tests for `registerHealthTool` and decomposing `newIndexCmd` and `newStatusCmd`
- Reduce GazeCRAPload from 34 to <=30 by decomposing `Similar` and strengthening navigate contract assertions
- Create headroom below the CI gates (CRAPload <=15, GazeCRAPload <=34) so future development doesn't immediately break CI

### Non-Goals
- Changing external MCP tool behavior or API contracts
- Refactoring the health tool's internal structure (only adding test coverage)
- Addressing functions below the top 5 recommendations (e.g., `RenamePage`, `AppendBlockInPage`, `DiskSource.Diff`)
- Achieving 100% contract coverage — focus on the worst Q3 functions only

## Decisions

### 1. Testing `registerHealthTool` via MCP server interface

The health tool is registered as a closure on the MCP server, making it impossible to call directly in tests. The test strategy is to create a server with `newServer(mockBackend, false, opts...)`, then invoke the health tool through the server's tool-call interface using `mcp.CallTool`. This matches how existing `TestNewServer_*` tests work in `server_test.go`.

Test configurations:
- Nil store + nil embedder (minimal config)
- Non-nil store with pages, blocks, embeddings, and sources
- Non-nil embedder with model available
- Non-nil embedder with model unavailable
- Backend ping error

**Rationale**: Testing through the MCP interface is the only option since the health handler is a closure. This approach also validates the full registration path, catching any issues with the `mcp.AddTool` call itself.

### 2. Decomposition pattern for `newIndexCmd` (cli.go:513, complexity 19)

Extract three private functions:
- `loadSourceConfigs(deweyDir string) ([]source.Config, error)` — loads and validates sources.yaml
- `indexDocuments(s *store.Store, allDocs map[string][]source.Document) (int, error)` — loops over fetched documents, upserts pages and blocks
- `generateEmbeddings(s *store.Store, e embed.Embedder) error` — handles the embedding pass logic

The outer `RunE` becomes an orchestrator: open store → load configs → create manager → fetch → index → embed → report.

**Rationale**: The 80-line `RunE` closure has three distinct phases already separated by comments. Each phase is independently testable. The store and config loading can be tested with in-memory SQLite and temp dirs; the embedding pass is already tested elsewhere.

### 3. Decomposition pattern for `(*Semantic).Similar` (tools/semantic.go:88, complexity 22)

Extract three private methods:
- `validateSimilarInput(input types.SimilarInput) *mcp.CallToolResult` — validates input fields and embedder/store availability; returns error result or nil
- `resolveQueryVector(ctx context.Context, input types.SimilarInput, modelID string) ([]float32, *mcp.CallToolResult)` — resolves the query embedding by UUID or page name; returns vector or error result
- `filterSimilarResults(results []store.SimilarityResult, excludeUUID, excludePage string, limit int) []types.SemanticSearchResult` — filters self-references and builds the response objects

The outer `Similar` method becomes: validate → resolve vector → search → filter → format.

**Rationale**: The function has complexity 22 (highest in the project) driven by multiple early-return error paths and the UUID-vs-page branching for vector resolution. Extracting validation and vector resolution removes the two largest sources of branching. The result filtering is a pure function with no error paths.

### 4. Navigate contract assertion strategy

The existing navigate tests in `tools/navigate_test.go` call `GetPage`/`GetBlock`/`ListPages` and check for non-error return. Strengthen by asserting:
- `GetPage`: block tree depth, outgoing link names, backlink count, property extraction from enriched blocks, truncation behavior with `MaxBlocks`
- `GetBlock`: ancestor chain, children present, sibling blocks when requested
- `ListPages`: page count, sort order, namespace filtering results

**Rationale**: These functions have 84-100% line coverage but only 25% contract coverage. The gap is entirely in assertion weakness — the mock backend provides rich data that the tests currently ignore.

### 5. Decomposition pattern for `newStatusCmd` (cli.go:271, complexity 21)

Extract two private functions:
- `queryStoreStatus(dbPath string) (statusData, error)` — opens the store, queries counts and sources, closes the store, returns a `statusData` struct
- `formatStatusOutput(data statusData, jsonOutput bool) error` — formats and prints the status in text or JSON mode

The outer `RunE` becomes: find .dewey dir → read config → query store → format output.

**Rationale**: The 130-line `RunE` closure interleaves store queries with output formatting. Separating them lets each be tested independently — the query function with in-memory SQLite, the format function with known input structs.

## Risks / Trade-offs

- **Health tool test coupling**: Testing through the MCP server interface couples the test to the MCP SDK's `CallTool` behavior. If the SDK changes how it dispatches tools, the test could break even though the health logic is correct. Mitigated by the SDK being a stable v1.2.0 dependency.
- **CLI decomposition increases function count**: Extracting helpers from `newIndexCmd` and `newStatusCmd` adds ~6 new functions. These increase the total function count in Gaze analysis, which could dilute average metrics. Mitigated by each new function being simple (complexity <5) and well-tested.
- **Navigate assertion fragility**: Adding structural assertions to navigate tests makes them more sensitive to changes in the JSON output format. Mitigated by asserting on semantic structure (key exists, type correct) rather than exact values where possible.
