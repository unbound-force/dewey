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

**Decision**: Start with current baseline values (CRAPload=88, GazeCRAPload=18, contract coverage=70%), then tighten after each improvement phase.

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
