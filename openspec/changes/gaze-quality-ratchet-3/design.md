## Context

Two quality ratchets have reduced CRAPload from 15 to 10 and GazeCRAPload from 34 to 33. The remaining top-5 functions are dominated by complexity rather than coverage gaps, requiring decomposition. One function (`indexDocuments`) was newly extracted in ratchet-2 and needs test coverage. One tool (`GetWhiteboard`) has the worst GazeCRAP in the project (85.7) due to assertion structure that Gaze's mapper can't trace.

Per the proposal's constitution alignment: N/A for Autonomous Collaboration, PASS for Composability First, Observable Quality, and Testability.

## Goals / Non-Goals

### Goals
- Reduce CRAPload from 10 to <=7 by decomposing `RenamePage`, `newSourceAddCmd`, and `DiskSource.Diff`
- Reduce GazeCRAPload from 33 to <=28 by restructuring whiteboard test assertions and adding `indexDocuments` tests
- Eliminate all Q4 Dangerous functions that are addressable in this iteration

### Non-Goals
- Changing external MCP tool behavior or API contracts
- Refactoring functions below the top 5 (e.g., `PrependBlockInPage`, `crawl`, `parseDecisionBlock`)
- Adding new test infrastructure or frameworks

## Decisions

### 1. Decomposition pattern for `RenamePage` (vault/vault.go:981, complexity 17)

Extract four private methods:
- `renameFile(oldPath, newAbsPath string) error` — creates target directory and renames the file
- `reindexRenamed(newRelPath, newAbsPath string)` — reads the renamed file and re-indexes it (called within the existing lock)
- `cleanupEmptyDirs(startDir, vaultAbs string)` — walks up from the old file's directory removing empty directories

The validation (name length, null bytes, page existence, target conflict) and path computation remain in `RenamePage`. The link-update call (`updateLinksAcrossVaultLocked`) is already a separate method and stays as-is.

**Rationale**: The complexity comes from four distinct phases: validation → file rename → link update → index rebuild → cleanup. Each phase is independently testable. The lock scope stays in `RenamePage` since all phases need the lock.

### 2. Decomposition pattern for `newSourceAddCmd` (cli.go:714, complexity 19)

Extract three private functions:
- `buildGitHubSource(org, repos, content, refresh string) (source.SourceConfig, error)` — validates required fields, parses repo/content lists, returns populated config
- `buildWebSource(webURL, webName, refresh string, depth int) (source.SourceConfig, error)` — validates URL, derives name, returns populated config
- `saveSourceConfig(sourcesPath string, existing []source.SourceConfig, newSource source.SourceConfig) error` — checks for duplicates, appends, and saves

The RunE becomes: get working dir → load existing → build config by type → save → log.

**Rationale**: The complexity comes from the `switch sourceType` block with two branches of validation + config construction, plus the duplicate check and save. Extracting per-type builders eliminates the switch branching from the orchestrator.

### 3. Test strategy for `indexDocuments` (cli.go:624, complexity 10)

Add tests in `cli_test.go` covering:
- Insert new page: verify `s.GetPage()` returns the inserted page with correct fields
- Update existing page: insert a page first, call `indexDocuments` with same title but different hash, verify update
- Source record creation: verify `s.GetSource()` returns the new source after indexing
- Properties marshaling: provide a document with `Properties` map, verify the page's `Properties` field is valid JSON

Test infrastructure: use in-memory SQLite (`store.New(":memory:")`), create `source.Document` structs with `time.Now()` for `FetchedAt`.

**Rationale**: The function was extracted during ratchet-2 and has 61.3% coverage from the parent test. Dedicated unit tests will push coverage above 85% and bring CRAP below 15.

### 4. Whiteboard assertion restructuring

The current `TestGetWhiteboard` tests parse the JSON output string and assert on the parsed map. Gaze's contract mapper can't trace assertions through `json.Unmarshal` because the function return is `*mcp.CallToolResult` (an opaque struct with a `Content` field).

The fix: keep the JSON parsing but add explicit assertions on the parsed map's fields using direct map key access with type assertions, ensuring each side effect (embedded pages, connections, element count) has a dedicated `t.Errorf` call that Gaze can map to the function's observable outputs.

**Rationale**: The issue isn't assertion count — the test already has 15+ assertions. The issue is assertion structure. Gaze maps assertions to side effects by tracing the return value through the test. Assertions that go through helper functions or intermediate variables lose traceability. Direct `if parsed["embeddedPages"]` assertions are traceable.

### 5. Decomposition pattern for `(*DiskSource).Diff` (source/disk.go:121, complexity 15)

Extract two private functions matching the ratchet-1 pattern:
- `walkDiskFiles(basePath string) (map[string]string, error)` — walks the directory, skips hidden dirs and non-.md files, returns relPath → hash map
- `diffFileChanges(currentFiles, storedHashes map[string]string, fetcher func(string) (Document, error)) []Change` — compares hashes, fetches changed/new docs, returns categorized changes

The outer `Diff` becomes: walk → diff → return.

**Rationale**: This mirrors the `walkVault`/`diffPages` pattern from ratchet-1. The walk is pure I/O, the diff is pure comparison logic. Both are independently testable.

## Risks / Trade-offs

- **RenamePage lock scope**: All extracted methods run within the existing `c.mu.Lock()`. The methods themselves don't acquire locks, which is correct but means they can't be called from unlocked contexts. GoDoc must document this clearly.
- **Whiteboard assertion restructuring may not fully satisfy Gaze**: The mapper's traceability depends on its SSA analysis quality. If restructured assertions still don't register, the GazeCRAP score won't improve. Mitigated by verifying with `gaze quality` after changes.
- **indexDocuments test isolation**: The function calls `logger.Warn` for failures, which writes to stderr. Tests should redirect or accept this output. Not a blocker.
