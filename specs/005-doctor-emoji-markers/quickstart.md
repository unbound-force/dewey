# Quickstart: Doctor Emoji Markers

**Branch**: `005-doctor-emoji-markers` | **Date**: 2026-03-30

## What Changes

The `dewey doctor` command output changes from text markers to emoji markers:

**Before**:
```
  [PASS] vault                /tmp/vault
  [WARN] config.yaml          not found (using defaults)
  [FAIL] .dewey/              not found
     Fix: dewey init --vault /tmp/vault
```

**After**:
```
  ✅ vault                /tmp/vault
  ⚠️ config.yaml          not found (using defaults)
  ❌ .dewey/              not found
     Fix: dewey init --vault /tmp/vault
```

## Files to Modify

| File | Change |
|------|--------|
| `cli.go` | Update `printCheck()` format string (line 1124) and GoDoc comment (lines 1109-1114) |
| `cli_test.go` | Update 6 string assertions across 3 test functions |

## Implementation Steps

### Step 1: Update `printCheck()` in `cli.go`

Replace the format string at line 1124:

```go
// Before:
_, _ = fmt.Fprintf(w, "  [%s] %-20s%s\n", marker, name, description)

// After:
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

Note: The switch for counter increment already exists above this line. The emoji mapping can be added to the same switch or as a separate mapping. Using the same switch keeps the logic co-located.

### Step 2: Update GoDoc comment

Update the `printCheck` GoDoc (lines 1109-1114) to show the new format.

### Step 3: Update test assertions in `cli_test.go`

Update 6 assertions:
- `[PASS] vault` → `✅ vault`
- `[WARN] config.yaml` → `⚠️ config.yaml`
- `[FAIL] graph.db` → `❌ graph.db`
- `[PASS] .dewey/` → `✅ .dewey/`
- `[PASS] graph.db` → `✅ graph.db`
- `[FAIL] .dewey/` → `❌ .dewey/`

### Step 4: Validate

```bash
go build ./...
go vet ./...
go test -race -count=1 ./...
```

## What Does NOT Change

- The `doctorCounter` struct
- The `printCheck()` method signature
- The `printSummaryBox()` function (already uses emoji)
- The `Fix:` hint indentation (already 5-space)
- All diagnostic logic, check ordering, and section structure
- Any MCP tools or backend interfaces
