# Implementation Plan: Quality Ratchets

**Branch**: `002-quality-ratchets` | **Date**: 2026-03-24 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/002-quality-ratchets/spec.md`

## Summary

Enforce Gaze quality thresholds in CI with ratcheted baselines, decompose the 4 highest-CRAP functions (CRAP 306-650), add tests and strengthen assertions to bring GazeCRAPload from 37 → ≤10, and improve module-wide average contract coverage from 56.5% → ≥80%. US1 (CI integration) is already implemented; the remaining work is US2 (decomposition + tests) and US3 (assertion strengthening + GoDoc improvements). GoDoc improvements are in scope to help Gaze's effect classifier distinguish contractual effects from ambiguous ones (FR-014).

## Technical Context

**Language/Version**: Go 1.25 (per `go.mod`)
**Primary Dependencies**: Gaze v1.4.6 (`go install github.com/unbound-force/gaze/cmd/gaze@latest`)
**Storage**: N/A (quality improvement, no storage changes)
**Testing**: `go test -race -count=1 -coverprofile=coverage.out ./...`
**Target Platform**: GitHub Actions CI (`.github/workflows/ci.yml`)
**Project Type**: MCP server + CLI tool (quality improvement, no new features)
**Performance Goals**: N/A (no runtime changes)
**Constraints**: No new production code dependencies. Production code changes limited to function decomposition (US2) and GoDoc improvements (US3). No changes to MCP tool behavior or contracts.
**Scale/Scope**: 355 total functions analyzed, 48 above CRAP threshold, 37 above GazeCRAP threshold, 56.5% contract coverage

### Current Baseline (measured 2026-03-24)

| Metric | Current | CI Gate | Target |
|--------|--------:|--------:|-------:|
| CRAPload | 48 | ≤48 | ≤53 (already met) |
| GazeCRAPload | 37 | ≤37 | ≤10 |
| Contract Coverage | 56.5% | ≥8% | ≥80% |
| Q1 (Safe) | 106 | — | Maximize |
| Q2 (Complex But Tested) | 1 | — | — |
| Q3 (Simple But Underspecified) | 32 | — | ≤10 |
| Q4 (Dangerous) | 5 | — | 0 |

### Highest-Priority Targets

**4 `decompose_and_test` functions (CRAP > 300):**

| Function | CRAP | Complexity | Coverage | File |
|----------|-----:|----------:|---------:|------|
| `executeServe` | 650 | 25 | 0% | main.go:116 |
| `createSource` | 506 | 22 | 0% | source/manager.go:53 |
| `(*Client).MoveBlock` | 462 | 21 | 0% | vault/vault.go:1097 |
| `(*Navigate).ListPages` | 306 | 17 | 0% | tools/navigate.go:114 |

**5 Q4 (Dangerous) functions — highest GazeCRAP:**

| Function | GazeCRAP | Contract Cov | File |
|----------|--------:|-----------:|------|
| `(*Semantic).Similar` | 165 | 33% | tools/semantic.go:72 |
| `(*VaultStore).IncrementalIndex` | 97 | 40% | vault/vault_store.go:229 |
| `(*Whiteboard).GetWhiteboard` | 86 | 20% | tools/whiteboard.go:90 |
| `(*DiskSource).Diff` | 82 | 33% | source/disk.go:108 |
| `(*Journal).JournalSearch` | 73 | 25% | tools/journal.go:120 |

**Top Q3 functions (by GazeCRAP, needing assertion strengthening):**

| Function | GazeCRAP | Contract Cov | File |
|----------|--------:|-----------:|------|
| `NewWebSource` | 42 | 0% | source/web.go:56 |
| `(*TopicClusters)` | 27 | 33% | graph/algorithms.go:205 |
| `(*GitHubSource).List` | 27 | 33% | source/github.go:104 |
| `(*WebSource).List` | 27 | 40% | source/web.go:96 |
| `client.New` | 20 | 0% | client/logseq.go:32 |
| `store.New` | 19 | 50% | store/store.go:34 |
| `(*Manager).FetchAll` | 19 | 67% | source/manager.go:128 |

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

### I. Composability First -- PASS

No changes to Dewey's runtime behavior. Quality tooling is a development-time concern. Dewey remains independently installable and usable without Gaze.

### II. Autonomous Collaboration -- PASS

No changes to MCP tools or their contracts. All 40 tools continue to function identically. Function decompositions preserve existing behavior (FR-009).

### III. Observable Quality -- PASS

This feature directly serves Observable Quality by making quality metrics machine-enforceable in CI. The ratchet mechanism ensures quality is auditable and monotonically non-decreasing. GoDoc improvements (FR-014) enhance observability of function contracts.

### IV. Testability -- PASS

This feature improves testability by:
- Adding tests to untested functions (US2)
- Strengthening contract assertions on existing tests (US3)
- Decomposing high-complexity functions into testable units (US2)
- Enforcing coverage ratchets in CI (US1, already done)

Constitution §IV explicitly requires: "Coverage ratchets MUST be enforced by automated checks in CI." This feature implements that requirement.

**Pre-design gate result: ALL PASS. No violations to justify.**

## Project Structure

### Documentation (this feature)

```text
specs/002-quality-ratchets/
├── plan.md              # This file
├── research.md          # Phase 0 output (5 decisions documented)
├── spec.md              # Feature specification (3 user stories, 14 FRs, 7 SCs)
├── tasks.md             # Task list (38 tasks, to be regenerated)
└── checklists/          # Validation checklists
```

### Source Code (repository root)

```text
# CI configuration (US1 — already done)
.github/workflows/ci.yml        # Gaze threshold gate step

# Production code changes (US2 — decomposition)
main.go                          # executeServe → extracted helpers
source/manager.go                # createSource → per-type factory functions
vault/vault.go                   # MoveBlock → extracted helpers
tools/navigate.go                # ListPages → extracted helpers

# Production code changes (US3 — GoDoc improvements)
# GoDoc comments improved on exported functions across all packages
# to help Gaze classify effects as contractual vs ambiguous

# Test files (US2 — new tests for untested functions)
tools/mock_backend_test.go       # Shared mock backend (already exists, 355 lines)
tools/decision_test.go           # Add DecisionResolve, DecisionDefer tests
tools/decision_tool_test.go      # Existing (14 tests)
tools/write_test.go              # Add MoveBlock tests
tools/navigate_test.go           # Add ListPages tests
tools/navigate_tool_test.go      # Existing (13 tests)
client/logseq_test.go            # Existing (14 tests)

# Test files (US3 — assertion strengthening)
tools/semantic_test.go           # Strengthen for Q4: Similar, Search, SearchFiltered
vault/vault_store_test.go        # Strengthen for Q4: IncrementalIndex
source/disk_test.go              # Strengthen for Q4: Diff
tools/whiteboard_test.go         # Strengthen for Q3: GetWhiteboard
tools/journal_test.go            # Strengthen for Q3: JournalSearch
tools/journal_tool_test.go       # Strengthen for Q3: JournalSearch
source/web_test.go               # Strengthen for Q3: NewWebSource, List
source/github_test.go            # Strengthen for Q3: List
source/manager_test.go           # Strengthen for Q3: FetchAll
store/store_test.go              # Strengthen for Q3: New
store/embeddings_test.go         # Strengthen for Q3: SearchSimilar
graph/algorithms_test.go         # Strengthen for Q3: TopicClusters
```

**Structure Decision**: No new directories or packages. All changes are to existing files. US2 modifies 4 production files (decomposition) and adds tests to existing test files. US3 modifies GoDoc comments on exported functions and strengthens assertions in existing test files.

## Decomposition Strategy

### executeServe (main.go) — CRAP 650, complexity 25

Extract into 4-5 focused helper functions:

| Extracted Function | Responsibility | Target Complexity |
|-------------------|----------------|------------------:|
| `initBackend` | Backend selection (logseq vs vault) and creation | ≤8 |
| `initStore` | SQLite store creation, path resolution, `.dewey/` directory | ≤6 |
| `initEmbedder` | Ollama embedder creation, model config | ≤4 |
| `startMCPServer` | MCP server creation, tool registration, transport start | ≤8 |
| `indexAndServe` | Orchestrate index + serve with embedder wiring | ≤6 |

### createSource (source/manager.go) — CRAP 506, complexity 22

Extract per-type factory functions:

| Extracted Function | Responsibility | Target Complexity |
|-------------------|----------------|------------------:|
| `createDiskSource` | DiskSource creation with config validation | ≤6 |
| `createGitHubSource` | GitHubSource creation with token resolution | ≤8 |
| `createWebSource` | WebSource creation with URL validation, cache dir | ≤8 |

### (*Client).MoveBlock (vault/vault.go) — CRAP 462, complexity 21

Extract validation and tree manipulation helpers:

| Extracted Function | Responsibility | Target Complexity |
|-------------------|----------------|------------------:|
| `validateMoveTarget` | Validate target page/block exists | ≤5 |
| `detachBlock` | Remove block from current parent's children | ≤6 |
| `attachBlock` | Insert block into target parent at position | ≤6 |
| `updateBlockPositions` | Recalculate sibling positions after move | ≤4 |

### (*Navigate).ListPages (tools/navigate.go) — CRAP 306, complexity 17

Extract filtering and sorting logic:

| Extracted Function | Responsibility | Target Complexity |
|-------------------|----------------|------------------:|
| `filterPagesByTag` | Apply tag filter to page list | ≤5 |
| `filterPagesByProperty` | Apply property filter to page list | ≤5 |
| `sortAndPaginatePages` | Apply sort field, direction, offset, limit | ≤7 |

## Contract Coverage Strategy

To move from 56.5% → ≥80%, two complementary approaches:

### 1. Assertion Strengthening (test changes)

For each Q3/Q4 function, add assertions that verify:
- **Return values**: Check the actual returned data, not just that no error occurred
- **Error conditions**: Verify specific error messages or types for invalid inputs
- **Side effects**: Verify state mutations (e.g., pages inserted, blocks moved)
- **Result ordering**: Verify semantic search results are ranked by similarity
- **Provenance metadata**: Verify source type, timestamps, document IDs in results

Example pattern for a Q3 function:
```go
// BEFORE (exercises code but doesn't verify behavior):
result, err := semantic.Search(ctx, req)
if err != nil { t.Fatal(err) }

// AFTER (verifies observable contract):
result, err := semantic.Search(ctx, req)
if err != nil { t.Fatal(err) }
if len(results) != 3 { t.Errorf("want 3 results, got %d", len(results)) }
if results[0].Score < results[1].Score { t.Error("results not ranked by score") }
if results[0].Source != "disk" { t.Errorf("want source disk, got %s", results[0].Source) }
```

### 2. GoDoc Improvements (production code, no behavioral change)

Improve GoDoc comments on exported functions to help Gaze's effect classifier:

```go
// BEFORE (ambiguous to classifier):
// New creates a new store.
func New(path string) (*Store, error) {

// AFTER (contractual signals for classifier):
// New creates a new Store backed by a SQLite database at the given path.
// Returns an error if the database cannot be opened or the schema migration fails.
// The returned Store must be closed with Close() when no longer needed.
func New(path string) (*Store, error) {
```

The classifier looks for:
- "Returns" / "returns" — signals a return value effect
- "error if" — signals an error condition effect
- "must be closed" / "must be called" — signals a lifecycle side effect
- Parameter documentation — signals input validation effects

## Verification Strategy

1. **Before each change**: Run `gaze crap --format=json --coverprofile=coverage.out ./...` to establish local baseline
2. **After each decomposition**: Run `go test -race -count=1 ./...` to verify no regressions
3. **After each test batch**: Re-run Gaze to verify metrics improve monotonically
4. **Final verification**: Run full Gaze report with target thresholds:
   ```bash
   gaze report ./... --coverprofile=coverage.out \
     --max-crapload=48 \
     --max-gaze-crapload=10 \
     --min-contract-coverage=80
   ```
5. **Tighten CI**: Update `.github/workflows/ci.yml` thresholds to achieved values

## Ratchet Tightening Schedule

| Phase | CRAPload Gate | GazeCRAPload Gate | Contract Cov Gate |
|-------|:------------:|:-----------------:|:-----------------:|
| Current (US1 done) | ≤48 | ≤37 | ≥8 |
| After US2 (decompose + test) | ≤(achieved) | ≤(achieved) | ≥(achieved) |
| After US3 (assertions + GoDoc) | ≤(achieved) | ≤10 | ≥80 |

Each phase tightens the ratchet to the achieved value, locking in gains permanently.

## Complexity Tracking

No complexity beyond constitutional principles. This feature adds tests, strengthens assertions, decomposes production functions, and improves GoDoc — no architectural decisions or trade-offs. All production code changes preserve existing behavior (FR-009).
