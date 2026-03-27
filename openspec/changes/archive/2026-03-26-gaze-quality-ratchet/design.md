## Context

The Gaze v1.4.9 quality report identifies 15 functions exceeding the CRAP threshold (15) and a GazeCRAPload of 34 against a CI gate of 18. Contract coverage sits at 61.6% against a gate of 70%. The project cannot merge PRs until these metrics are brought within thresholds.

The five highest-impact issues are concentrated in three packages: `vault/` (handleEvent, IncrementalIndex), `main` (newServer), and `tools/` (Q3 underspecification). The `types/` package has a documentation gap affecting classifier accuracy.

Per the proposal's constitution alignment: this change is N/A for Autonomous Collaboration, PASS for Composability First, Observable Quality, and Testability.

## Goals / Non-Goals

### Goals
- Reduce CRAPload from 15 to <=12 by decomposing the 5 worst-scoring functions
- Reduce GazeCRAPload from 34 to <=18 by improving contract coverage on Q3 functions
- Increase average contract coverage from 61.6% to >=70% to meet CI gate
- Improve classification accuracy for `types/logseq.go` from ambiguous to contractual via GoDoc

### Non-Goals
- Changing external MCP tool behavior or API contracts
- Adding new test infrastructure, frameworks, or assertion libraries
- Refactoring packages not flagged by Gaze (graph/, parser/, store/, embed/)
- Addressing CRAPload functions below the top 5 (e.g., `newSourceAddCmd`, `newStatusCmd`)
- Achieving 100% contract coverage — target is the 70% floor

## Decisions

### 1. Decomposition pattern for `handleEvent` (vault/vault.go:318)

Extract the switch-case body into three private methods: `handleFileWrite(relPath, absPath)`, `handleFileRemove(relPath)`, `handleFileRename(relPath)`. The shared pre-checks (non-.md skip, hidden dir skip, relative path computation) remain in `handleEvent` as a dispatcher. Each handler receives the computed `relPath` and the absolute path where needed.

**Rationale**: The complexity comes from three event-type branches each doing file I/O, index updates, and optional store persistence. Splitting by event type maps to the natural domain boundary and makes each handler independently testable. The shared store-persistence logic (`persistToStore`) is extracted as a private helper since it appears in both create/write and rename flows.

### 2. Decomposition pattern for `newServer` (server.go:40)

Extract tool registration into category-specific private functions: `registerNavigateTools(srv, nav, hasDataScript)`, `registerSearchTools(srv, search, hasDataScript)`, `registerAnalyzeTools(srv, analyze)`, `registerWriteTools(srv, write, readOnly)`, `registerDecisionTools(srv, decision, readOnly)`, `registerJournalTools(srv, journal)`, `registerFlashcardTools(srv, flashcard, hasDataScript, readOnly)`, `registerWhiteboardTools(srv, whiteboard, hasDataScript)`, `registerSemanticTools(srv, semantic)`, `registerHealthTool(srv, health)`.

**Rationale**: The function is a 360-line sequence of `mcp.AddTool` calls grouped by category with comments. The groups are already logically separated — extracting them into functions matches the existing structure. Each registration function is pure (no side effects beyond `mcp.AddTool`) and testable by verifying tool count on the returned server.

### 3. Decomposition pattern for `IncrementalIndex` (vault/vault_store.go:229)

Split into three phases: `walkVault(vaultPath) → (currentFiles, fileContents, error)`, `diffPages(currentFiles, storedHashes) → (newPages, changedPages, deletedPages)`, and the persist/cleanup logic remains in `IncrementalIndex` as the orchestrator. The walk and diff functions are pure and independently testable.

**Rationale**: The 115-line function has three clear algorithmic phases already marked by comments ("Step 1/2/3/4/5"). Extracting walk and diff preserves the orchestration structure while reducing `IncrementalIndex` to a coordination function. The GazeCRAP score of 97.0 is primarily driven by the combination of complexity 19 and 40% contract coverage — splitting enables targeted assertions on each phase.

### 4. GoDoc enhancement for `types/logseq.go`

Add GoDoc comments to `(*PageRef).UnmarshalJSON` and `(*BlockRef).UnmarshalJSON` that explicitly state the interface contract: "implements json.Unmarshaler" and documents the dual-format handling. The existing `(*BlockEntity).UnmarshalJSON` already has adequate GoDoc but is missing the standard `implements` phrasing.

**Rationale**: Gaze's classifier assigns a +15 `godoc` signal weight when GoDoc comments explicitly describe the function's contract. The current ambiguous classification (confidence 54) with contradiction penalty (-20) means the godoc signal would boost confidence to 69, and the `ai_pattern` signal (+10 for recognizable Unmarshaler interface) would push it to ~79 — borderline but combined with the removed contradiction penalty (interface implementation removes the contradiction), it reaches contractual threshold (>=80).

### 5. Q3 contract assertion strategy for `tools/`

Focus on the 5 worst GazeCRAP Q3 functions identified in the report:

| Function | GazeCRAP | Package |
|----------|---------|---------|
| `GetWhiteboard` | 85.7 | tools/whiteboard.go:90 |
| `JournalSearch` | 72.8 | tools/journal.go:120 |
| `AnalysisHealth` | 72.1 | tools/decision.go:339 |
| `FindByTag` | 62.0 | tools/search.go:193 |
| `TopicClusters` | 27.0 | graph/algorithms.go:220 |

For each, the existing tests call the function and check for non-error return, but don't assert on the return value structure. Add assertions that verify:
- Return value contains expected fields populated from mock data
- Error conditions return appropriate errors (not just `nil`)
- Mutation side effects (for decision tools) are reflected in backend state

**Rationale**: These functions have adequate line coverage (84-100%) but low contract coverage (20-33%). The fix is assertion strengthening, not more test cases. Each assertion targets a specific side effect that Gaze classifies.

## Risks / Trade-offs

- **Decomposition boundary risk**: Extracting methods from `handleEvent` and `IncrementalIndex` changes the call graph, which could affect Gaze's inter-procedural analysis. Mitigated by running `gaze crap ./...` after each decomposition to verify CRAP improvement.
- **Test assertion false positives**: Adding assertions to existing tests may expose pre-existing bugs in the mock backend behavior. Mitigated by running the full test suite before and after changes and investigating any new failures.
- **GoDoc signal uncertainty**: The exact classifier weight for GoDoc-enhanced scoring depends on Gaze's internal model. The +15 weight is documented but actual improvement may vary. Mitigated by verifying classification output after changes.
- **Scope creep**: The Q3 function list includes `TopicClusters` in `graph/algorithms.go` — a package otherwise not in scope. This single function is included because it appears in the Gaze worst-GazeCRAP list, but no other `graph/` functions are modified.
