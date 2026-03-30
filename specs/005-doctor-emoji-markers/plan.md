# Implementation Plan: Doctor Emoji Markers

**Branch**: `005-doctor-emoji-markers` | **Date**: 2026-03-30 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `specs/005-doctor-emoji-markers/spec.md`

## Summary

Replace `[PASS]`/`[WARN]`/`[FAIL]` text markers in `dewey doctor` check lines with emoji markers (`✅`/`⚠️`/`❌`) to match the `uf doctor` visual style. The change is confined to the `printCheck()` format string in `cli.go` and the corresponding test assertions in `cli_test.go`. The `Fix:` hint indentation is updated to 5-space indent to align under the description column. No diagnostic logic, check ordering, or summary box changes.

## Technical Context

**Language/Version**: Go 1.25 (per `go.mod`)
**Primary Dependencies**: `github.com/spf13/cobra` (CLI), `github.com/charmbracelet/log` (logging), `github.com/mattn/go-runewidth` (terminal width — already used by summary box)
**Storage**: N/A (no storage changes)
**Testing**: Standard library `testing` package, `go test -race -count=1 ./...`
**Target Platform**: darwin/linux (amd64/arm64)
**Project Type**: CLI tool / MCP server
**Performance Goals**: N/A (cosmetic output change)
**Constraints**: N/A
**Scale/Scope**: 2 files changed (`cli.go`, `cli_test.go`), ~15 lines of production code, ~10 lines of test assertions

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Assessment |
|-----------|--------|------------|
| **I. Composability First** | ✅ PASS | No new dependencies introduced. Dewey remains independently installable. The emoji markers are UTF-8 strings with no external requirements. |
| **II. Autonomous Collaboration** | ✅ PASS | No changes to MCP tools, tool schemas, or structured responses. This is a CLI-only cosmetic change. |
| **III. Observable Quality** | ✅ PASS | No changes to provenance metadata, health reporting, or index state. The `doctor` command continues to report the same diagnostic information with improved visual consistency. |
| **IV. Testability** | ✅ PASS | All existing tests will be updated to assert the new emoji format. No external services required. Tests run with `go test ./...` on a clean checkout. Coverage strategy: update existing `TestDoctorCounter_PrintCheck`, `TestDoctorCmd_WithInitializedVault`, and `TestDoctorCmd_MissingDeweyDir` assertions to match new format. |

**Gate result**: PASS — all four principles satisfied. No violations to justify.

## Project Structure

### Documentation (this feature)

```text
specs/005-doctor-emoji-markers/
├── plan.md              # This file
├── research.md          # Phase 0 output (minimal — change is well-understood)
├── quickstart.md        # Phase 1 output (implementation guide)
└── tasks.md             # Phase 2 output (/speckit.tasks command)
```

**Skipped artifacts**:
- `data-model.md` — N/A. No data model changes. This is a format-string-only change.
- `contracts/` — N/A. No new interfaces, APIs, or MCP tool contracts. The `printCheck()` method signature is unchanged.

### Source Code (repository root)

```text
cli.go                   # printCheck() format string + Fix: hint indent
cli_test.go              # Test assertion updates (3 test functions)
```

**Structure Decision**: No new files or directories. The change modifies two existing files at the repository root. This aligns with the existing flat package layout documented in AGENTS.md.

## Design

### Current Format (cli.go:1124)

```go
_, _ = fmt.Fprintf(w, "  [%s] %-20s%s\n", marker, name, description)
```

Produces:
```
  [PASS] vault                /tmp/vault
  [WARN] config.yaml          not found (using defaults)
  [FAIL] .dewey/              not found
```

### Target Format

```go
// Map marker string to emoji
var emoji string
switch marker {
case "PASS":
    emoji = "✅"
case "WARN":
    emoji = "⚠️"
case "FAIL":
    emoji = "❌"
}
_, _ = fmt.Fprintf(w, "  %s %-20s%s\n", emoji, name, description)
```

Produces:
```
  ✅ vault                /tmp/vault
  ⚠️ config.yaml          not found (using defaults)
  ❌ .dewey/              not found
```

### Fix: Hint Indentation

Current `Fix:` hints use a 5-space indent (`"     Fix: ..."`), which already matches the spec requirement (FR-006). The current code at lines like `cli.go:1189` already uses `dp("     Fix: ...")`. No change needed for Fix: hint indentation — it already aligns correctly.

### Column Width Consideration

The name column uses `%-20s` padding (20 characters). This is preserved unchanged per FR-005 — consistent alignment across all check lines regardless of name length. The emoji markers are narrower than `[PASS]` (6 chars) but the 2-space leading indent and the `%-20s` padding ensure alignment remains consistent.

### Emoji Width Note

The summary box (already using emoji since v1.4.2) uses `runewidth.StringWidth` for width calculation. The check lines use `fmt.Fprintf` with `%-20s` for the name field, which pads by byte count. Since the name field contains ASCII-only strings, this works correctly. The emoji is outside the padded field, so terminal rendering width of emoji does not affect column alignment.

### Test Updates Required

Three test functions need assertion updates:

1. **`TestDoctorCounter_PrintCheck`** (cli_test.go:2465-2500):
   - Change `[PASS] vault` → `✅ vault`
   - Change `[WARN] config.yaml` → `⚠️ config.yaml`
   - Change `[FAIL] graph.db` → `❌ graph.db`

2. **`TestDoctorCmd_WithInitializedVault`** (cli_test.go:2569-2638):
   - Change `[PASS] .dewey/` → `✅ .dewey/`
   - Change `[PASS] graph.db` → `✅ graph.db`

3. **`TestDoctorCmd_MissingDeweyDir`** (cli_test.go:2640-2681):
   - Change `[FAIL] .dewey/` → `❌ .dewey/`

### GoDoc Update

The `printCheck` GoDoc comment (cli.go:1109-1114) references `[PASS]` format. Update to reflect emoji format:

```go
// printCheck writes a formatted check line in the `uf doctor` style:
//
//	✅ name                description
//
// The name field is left-aligned and padded to 20 characters. The marker
// is one of PASS, WARN, or FAIL — the counter is incremented accordingly
// and the corresponding emoji (✅, ⚠️, ❌) is displayed.
```

## Coverage Strategy

**Approach**: Update existing test assertions to match the new format. No new test functions needed — the existing tests already cover the contract surface (counter increments, formatted output, integration with real vault).

**Tests affected**:
- `TestDoctorCounter_PrintCheck` — unit test for `printCheck()` format and counter logic
- `TestDoctorCmd_WithInitializedVault` — integration test for pass-state output
- `TestDoctorCmd_MissingDeweyDir` — integration test for fail-state output

**Coverage impact**: Zero — same lines covered, same assertions, different expected strings.

**CI validation**: `go test -race -count=1 ./...` + `gaze crap --max-crapload=15 --max-gaze-crapload=35 ./...`

## Complexity Tracking

No constitution violations. No complexity justifications needed.

## Post-Design Constitution Re-Check

| Principle | Status | Re-Assessment |
|-----------|--------|---------------|
| **I. Composability First** | ✅ PASS | Confirmed: no new dependencies. Emoji are UTF-8 string literals. |
| **II. Autonomous Collaboration** | ✅ PASS | Confirmed: no MCP tool changes. |
| **III. Observable Quality** | ✅ PASS | Confirmed: diagnostic information unchanged, only visual format updated. |
| **IV. Testability** | ✅ PASS | Confirmed: all tests updated in-place. No external services needed. Coverage strategy documented above. |
