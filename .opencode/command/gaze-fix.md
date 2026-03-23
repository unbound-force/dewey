---
description: >
  Generate tests and documentation fixes for functions identified by
  gaze quality analysis. Prioritizes by fix strategy to reduce
  CRAPload and GazeCRAPload below ratchet thresholds.
agent: gaze-test-generator
---
<!-- scaffolded by gaze v1.4.6 -->

# Command: /gaze fix

## Description

Batch remediation command that reads gaze analysis data, identifies
functions needing tests or documentation, and generates complete,
compilable fixes. Delegates to the `gaze-test-generator` agent for
each target function.

## Usage

```
/gaze fix [package-pattern]
/gaze fix --strategy=add_tests [pattern]
/gaze fix --top=5 [pattern]
/gaze fix --dry-run [pattern]
```

### Options

| Option | Description |
|--------|-------------|
| `[pattern]` | Go package pattern (default: `./...`) |
| `--strategy=X` | Filter to one strategy: `add_tests`, `add_assertions`, `add_docs`, `decompose_and_test` |
| `--top=N` | Process only the top N functions by CRAP score |
| `--dry-run` | Show what would be generated without writing files |

## Instructions

### Step 1: Run gaze analysis

Resolve the `gaze` binary (check PATH, then try `go run ./cmd/gaze`
if in the gaze repo). Run both commands:

```bash
gaze crap --format=json [pattern] > /tmp/gaze-fix-crap.json
gaze quality --format=json [pattern] > /tmp/gaze-fix-quality.json
```

Parse the CRAP JSON for `scores` array — each score has `function`,
`package`, `file`, `line`, `fix_strategy`, `crap`, `gaze_crap`,
`quadrant`, `contract_coverage`, `contract_coverage_reason`,
`effect_confidence_range`.

### Step 2: Build target list

Filter scores to actionable fix strategies:
1. `add_tests` — functions with 0% line coverage
2. `add_assertions` — functions with line coverage but Q3 quadrant
3. `decompose_and_test` — complex functions with 0% coverage
4. Skip `decompose` — not fixable with tests

Additionally, check for `add_docs` candidates: functions where
`contract_coverage_reason` is `all_effects_ambiguous` AND
`effect_confidence_range[0]` >= 58 (close enough to push above 70
with GoDoc).

Apply `--strategy` filter if specified.
Sort by priority: `add_tests` first (by CRAP desc), then
`add_assertions`, then `add_docs`, then `decompose_and_test`.
Apply `--top=N` limit if specified.

### Step 3: Process each target

For each function in the target list:

1. **Read the function source**: Use the `file` and `line` from the
   CRAP score to read the function implementation
2. **Read existing tests**: Look for `*_test.go` in the same
   directory. Read it if it exists.
3. **Get quality data**: Find the matching entry in the quality JSON
   (match by function name + package). Extract `Gaps`, `GapHints`,
   `DiscardedReturns`, `DiscardedReturnHints`, `AmbiguousEffects`,
   `UnmappedAssertions`.
4. **Determine action**: Based on fix strategy + quality data:
   - `add_tests` → generate full test function
   - `add_assertions` → add assertions to existing test +
     restructure helper-wrapped assertions
   - `add_docs` → add/improve GoDoc comments
   - `decompose_and_test` → generate test skeleton
   - `decompose` → skip with explanation
5. **Generate code**: Following the quality criteria and convention
   detection rules in the agent prompt
6. **Write code**: Append to the `*_test.go` file (or modify the
   source file for `add_docs`). In `--dry-run` mode, show the
   code but don't write.

### Step 4: Verify

After all generation:

```bash
go build [pattern]
go test -race -count=1 -run "TestGenerated1|TestGenerated2|..." [pattern]
```

Report any compilation errors or test failures with context.

### Step 5: Report

```
## /gaze fix Results

Processed: N functions
- add_tests: X generated
- add_assertions: Y strengthened
- add_docs: Z documented
- decompose_and_test: W skeletons
- decompose: V skipped

Compilation: PASS/FAIL
Tests: K/X pass

Files modified:
- path/to/foo_test.go (2 tests added)
- path/to/bar.go (GoDoc improved)
```

## Error Handling

- If `gaze` binary is not found: error with install instructions
- If analysis produces no actionable targets: report "No functions
  need remediation in [pattern]"
- If a generated test fails to compile: report the error, skip that
  function, continue with others
- If a generated test fails: report the failure, suggest the
  assertion may need adjustment, keep the test (failing tests are
  still valuable as documentation of expected behavior)
