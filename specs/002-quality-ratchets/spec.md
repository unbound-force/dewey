# Feature Specification: Quality Ratchets

**Feature Branch**: `002-quality-ratchets`
**Created**: 2026-03-23
**Status**: In Progress (US1 complete, US2 complete, US3 partially complete — contract coverage 61.6% vs 80% target, blocked by Gaze classifier issues #77-#79)
**Input**: User description: "please create a plan to bring the CRAPload grade up to a B, the Contract Coverage up to a B, add Gaze to the CI checks for main, and add ratchets for the Gaze thresholds"

## User Scenarios & Testing *(mandatory)*

### User Story 1 -- CI Enforces Quality Thresholds (Priority: P1)

A developer pushes code to the dewey repository. Today, CI only runs `go vet`, `go build`, and `go test -race`. There is no automated check that prevents quality regression. A developer could merge code with high complexity and zero test coverage without any pipeline feedback.

With quality ratchets in CI, the pipeline runs Gaze after tests pass and compares the results against committed threshold files. If any metric regresses beyond the threshold (e.g., CRAPload increases or contract coverage decreases), the CI build fails with a clear message identifying which metric regressed and by how much. The thresholds are committed to the repository as version-controlled files, so the team can see the quality baseline and intentionally tighten it over time.

**Why this priority**: Without CI enforcement, the other user stories (reducing CRAPload, improving contract coverage) have no guard against regression. CI is the ratchet mechanism -- everything else is a one-time improvement that would erode without it.

**Independent Test**: Configure Gaze in CI, commit threshold files, push a commit that introduces a function with CRAP score above threshold, and verify the pipeline fails with an actionable error message.

**Acceptance Scenarios**:

1. **Given** a pull request with no quality regressions, **When** CI runs Gaze checks, **Then** the pipeline passes and reports current quality metrics in the build log.
2. **Given** a pull request that increases CRAPload (adds a high-complexity untested function), **When** CI runs Gaze checks, **Then** the pipeline fails with a message identifying the new function and its CRAP score.
3. **Given** a pull request that decreases contract coverage below the threshold, **When** CI runs Gaze checks, **Then** the pipeline fails with a message identifying the coverage drop.
4. **Given** a developer intentionally improves quality beyond the current threshold, **When** they update the threshold file to reflect the new baseline, **Then** subsequent PRs are held to the tighter standard.
5. **Given** a new developer unfamiliar with the project, **When** their CI build fails due to a quality regression, **Then** the error message includes instructions on how to fix the issue (e.g., "add tests for function X" or "reduce complexity of function Y").

---

### User Story 2 -- Reduce CRAPload to B Grade (Priority: P2)

A project maintainer reviews the Gaze quality report and sees a CRAPload of 48 functions (13.5% of all functions above threshold 15), graded "C." While the CRAPload count is below the ≤53 target, 4 functions have extreme CRAP scores (306-650) due to high complexity combined with zero test coverage. The maintainer wants to decompose these high-risk functions and add tests to bring the GazeCRAPload (contract-aware metric) from 37 down to ≤10.

The maintainer works through the highest-CRAP functions, applying two strategies: (1) adding tests for functions that are complex but testable as-is (the `add_tests` category: 31 functions), and (2) decomposing functions that are too complex to test effectively and then testing the decomposed parts (the `decompose_and_test` category: 4 functions, plus 13 `decompose` functions). The work is prioritized by CRAP score -- the worst offenders are addressed first for maximum impact.

**Why this priority**: CRAPload reduction delivers the largest quality improvement per effort. The 31 `add_tests` functions need only new test files, not production code changes. The 4 `decompose_and_test` functions (`executeServe`, `createSource`, `MoveBlock`, `ListPages`) need refactoring but are also the highest-risk code. Doing this after CI enforcement (US1) means every improvement is locked in by the ratchet.

**Independent Test**: Run Gaze before and after the changes. Verify GazeCRAPload drops from 37 toward ≤10. Verify no existing tests break.

**Acceptance Scenarios**:

1. **Given** the current CRAPload of 48 functions above threshold (already below ≤53 target), **When** the 4 highest-CRAP functions are decomposed and tested, **Then** CRAPload drops further and GazeCRAPload improves toward ≤10.
2. **Given** a function with complexity >15 and 0% coverage, **When** tests are added, **Then** the function's CRAP score drops below threshold 15.
3. **Given** a function with complexity >20 (e.g., `executeServe` at 25), **When** it is decomposed into smaller functions, **Then** each resulting function has complexity ≤10 and is individually tested.
4. **Given** the decomposition of a production function, **When** the refactoring is complete, **Then** all existing tests continue to pass (no behavioral regression).

---

### User Story 3 -- Improve Contract Coverage to B Grade (Priority: P3)

A project maintainer reviews the Gaze quality report and sees contract coverage at 56.5% (grade "D"). This means 43.5% of tested functions have tests that exercise the code but do not verify observable behavior -- tests that pass regardless of whether the function produces correct output. The maintainer wants to reach 80% contract coverage (grade "B").

The maintainer strengthens existing test assertions across the 32 Q3 (Needs Tests) functions and the 5 Q4 (Dangerous) functions, and improves GoDoc comments to help Gaze's effect classifier distinguish contractual effects from ambiguous ones. For Q3 functions, the fix is adding assertions that verify return values, side effects, or error conditions. For Q4 functions, the fix involves both decomposition (to reduce complexity) and assertion strengthening.

**Why this priority**: Contract coverage improvements build on the CRAPload work (US2). Many of the same functions that need tests (US2) also need stronger assertions (US3). Doing this last means the assertions are written against well-decomposed, testable functions rather than monolithic ones.

**Independent Test**: Run Gaze before and after. Verify average contract coverage across analyzed functions reaches ≥80%. Verify GazeCRAPload (the contract-aware metric) improves.

**Acceptance Scenarios**:

1. **Given** current contract coverage of 56.5%, **When** test assertions are strengthened, **Then** contract coverage reaches 80% or higher.
2. **Given** a Q3 function (simple but underspecified), **When** contract assertions are added, **Then** the function moves from Q3 to Q1 (Safe).
3. **Given** a Q4 function (dangerous: complex and underspecified), **When** decomposition and assertion strengthening are applied, **Then** the function moves from Q4 to Q1 or Q2.
4. **Given** the GazeCRAPload of 37 (5 Q4 + 32 Q3), **When** Q3 and Q4 functions are addressed, **Then** GazeCRAPload drops to 10 or fewer.

---

### Edge Cases

- What happens when a new function is added that pushes CRAPload above the ratcheted threshold? CI MUST fail and the developer must either add tests or reduce complexity before merging.
- What happens when a legitimate refactoring temporarily increases complexity (e.g., extracting a function creates two functions where one existed)? The threshold file can be temporarily updated in the same PR, with a justification in the commit message. The ratchet is a floor, not a straitjacket.
- What happens when Gaze is unavailable in CI (e.g., binary not installed, network issue)? CI MUST fail clearly rather than silently skipping the quality check. A missing quality gate is worse than a failing one.
- What happens when threshold files are out of sync with the codebase (e.g., someone manually edits the threshold to a value worse than current)? The CI step SHOULD validate that thresholds are not looser than the actual current values and warn if they are.

## Requirements *(mandatory)*

### Functional Requirements

**CI Integration (US1)**:
- **FR-001**: The CI pipeline MUST run Gaze quality checks after `go test` passes on every pull request targeting `main`.
- **FR-002**: Gaze quality check results MUST be compared against committed threshold files in the repository.
- **FR-003**: The CI pipeline MUST fail with an actionable error message when any Gaze metric regresses beyond the committed threshold.
- **FR-004**: The error message MUST identify the specific metric that regressed, the current value, the threshold value, and the function(s) responsible.
- **FR-005**: Threshold files MUST be version-controlled in the repository so the quality baseline is visible and auditable.
- **FR-006**: The CI pipeline MUST fail if Gaze is not available or the quality check cannot run, rather than silently passing.

**CRAPload Reduction (US2)**:
- **FR-007**: Tests MUST be added for functions in the `add_tests` remediation category, prioritized by CRAP score (highest first).
- **FR-008**: Functions with complexity >20 and 0% coverage MUST be decomposed into smaller functions with complexity ≤15 each, then tested.
- **FR-009**: All function decompositions MUST preserve existing behavior -- no functional regressions allowed.
- **FR-010**: The CRAPload threshold in the committed threshold file MUST be set to the achieved value after improvements (ratchet tightened).

**Contract Coverage (US3)**:
- **FR-011**: Test assertions MUST be strengthened for functions in Q3 (Needs Tests) and Q4 (Dangerous) quadrants, verifying observable behavior rather than just exercising code paths.
- **FR-012**: Each strengthened test MUST verify at least one of: return value correctness, side effect presence, error condition accuracy, or state mutation.
- **FR-013**: The contract coverage threshold in the committed threshold file MUST be set to the achieved value after improvements.
- **FR-014**: GoDoc comments SHOULD be improved on exported functions to help Gaze's effect classifier distinguish contractual effects from ambiguous ones. This includes documenting return values, error conditions, and observable side effects explicitly.

### Assumptions

- Gaze v1.4.6 or later is available for installation in the CI environment. The Gaze binary can be installed via `go install` from the gaze repository.
- The Gaze `crap` and `quality` subcommands produce machine-readable output that can be parsed by a CI script for threshold comparison.
- The current CRAPload baseline is 48 functions (13.5%, measured 2026-03-24, improved from the original 88). The target "B" grade requires ≤15% (≤53 functions). CRAPload target is already met; the remaining work is GazeCRAPload (37 → ≤10) and contract coverage (56.5% → ≥80%).
- The current contract coverage baseline is 56.5% (measured 2026-03-24). The target "B" grade requires ≥80%. The gap is 23.5 percentage points.
- Some inherited graphthulhu functions (e.g., `MoveBlock`, `ListPages`) may require significant refactoring. The effort is justified because these are the highest-risk functions in the codebase.
- Functions in the `tools/` package require mock `backend.Backend` implementations for testing. A shared `mockBackend` (355 lines, 28 mock funcs) already exists in `tools/mock_backend_test.go` and is reusable across all tool tests. Most tool test files already exist with substantial coverage; the remaining work is adding tests for uncovered functions (DecisionResolve, DecisionDefer, MoveBlock, ListPages) and strengthening assertions on existing tests.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: CRAPload is ≤53 functions above threshold 15 (≤15% of total functions, grade B or better).
- **SC-002**: Module-wide average contract coverage across all analyzed functions is ≥80% (grade B or better). Individual packages may be below 80% as long as the overall average meets the target.
- **SC-003**: GazeCRAPload is ≤10 functions (reduced from 37: 5 Q4 + 32 Q3).
- **SC-004**: CI pipeline runs Gaze checks on every PR to `main` and fails on quality regression.
- **SC-005**: Threshold files are committed to the repository and set to the achieved quality values (ratchet locked).
- **SC-006**: No existing tests are broken by function decompositions (backward compatibility maintained).
- **SC-007**: All Q4 (Dangerous) functions are moved to Q1 (Safe) or Q2 (Complex But Tested).

## Clarifications

### Session 2026-03-24

- Q: Contract coverage baseline is 56.5% (not 70.1% as originally assumed). Update approach? → A: Update baseline to 56.5%, keep ≥80% target. Accept the larger effort.
- Q: Should documentation improvements (GoDoc) be in scope for improving contract coverage classifier accuracy? → A: Yes, both documentation improvements and assertion strengthening are in scope. Most efficient path to 80%.
- Q: Is the ≥80% contract coverage target module-wide average or per-package? → A: Module-wide average. Individual packages may be below 80% as long as the overall average meets the target.
- Q: GazeCRAPload baseline is 37 (not 18 as assumed). Keep ≤10 target? → A: Yes, keep ≤10 target. Q3 functions (32) primarily need assertion strengthening, making this achievable.
- Q: CRAPload already at 48 (below ≤53 target). Still decompose the 4 highest-CRAP functions? → A: Yes, still required. CRAP 306-650 functions are highest-risk code, and decomposition makes US3 assertion work more effective.
