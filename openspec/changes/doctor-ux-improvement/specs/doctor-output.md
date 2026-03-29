# Doctor Output Format Spec

## ADDED Requirements

_None_

## MODIFIED Requirements

### Requirement: Doctor Output Format Consistency

The `dewey doctor` command MUST produce output that follows the same visual style as `uf doctor`:

- Check lines use `[PASS]`, `[WARN]`, or `[FAIL]` markers only (no `[    ]`)
- Check names are left-padded to a consistent column width
- Descriptions are human-readable with paths in parentheses
- Fix hints use a single line with consistent indentation
- A summary box at the bottom shows pass/warn/fail counts

#### Scenario: Healthy system
- **GIVEN** all prerequisites are met
- **WHEN** the user runs `dewey doctor --vault .`
- **THEN** the output ends with a summary box showing pass count and zero warnings/failures

#### Scenario: Missing Ollama model
- **GIVEN** Ollama is running but the model is not pulled
- **WHEN** the user runs `dewey doctor --vault .`
- **THEN** the model check shows `[FAIL]` with a single-line `Fix: ollama pull granite-embedding:30m`

#### Scenario: Output alignment
- **GIVEN** any system state
- **WHEN** the user runs `dewey doctor --vault .`
- **THEN** all check names align to the same column width and all descriptions start at the same position

## REMOVED Requirements

### Removed: Neutral `[    ]` Marker

The `[    ]` marker for informational items is removed. All items MUST use `[PASS]`, `[WARN]`, or `[FAIL]`.
