# Research: Doctor Emoji Markers

**Branch**: `005-doctor-emoji-markers` | **Date**: 2026-03-30

## Research Questions

### Q1: What is the current `printCheck()` format?

**Answer**: The format string is at `cli.go:1124`:
```go
_, _ = fmt.Fprintf(w, "  [%s] %-20s%s\n", marker, name, description)
```

This produces lines like `  [PASS] vault                /tmp/vault`. The `marker` parameter is a string (`"PASS"`, `"WARN"`, or `"FAIL"`), the `name` is left-padded to 20 characters, and the `description` follows immediately.

### Q2: Does the summary box already use emoji?

**Answer**: Yes. `printSummaryBox()` at `cli.go:1148` already uses `✅`, `⚠️`, `❌` emoji in the summary line. The summary box is unchanged by this feature (FR-007).

### Q3: What is the `Fix:` hint indentation pattern?

**Answer**: All `Fix:` hints already use 5-space indent: `dp("     Fix: ...")`. This matches the spec requirement (FR-006). Examples at `cli.go:1189`, `1230`, `1238`, `1262`, `1301`, `1305`, `1316`, `1327`, `1354`. No change needed.

### Q4: How does `uf doctor` format its output?

**Answer**: Per the spec, `uf doctor` uses emoji markers (`✅`, `⚠️`, `❌`) with a space, then a padded name column, then a description. The `Fix:` hints use 5-space indent. This is the target format for `dewey doctor`.

### Q5: Are there any emoji rendering concerns?

**Answer**: The project already uses `github.com/mattn/go-runewidth` for the summary box width calculation. The check lines use `%-20s` for the name field (ASCII-only), so emoji width does not affect name column alignment. The emoji appears before the padded name field. This is the same approach the summary box uses successfully since v1.4.2.

### Q6: Which test functions need updating?

**Answer**: Three test functions assert on the `[PASS]`/`[WARN]`/`[FAIL]` text format:

1. `TestDoctorCounter_PrintCheck` (cli_test.go:2488-2494) — 3 assertions
2. `TestDoctorCmd_WithInitializedVault` (cli_test.go:2606, 2611) — 2 assertions
3. `TestDoctorCmd_MissingDeweyDir` (cli_test.go:2657) — 1 assertion

Total: 6 string assertions to update.

### Q7: Does the `printCheck` method signature change?

**Answer**: No. The method signature `func (c *doctorCounter) printCheck(w io.Writer, marker, name, description string)` is unchanged. Callers continue to pass `"PASS"`, `"WARN"`, `"FAIL"` as the marker string. The emoji mapping happens inside `printCheck()` (FR-009: single point of control).

## NEEDS CLARIFICATION Items

None. All technical questions are resolved. The change is fully understood.
