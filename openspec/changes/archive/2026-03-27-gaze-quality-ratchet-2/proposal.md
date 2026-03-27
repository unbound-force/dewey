## Why

The first quality ratchet (PR #5) reduced CRAPload from 15 to 13 and held GazeCRAPload at 34, matching the CI gate boundary. However, the decomposition of `newServer` created a new worst-offender: `registerHealthTool` at CRAP 67.5 with only 2.4% line coverage. Additionally, the GazeCRAPload gate (34) leaves zero headroom — any new complex function would fail CI.

Gaze v1.4.9 post-ratchet-1 report identifies 5 prioritized issues:

1. `registerHealthTool` (server.go:346) — CRAP 67.5, complexity 8, 2.4% coverage. Worst CRAP in the project, created by the ratchet-1 decomposition.
2. `newIndexCmd` (cli.go:513) — CRAP 28.6, complexity 19, 70.1% coverage. Monolithic CLI command.
3. `(*Semantic).Similar` (tools/semantic.go:88) — CRAP 23.1, complexity 22, 87% coverage, GazeCRAP 39.9 (Q4 Dangerous). Highest complexity function in the project.
4. Q3 navigate functions (`GetPage`, `GetBlock`, `ListPages`) — GazeCRAP 62.0, 100% line coverage but only 25% contract coverage.
5. `newStatusCmd` (cli.go:271) — CRAP 21.2, complexity 21, 92% coverage. Monolithic status formatting.

Target: reduce CRAPload from 13 to <=10 and GazeCRAPload from 34 to <=30, creating headroom for future development.

## What Changes

Address all 5 recommendations through targeted tests, decomposition, and contract assertion strengthening.

1. **Add tests for `registerHealthTool`** — test the health JSON response with nil store, nil embedder, non-nil store with sources, and non-nil embedder configurations. This is the highest-impact single fix (CRAP 67.5 → expected <15).
2. **Decompose `newIndexCmd`** — extract source loading, document indexing, and embedding generation into private helpers. Reduce complexity from 19 to <10 per function.
3. **Decompose `(*Semantic).Similar`** — extract input validation, query vector resolution (by UUID vs by page), and result filtering into private methods. Reduce complexity from 22 to <10 per method.
4. **Strengthen navigate test contracts** — add assertions to `TestGetPage`, `TestGetBlock`, `TestListPages` that verify returned block tree structure, link parsing, property extraction, and truncation behavior from mock data.
5. **Decompose `newStatusCmd`** — extract store querying and output formatting into private helpers. Reduce complexity from 21 to <10 per function.

## Capabilities

### New Capabilities
- None

### Modified Capabilities
- `health-tool`: `registerHealthTool` gains dedicated test coverage exercising all config combinations
- `index-command`: `newIndexCmd` split into helpers for source loading, document indexing, and embedding pass
- `semantic-similar`: `(*Semantic).Similar` split into input validation, vector resolution, and result filtering methods
- `status-command`: `newStatusCmd` split into store querying and output formatting helpers
- `navigate-test-contracts`: Existing navigate tests strengthened with structural assertions

### Removed Capabilities
- None

## Impact

| File | Change Type | Risk |
|------|-------------|------|
| `server.go` | Add test infrastructure (no production changes) | Minimal |
| `server_test.go` | Add `TestRegisterHealthTool_*` tests | Low — test-only |
| `cli.go` | Decompose `newIndexCmd` and `newStatusCmd` | Low — internal refactor |
| `tools/semantic.go` | Decompose `(*Semantic).Similar` | Low — internal refactor |
| `tools/navigate_test.go` | Strengthen contract assertions | Low — test-only |

No external API changes. No new dependencies. No behavioral changes. All 40 MCP tools produce identical results.

## Constitution Alignment

Assessed against the Unbound Force org constitution.

### I. Autonomous Collaboration

**Assessment**: N/A

Internal quality improvement. No MCP tool contracts, artifact formats, or inter-hero communication affected. All 40 tools continue to produce identical outputs.

### II. Composability First

**Assessment**: PASS

No new dependencies introduced. All refactoring is internal to existing packages. Dewey remains independently installable and usable without any other Unbound Force tool.

### III. Observable Quality

**Assessment**: PASS

Directly improves observable quality by reducing CRAPload (13→≤10) and GazeCRAPload (34→≤30). The `registerHealthTool` test gap is the most visible quality regression from ratchet-1, and fixing it demonstrates that decomposition-introduced coverage gaps are tracked and remediated.

### IV. Testability

**Assessment**: PASS

Improves testability. The `registerHealthTool` function is currently untestable in isolation because it registers a closure on the MCP server — the new tests exercise it through the server's tool-call interface. Decomposed CLI and semantic functions are individually testable.
