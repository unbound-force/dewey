## Why

The second quality ratchet (PR #6) reduced CRAPload from 13 to 10 and GazeCRAPload from 34 to 33, establishing headroom below the CI gates (15/34). However, 10 functions still exceed the CRAP threshold of 15, and 3 remain in the Q4 Dangerous quadrant. The worst CRAP scores are now dominated by complexity rather than coverage gaps, indicating the remaining work is decomposition-focused.

Five prioritized issues from the post-ratchet-2 Gaze report:

1. `(*Client).RenamePage` (vault/vault.go:981) — CRAP 22.6, complexity 17 (Q4 Dangerous). Despite 100% contract coverage, complexity alone drives the score above threshold.
2. `newSourceAddCmd` (cli.go:714) — CRAP 21.6, complexity 19. Monolithic CLI command with interleaved validation, config construction, and YAML writing.
3. `indexDocuments` (cli.go:624) — CRAP 15.8, complexity 10, 61.3% coverage. Extracted during ratchet-2 but lacks dedicated tests.
4. `(*Whiteboard).GetWhiteboard` (tools/whiteboard.go:90) — GazeCRAP 85.7 (worst in project), 100% line coverage but only 20% contract coverage. Existing tests assert through JSON output, which Gaze's mapper can't trace.
5. `(*DiskSource).Diff` (source/disk.go:121) — CRAP 15.7, complexity 15 (Q4 Dangerous), GazeCRAP 23.3.

Target: reduce CRAPload from 10 to <=7 and GazeCRAPload from 33 to <=28.

## What Changes

1. **Decompose `RenamePage`** — Extract file-rename I/O, link-update traversal, index-rebuild, and empty-directory cleanup into separate private methods.
2. **Decompose `newSourceAddCmd`** — Extract source config builders (`buildGitHubSource`, `buildWebSource`) and the save-with-dedup logic into private functions.
3. **Add tests for `indexDocuments`** — Dedicated tests with in-memory store verifying page insert, page update, source record creation, and properties marshaling.
4. **Restructure whiteboard test assertions** — Bypass JSON serialization and assert directly on the return map fields so Gaze's contract mapper can trace them.
5. **Decompose `(*DiskSource).Diff`** — Extract `walkDiskFiles` (filesystem scan) and `diffFileChanges` (hash comparison) matching the `walkVault`/`diffPages` pattern from ratchet-1.

## Capabilities

### New Capabilities
- None

### Modified Capabilities
- `vault-rename`: `RenamePage` split into file-rename, link-update, index-rebuild, and cleanup phases
- `source-add-command`: `newSourceAddCmd` refactored with per-type config builders and save helper
- `index-documents-coverage`: `indexDocuments` gains dedicated test coverage
- `whiteboard-test-contracts`: Whiteboard test assertions restructured for Gaze mapper visibility
- `disk-source-diff`: `DiskSource.Diff` split into walk and comparison phases

### Removed Capabilities
- None

## Impact

| File | Change Type | Risk |
| ---- | ----------- | ---- |
| `vault/vault.go` | Decompose `RenamePage` | Medium — touches rename logic with file I/O, requires careful phase extraction |
| `cli.go` | Decompose `newSourceAddCmd` | Low — internal refactor, no API change |
| `cli_test.go` | Add `indexDocuments` tests | Low — test-only |
| `tools/whiteboard_test.go` | Restructure assertions | Low — test-only, no production changes |
| `source/disk.go` | Decompose `Diff` | Low — internal refactor, well-established pattern |

No external API changes. No new dependencies. No behavioral changes. All 40 MCP tools produce identical results.

## Constitution Alignment

Assessed against the Unbound Force org constitution.

### I. Autonomous Collaboration

**Assessment**: N/A

Internal quality improvement. No MCP tool contracts or inter-hero communication affected.

### II. Composability First

**Assessment**: PASS

No new dependencies. All refactoring is internal to existing packages. Dewey remains independently installable.

### III. Observable Quality

**Assessment**: PASS

Directly improves observable quality by reducing CRAPload and GazeCRAPload, creating further headroom below CI gates.

### IV. Testability

**Assessment**: PASS

Decomposed functions are individually testable. The `indexDocuments` test gap is explicitly addressed. Whiteboard assertion restructuring improves Gaze's ability to measure contract coverage.
