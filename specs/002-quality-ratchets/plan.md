# Implementation Plan: Quality Ratchets

**Branch**: `002-quality-ratchets` | **Date**: 2026-03-23 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/002-quality-ratchets/spec.md`

## Summary

Add Gaze quality enforcement to CI with ratcheted thresholds, then improve CRAPload from D grade (88 functions, 24.8%) to B grade (≤53 functions, ≤15%), and contract coverage from C grade (70.1%) to B grade (≥80%). The ratchet mechanism uses Gaze CLI flags (`--max-crapload`, `--max-gaze-crapload`, `--min-contract-coverage`) which exit non-zero when thresholds are exceeded. Thresholds are version-controlled as literal values in the CI workflow file.

## Technical Context

**Language/Version**: Go 1.25 (per `go.mod`)
**Quality Tool**: Gaze v1.4.6 (`go install github.com/unbound-force/gaze/cmd/gaze@v1.4.6`)
**CI Platform**: GitHub Actions (`.github/workflows/ci.yml`)
**Testing**: `go test -race -count=1 -coverprofile=coverage.out ./...`
**Project Type**: MCP server + CLI tool (quality improvement, no new features)
**Constraints**: No new production code dependencies. Test-only changes for US2/US3. CI workflow changes for US1.
**Baseline Metrics**: CRAPload 88 (24.8%, grade D), contract coverage 70.1% (grade C), GazeCRAPload 18 (grade B)
**Target Metrics**: CRAPload ≤53 (≤15%, grade B), contract coverage ≥80% (grade B), GazeCRAPload ≤10 (grade A)

## Constitution Check

### I. Composability First -- PASS

No changes to Dewey's runtime behavior. Quality tooling is a development-time concern. Dewey remains independently installable and usable without Gaze.

### II. Autonomous Collaboration -- PASS

No changes to MCP tools or their contracts. All 40 tools continue to function identically.

### III. Observable Quality -- PASS

This feature directly serves Observable Quality by making quality metrics machine-enforceable in CI. The ratchet mechanism ensures quality is auditable and monotonically non-decreasing.

### IV. Testability -- PASS

This feature improves testability by adding tests to untested functions and strengthening contract assertions on existing tests. All new tests are isolated and run without external services.

**Pre-design gate result: ALL PASS.**

## Project Structure

### CI Integration

```text
.github/workflows/ci.yml        # Extended with Gaze threshold checks
```

### Test Files (new or modified)

```text
# Decomposition targets (production code changes)
main.go                          # executeServe decomposed
source/manager.go                # createSource decomposed

# New test files for untested packages
tools/mock_backend_test.go       # Shared mock backend for all tool tests
tools/decision_test.go           # Decision tool tests
tools/journal_test.go            # Journal tool tests
tools/search_test.go             # Search tool tests
tools/analyze_test.go            # Analyze tool tests
tools/write_test.go              # Write tool tests
tools/helpers_test.go            # Helper function tests
client/logseq_test.go            # Logseq client tests (mock HTTP)

# Existing test files (assertion strengthening)
tools/semantic_test.go           # Strengthen assertions for Q3/Q4
vault/vault_store_test.go        # Strengthen assertions
source/disk_test.go              # Strengthen assertions
```

## Ratchet Strategy

Gaze enforces thresholds via CLI flags on `gaze report`. The values are hardcoded in the CI workflow file, making them version-controlled and auditable. The reference pattern comes from the Gaze repo's own CI (`.github/workflows/test.yml`):

**Two-step CI approach:**
1. **Threshold gate** (always runs on PRs, no AI key needed): `gaze report ./... --format=json --coverprofile=coverage.out --max-crapload=N --max-gaze-crapload=N --min-contract-coverage=N > /dev/null`
2. **Full AI report** (push to main only, optional): `gaze report ./... --ai=opencode --coverprofile=coverage.out --max-crapload=N ...`

**Initial thresholds** (set to current baseline values first, then tightened):
- `--max-crapload=88` → tightened to ≤53 after US2
- `--max-gaze-crapload=18` → tightened to ≤10 after US3
- `--min-contract-coverage=70` → tightened to ≥80 after US3

**Coverage reuse**: `go test -coverprofile=coverage.out` runs once, then `--coverprofile=coverage.out` passes the profile to Gaze, avoiding double-testing.

## CRAPload Reduction Strategy

The 88 functions above threshold break down by remediation strategy:

| Strategy | Count | Approach |
|----------|-------|----------|
| `add_tests` | 71 | Write tests for untested functions (no production changes) |
| `decompose` | 13 | Refactor for lower complexity, then test |
| `decompose_and_test` | 4 | Highest priority: refactor AND test |

To reach ≤53 (a reduction of 35+), the most efficient approach:

1. **Decompose the 4 worst offenders** (`executeServe` CRAP 650, `createSource` 506, `MoveBlock` 462, `ListPages` 306). Each produces ~3-4 lower-complexity functions that individually drop below threshold 15.

2. **Add tests for 31+ `add_tests` functions**, prioritized by CRAP score. A function with complexity 6 and 0% coverage has CRAP=42; adding even basic tests (50% coverage) drops it to ~7.5.

3. **Strengthen assertions for Q3/Q4 functions** -- moves them from dangerous/underspecified to safe, improving GazeCRAPload and contract coverage simultaneously.

### Mock Backend for Tool Tests

The `tools/` package functions accept `context.Context`, `*mcp.CallToolRequest`, and a tool struct wrapping `backend.Backend`. A shared mock is reusable across all tool test files.

## Verification Strategy

1. **Before**: `gaze report ./... --format=json --coverprofile=coverage.out` to establish baseline
2. **After each phase**: Re-run Gaze, verify metrics improve monotonically
3. **Final**: CI passes with tightened thresholds
4. **Regression**: `go test -race -count=1 ./...` must still pass after all changes

## Complexity Tracking

No complexity beyond constitutional principles. This feature adds tests and CI configuration -- no architectural decisions or trade-offs.
