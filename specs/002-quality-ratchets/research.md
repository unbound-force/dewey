# Research: Quality Ratchets

**Branch**: `002-quality-ratchets` | **Date**: 2026-03-23

## Research Summary

No NEEDS CLARIFICATION items existed in the Technical Context. This research documents the key decisions around Gaze CI integration and quality improvement approach.

## Decision 1: Gaze Threshold Enforcement Mechanism

**Decision**: Use `gaze report` CLI flags (`--max-crapload`, `--max-gaze-crapload`, `--min-contract-coverage`) for threshold enforcement. Thresholds are literal values in the CI workflow file.

**Rationale**: Gaze does not have a dedicated threshold file format or `gaze ci` command. The `gaze report` command with threshold flags exits non-zero when any metric regresses beyond the threshold. This is the same pattern used by the Gaze project's own CI (`.github/workflows/test.yml`). Threshold values in the workflow file are version-controlled via git, providing the same auditability as a separate threshold file.

**Alternatives considered**:
- Separate threshold JSON/YAML file: Gaze does not support reading thresholds from a file. Would require a custom wrapper script. Over-engineering for a simple problem.
- Custom CI script parsing `gaze crap --format=json`: Possible but reinvents what `gaze report` already does with threshold flags. More code to maintain.

## Decision 2: Two-Step CI Pattern

**Decision**: Use two CI steps: (1) threshold gate on every PR (JSON output, no AI), and (2) full AI-formatted report on push to main only.

**Rationale**: This is the reference pattern from the Gaze repo's own CI. The threshold gate is fast and requires no API keys. The AI report is slower and requires API credentials, so it only runs on main merges for historical tracking.

**Alternatives considered**:
- Single step with AI on every PR: Rejected because it requires API keys on forks and slows every PR. The threshold gate catches regressions; the AI report is a bonus.
- AI report on PRs too: Nice to have but expensive and slow. Can be added later if desired.

## Decision 3: Coverage Profile Reuse

**Decision**: Run `go test -coverprofile=coverage.out ./...` once, then pass `--coverprofile=coverage.out` to `gaze report`.

**Rationale**: Avoids running the test suite twice. Gaze can use an existing coverage profile instead of running tests internally. This halves CI execution time for the quality check step.

**Alternatives considered**:
- Let Gaze run its own tests: Rejected because it doubles the test execution time in CI.

## Decision 4: Initial Threshold Values

**Decision**: Start with current baseline values, then tighten after each improvement phase. The baselines have evolved:
- Original (spec written): CRAPload=88, GazeCRAPload=18, contract coverage=70%
- Actual (measured 2026-03-24): CRAPload=48, GazeCRAPload=37, contract coverage=56.5%
- Current CI gates: `--max-crapload=48 --max-gaze-crapload=37 --min-contract-coverage=8`

The CRAPload improved from 88 to 48 during Spec 001 core implementation (109+ tests added). However, GazeCRAPload increased (18 → 37) and contract coverage decreased (70% → 56.5%) because the new tests exercise code paths without verifying observable behavior.

**Rationale**: Setting the ratchet at the current baseline means CI immediately prevents regression. As improvements land, the thresholds are tightened in the same PR. This is a monotonically non-decreasing quality guarantee.

**Alternatives considered**:
- Start with target values (53, 10, 80): Rejected because CI would immediately fail on the current codebase, blocking all work until quality targets are met. The ratchet should lock in gains, not block work.

## Decision 5: Gaze Installation in CI

**Decision**: Use `go install github.com/unbound-force/gaze/cmd/gaze@v1.4.6` in CI.

**Rationale**: Simple, reproducible, version-pinned. The Gaze binary is a standalone tool with no CGO dependencies. The `go install` approach leverages the existing Go toolchain already set up in CI.

**Alternatives considered**:
- Build from source (Gaze repo's approach): Only appropriate when Gaze is the project being tested. For consumers, `go install` is simpler.
- Download pre-built binary from GitHub releases: More complex CI config, no benefit since `go install` works and Go is already installed.
- GitHub Action: No published `action.yml` exists for Gaze.

## Decision 6: GoDoc Improvements in Scope (Clarification 2026-03-24)

**Decision**: GoDoc comment improvements on exported functions are in scope for reaching ≥80% contract coverage (FR-014). This is in addition to test assertion strengthening.

**Rationale**: Gaze's effect classifier determines whether a function's effects are "contractual" (verifiable) or "ambiguous" (unclear). When GoDoc comments explicitly document return values, error conditions, and lifecycle requirements, the classifier can more accurately categorize effects as contractual. This means existing test assertions that already verify behavior get properly counted toward contract coverage. Without GoDoc improvements, some functions would require disproportionate test effort to reach 80% because the classifier cannot recognize what the tests are verifying.

**Alternatives considered**:
- Test changes only: Rejected because 66.5% of effects are classified as ambiguous. Even perfect test assertions would not be counted if the classifier cannot determine what the function's contract is. GoDoc improvements provide the signal the classifier needs.
- Blanket documentation pass: Rejected. Only functions where GoDoc improvements materially affect contract coverage classification should be updated. Not a documentation-for-documentation's-sake effort.

## Decision 7: Module-Wide Average for Contract Coverage (Clarification 2026-03-24)

**Decision**: The ≥80% contract coverage target is measured as the module-wide average across all Gaze-analyzed functions, not per-package.

**Rationale**: Per-package minimums would force disproportionate effort on packages with many inherited graphthulhu functions that are difficult to test (e.g., `vault/` with `MoveBlock`). A module-wide average allows strategic prioritization — focusing assertion effort on functions where the ROI is highest.

**Alternatives considered**:
- Per-package minimum of 80%: Rejected because some packages (vault, client) have many inherited functions with low contract coverage that would require extensive mocking infrastructure.
- Module-wide average + per-package floor of 60%: Considered but deferred. The floor can be added as a future ratchet once the module-wide average target is met.
