# Proposal: doctor-ux-improvement

## Why

`dewey doctor` output does not match the style, format, and look-and-feel of `uf doctor` from the parent `unbound-force` CLI. This creates a jarring experience when users run both tools — `uf doctor` has consistent column alignment, human-readable descriptions, a summary box with pass/warn/fail counts, and single-line fix hints, while `dewey doctor` has inconsistent spacing, raw paths as values, a neutral `[    ]` marker that `uf doctor` doesn't use, and no summary.

Since Dewey is part of the Unbound Force ecosystem and `uf doctor` already checks Dewey's health, the two doctor outputs should feel like they come from the same family.

## What Changes

- Rewrite `dewey doctor` output formatting to match `uf doctor` style
- Remove implementation details from output (WAL/SHM files, raw byte counts)
- Add summary box with pass/warn/fail counts
- Standardize column alignment and check descriptions
- Remove duplicate lock status reporting

## Capabilities

### New Capabilities
- None

### Modified Capabilities
- `cli-doctor`: Output format updated to match `uf doctor` style — consistent column alignment, human-readable descriptions with paths in parens, summary box, no neutral `[    ]` marker

### Removed Capabilities
- None

## Impact

- **`cli.go`**: Rewrite `runDoctorChecks()` output formatting
- **`cli_test.go`**: Update doctor test assertions for new output format
- **User impact**: Visually consistent doctor output across the Unbound Force tool chain

## Constitution Alignment

### I. Autonomous Collaboration
**Assessment**: N/A — UI formatting only.

### II. Composability First
**Assessment**: PASS — Dewey doctor remains independently usable. The formatting change aligns it with the ecosystem but doesn't create dependencies on `uf`.

### III. Observable Quality
**Assessment**: PASS — Improved readability and the summary box make the diagnostic output more observable and actionable.

### IV. Testability
**Assessment**: PASS — Tests updated to match new output format.
