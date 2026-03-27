## Why

Gaze v1.4.9 full quality report on `main` (2026-03-26) reveals 15 functions exceeding the CRAPload threshold (15) and a GazeCRAPload of 34, with 5 functions in the Q4 Dangerous quadrant. The CI gate enforces `--max-crapload=48 --max-gaze-crapload=18 --min-contract-coverage=70`, meaning the project currently **fails** the GazeCRAPload gate (34 > 18) and is below the contract coverage floor (61.6% < 70%).

Five prioritized issues identified:

1. `handleEvent` (vault/vault.go:318) — CRAP 65.4, complexity 16, 42% coverage. Highest CRAP score in the project.
2. `newServer` (server.go:40) — CRAP 44.3, complexity 20, 61% coverage. Monolithic tool registration.
3. `IncrementalIndex` (vault/vault_store.go:229) — GazeCRAP 97.0, complexity 19, 40% contract coverage. Worst GazeCRAP in the project.
4. `UnmarshalJSON` methods in `types/logseq.go` — all 6 side effects classified as ambiguous due to missing GoDoc signals.
5. Q3 functions in `tools/` — 29 functions with low contract coverage despite adequate line coverage, concentrated in `whiteboard.go`, `journal.go`, `decision.go`, and `search.go`.

Without this change, adding any new code is likely to push CRAPload and GazeCRAPload further above the CI thresholds, blocking future PRs.

## What Changes

Reduce CRAPload and GazeCRAPload through targeted decomposition, test contract strengthening, and GoDoc improvements — without changing any external behavior.

1. **Decompose high-complexity functions** — Extract per-event handlers from `handleEvent`, extract per-category tool registration helpers from `newServer`, and split `IncrementalIndex` into walk/diff/persist phases.
2. **Add GoDoc comments** to `types/logseq.go` UnmarshalJSON methods to provide the `godoc` signal Gaze needs to classify their side effects as contractual.
3. **Strengthen test contracts** for the worst Q3 functions in `tools/` by adding assertions that verify observable side effects (return values, error conditions, mutation results) rather than just exercising code paths.

## Capabilities

### New Capabilities
- None

### Modified Capabilities
- `vault-event-handling`: `handleEvent` split into `handleCreate`, `handleRemove`, `handleRename` per-event methods with shared helper for store persistence
- `server-tool-registration`: `newServer` refactored to use per-category registration helpers (`registerNavigateTools`, `registerSearchTools`, etc.)
- `vault-incremental-indexing`: `IncrementalIndex` split into `walkVault`, `diffPages`, and `persistChanges` phases
- `types-classification`: UnmarshalJSON GoDoc comments enhanced to provide Gaze classifier signal
- `tools-test-contracts`: Existing tests in `tools/whiteboard_test.go`, `tools/journal_test.go`, `tools/decision_test.go`, `tools/search_test.go` strengthened with behavioral assertions

### Removed Capabilities
- None

## Impact

Files affected:

| File | Change Type | Risk |
|------|-------------|------|
| `vault/vault.go` | Decompose `handleEvent` into per-event methods | Low — internal refactor, no API change |
| `server.go` | Extract tool registration helpers from `newServer` | Low — internal refactor, no API change |
| `vault/vault_store.go` | Decompose `IncrementalIndex` into phases | Medium — touches indexing logic, must verify identical behavior |
| `types/logseq.go` | Add GoDoc comments only | Minimal — documentation only |
| `tools/whiteboard_test.go` | Add contract assertions | Low — test-only |
| `tools/journal_test.go` | Add contract assertions | Low — test-only |
| `tools/decision_test.go` | Add contract assertions | Low — test-only |
| `tools/search_test.go` | Add contract assertions | Low — test-only |

No external API changes. No new dependencies. No behavioral changes. All 40 MCP tools must produce identical results before and after.

## Constitution Alignment

Assessed against the Unbound Force org constitution.

### I. Autonomous Collaboration

**Assessment**: N/A

This change is an internal quality improvement. No MCP tool contracts, artifact formats, or inter-hero communication is affected. All 40 tools continue to produce identical outputs.

### II. Composability First

**Assessment**: PASS

No new dependencies introduced. All refactoring is internal to existing packages. Dewey remains independently installable and usable without any other Unbound Force tool. The optional persistence and embedding patterns are preserved.

### III. Observable Quality

**Assessment**: PASS

This change directly improves observable quality. Gaze metrics are the project's quality measurement system. Reducing CRAPload from 15 to target <=12 and GazeCRAPload from 34 to target <=18 moves the project within CI gate thresholds. Contract coverage is expected to increase from 61.6% toward the 70% floor.

### IV. Testability

**Assessment**: PASS

This change improves testability. Decomposed functions are individually testable in isolation. Strengthened test contracts verify observable side effects rather than just exercising code paths. No external services required — all tests use in-memory SQLite, httptest, and t.TempDir().
