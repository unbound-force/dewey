# Feature Specification: Doctor Emoji Markers

**Feature Branch**: `005-doctor-emoji-markers`
**Created**: 2026-03-29
**Status**: Draft
**Input**: User description: "Replace dewey doctor text markers with emoji markers to match uf doctor style"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Consistent Diagnostic Output Across Unbound Force Tools (Priority: P1)

A developer using the Unbound Force ecosystem runs both `uf doctor` and `dewey doctor` during troubleshooting. Today, `uf doctor` displays check results with emoji markers (`✅`, `⚠️`, `❌`) while `dewey doctor` uses text markers (`[PASS]`, `[WARN]`, `[FAIL]`). The visual inconsistency makes the outputs feel like they come from different tool families. The developer expects the same look and feel across all Unbound Force diagnostic commands.

**Why this priority**: This is the entire scope of the change — the visual format of check lines is the only thing being modified.

**Independent Test**: Run `dewey doctor --vault .` and visually confirm all check lines use emoji markers instead of text markers, matching the column layout of `uf doctor`.

**Acceptance Scenarios**:

1. **Given** a fully initialized vault with Ollama running, **When** the user runs `dewey doctor`, **Then** all passing checks display with `✅` prefix, not `[PASS]`
2. **Given** a vault missing `config.yaml`, **When** the user runs `dewey doctor`, **Then** the missing config check displays with `⚠️` prefix, not `[WARN]`
3. **Given** a vault without `.dewey/` directory, **When** the user runs `dewey doctor`, **Then** the initialization check displays with `❌` prefix followed by a `Fix:` hint, not `[FAIL]`
4. **Given** any system state, **When** the user runs `dewey doctor`, **Then** all check name columns align consistently and descriptions start at the same position, matching the `uf doctor` column layout

---

### User Story 2 - Scannable Fix Hints (Priority: P2)

When a check fails or warns, the developer needs to quickly identify what to fix. `uf doctor` places `Fix:` hints on the line below the check with a 5-space indent, aligning under the description column. `dewey doctor` should follow the same indentation pattern so developers can scan for fixable items using the same visual pattern across tools.

**Why this priority**: Fix hint formatting is secondary to the marker format itself but still affects the visual consistency.

**Independent Test**: Run `dewey doctor` in a state that produces warnings or failures and confirm `Fix:` lines use the same indentation as `uf doctor`.

**Acceptance Scenarios**:

1. **Given** `.dewey/` does not exist, **When** the user runs `dewey doctor`, **Then** the `Fix:` hint appears on the next line with 5-space indent (`     Fix: dewey init ...`)
2. **Given** Ollama is not running, **When** the user runs `dewey doctor`, **Then** the `Fix:` hint for starting Ollama uses the same 5-space indent pattern

---

### Edge Cases

- What happens when the terminal does not support Unicode/emoji? Output will contain raw UTF-8 bytes. This is acceptable — the same constraint applies to `uf doctor` and the existing summary box already uses emoji.
- What happens when output is piped to a file? Emoji bytes are written as UTF-8 to the file. No special handling needed — same behavior as the existing summary box.
- What happens with the summary box? The summary box at the bottom already uses emoji (`✅`, `⚠️`, `❌`). No change needed for the summary box itself.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Check lines MUST use `✅` as the pass marker instead of `[PASS]`
- **FR-002**: Check lines MUST use `⚠️` as the warning marker instead of `[WARN]`
- **FR-003**: Check lines MUST use `❌` as the failure marker instead of `[FAIL]`
- **FR-004**: Check line format MUST be `  {emoji} {name padded to column width}{description}` — matching the `uf doctor` column layout where the name and description form two distinct visual columns
- **FR-005**: The name column width MUST produce consistent alignment across all check lines regardless of name length
- **FR-006**: `Fix:` hint lines MUST use 5-space indent to align under the description column start
- **FR-007**: The summary box at the bottom MUST remain unchanged (it already uses emoji)
- **FR-008**: All existing tests MUST be updated to assert the new emoji format instead of text markers
- **FR-009**: The `doctorCounter` struct and `printCheck` method MUST remain the single point of control for check line formatting
- **FR-010**: Fix hints MUST NOT include the `--no-embeddings` flag — `dewey reindex` without flags is the correct remediation so users get both pages and embeddings
- **FR-011**: Section headings (e.g., "Environment", "Database") MUST render with lipgloss bold styling, matching `uf doctor`'s `boldStyle.Render(group.Name)` pattern in `internal/doctor/format.go`

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Running `dewey doctor` produces zero instances of `[PASS]`, `[WARN]`, or `[FAIL]` text markers in the output
- **SC-002**: Every check line in `dewey doctor` output begins with one of `✅`, `⚠️`, or `❌` followed by a space and a consistently-aligned name column
- **SC-003**: A side-by-side visual comparison of `uf doctor` and `dewey doctor` shows the same marker style, column alignment pattern, and `Fix:` hint indentation
- **SC-004**: All existing tests pass after the format change with updated assertions

## Assumptions

- The `uf doctor` format is the canonical reference. The exact column width (currently 20 characters in `dewey doctor`) may be adjusted to match `uf doctor`'s visual alignment, but consistent alignment across all check lines is the requirement.
- Terminal emoji rendering width varies, but this is already an accepted constraint — the summary box has used emoji since v1.4.2 with `runewidth.StringWidth` for width calculation.
- This change primarily affects the `printCheck()` format string and test assertions. One Fix hint content correction is included (removing misleading `--no-embeddings` flag).

## Clarifications

### Session 2026-03-30

- Q: Why does the Fix hint suggest `dewey reindex --no-embeddings`? → A: It shouldn't. The `--no-embeddings` flag makes reindex faster but skips embedding generation, which users typically want. Changed to just `dewey reindex` (FR-010).
- Q: Should section headings be visually lighter than check lines? → A: Use lipgloss bold styling (same as `uf doctor`'s `boldStyle.Render()` in `internal/doctor/format.go`), not ANSI dim. Bold headings with emoji check lines creates the same visual hierarchy as `uf doctor` (FR-011).
