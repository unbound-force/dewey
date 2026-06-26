---
description: "Shared pre-flight skill for CI detection and local tool execution. Supports hard-gate and ci-aware execution policies."
---
<!-- scaffolded by uf vdev -->
# Skill: Pre-flight Checks

Shared logic for CI workflow detection, local tool
detection, CI coverage matrix generation, and local tool
execution. Consuming commands load this skill and select
an execution policy.

## Execution Policies

| Mode | Behavior | Typical consumer |
|------|----------|-----------------|
| `hard-gate` | Run all detected tools. Stop on first failure. | `/review-council`, `/unleash` |
| `ci-aware` | Build CI coverage matrix against PR check results. Skip tools CI already verified. Run the rest. | `/review-pr` |

The consuming command specifies which mode to use.

---

## Phase 1: CI Workflow Parsing

Read all files in `.github/workflows/` to identify the
exact commands CI runs. Do NOT hardcode language-specific
commands — the workflow files are the source of truth.

### Extraction rules

- Extract only `run:` step commands from workflow YAML.
- **Ignore** `uses:` steps — these are GitHub-hosted
  actions and are not locally executable.
- **Skip** commands containing unresolvable CI expressions
  (`${{ secrets.* }}`, `${{ github.* }}`) with a warning
  noting unresolvable CI expressions. These commands
  depend on CI runtime context and cannot run locally.
- Multi-line `run:` blocks: extract each command line
  individually. Skip lines that are pure shell control
  flow (if/then/fi, variable assignments used only
  within the block).

### Output

A list of CI commands discovered from workflows, e.g.:

```
CI commands discovered from .github/workflows/:
  - go build ./...        (ci_local.yml)
  - go test -race -count=1 -coverprofile=coverage.out ./...  (ci_local.yml)
```

If no `.github/workflows/` directory exists, report
"No CI workflows found" and proceed to Phase 2.

---

## Phase 2: Local Tool Detection

Check which tools are available by looking for their
configuration files:

```bash
test -f Makefile && echo "MAKEFILE=yes"
test -f .golangci.yml && echo "GO_LINT=yes"
test -f ruff.toml -o -f pyproject.toml && echo "PYTHON_LINT=yes"
test -f .yamllint.yml && echo "YAML_LINT=yes"
test -f .pre-commit-config.yaml && echo "PRECOMMIT=yes"
test -f go.mod && echo "GO_TEST=yes"
test -f setup.py && echo "PYTHON_TEST=yes"
```

When `pyproject.toml` is present, detect both ruff and
pytest as separate tools.

### Tool-to-command mapping

| Config file | Tool | Command | What it checks |
|-------------|------|---------|----------------|
| `Makefile` | Make | `make check` (preferred), else `make lint` | Project-defined lint/format/vet |
| `.golangci.yml` | golangci-lint | `golangci-lint run ./...` | Go lint rules |
| `ruff.toml` or `pyproject.toml` | ruff | `ruff check .` | Python lint rules |
| `.yamllint.yml` | yamllint | `yamllint .` | YAML lint rules |
| `.pre-commit-config.yaml` | pre-commit | `pre-commit run --all-files` | Pre-commit hooks |
| `go.mod` | go test | `go test ./...` | Go tests |
| `pyproject.toml` or `setup.py` | pytest | `pytest` or `python -m pytest` | Python tests |

### Binary availability check

For each detected tool, verify the binary is available:

```bash
which <binary-name>
```

If a config file is present but the tool binary is NOT
in PATH, report the tool as "detected but not available"
and skip it with a warning. Do NOT treat a missing binary
as a hard failure.

### Output

A list of detected tools with availability status, e.g.:

```
Local tools detected:
  - Make (Makefile) ✓ available
  - golangci-lint (.golangci.yml) ✓ available
  - yamllint (.yamllint.yml) ✗ not available (skipped)
```

If no tools are detected, report "No local tools detected"
and proceed to Phase 3.

---

## Phase 3: CI Coverage Matrix

Build and display a coverage matrix that maps each
detected local tool to the CI check that covers the same
verification. This matrix makes the skip/run decision
visible and auditable.

### Matrix construction

For each detected and available tool, determine which CI
check (if any) covers the same verification. Map tool
names to CI check names by matching on the tool's purpose
(e.g., `go test` maps to a CI check containing "test",
`golangci-lint` maps to a check containing "lint").

### Decision rules (ci-aware mode)

| CI status | Run locally? | Rationale |
|-----------|-------------|-----------|
| PASS | No | CI already verified |
| FAIL | No | Failure already captured from CI; will be included in AI review context |
| NONE (no matching check) | Yes | No CI coverage for this tool |
| No CI checks at all | Yes (all tools) | Cannot determine CI coverage |

### Decision rules (hard-gate mode)

In hard-gate mode, ALL detected and available tools are
marked "Run locally = Yes" regardless of CI status. The
CI status column in the matrix shows the actual status if
available, or "N/A" if CI results were not provided. The
coverage matrix is still displayed for visibility, but
skip decisions are not applied.

### Display format

```
### CI Coverage Matrix
| Local tool | CI check | CI status | Run locally? |
|------------|----------|-----------|--------------|
| go test | Local CI / test | PASS | No |
| golangci-lint | CI Checks / lint | PASS | No |
| yamllint | (none) | NONE | Yes |
```

---

## Phase 4: Execution

Run only the tools marked "Run locally = Yes" in the
coverage matrix.

### hard-gate mode

Execute each tool in order. If any tool exits with a
non-zero exit code:

1. **STOP immediately** — do not run remaining tools.
2. Report the failure as a CRITICAL finding with the
   full error output.
3. The consuming command MUST NOT proceed to AI review
   or implementation.

If all tools pass, report success.

### ci-aware mode

Execute each tool marked "Yes" in the coverage matrix.
Record all exit codes and output.

- If tools pass: skip those categories in AI review.
- If tools fail: include the failure output as context
  for AI review. Do NOT stop — the consuming command
  decides how to handle failures.

If no tools are marked "Yes" (all covered by CI): report
"All tools covered by CI — no local execution needed."

---

## Phase 5: Result Format

Present results in a standardized format:

```
## Pre-flight Results

### CI Coverage Matrix
| Local tool | CI check | CI status | Run locally? |
|------------|----------|-----------|--------------|
| ...        | ...      | ...       | ...          |

### Execution Results
| Tool | Command | Exit code | Status |
|------|---------|-----------|--------|
| ...  | ...     | ...       | ...    |

### Verdict
- **Mode**: hard-gate | ci-aware
- **Result**: PASS | FAIL
- **Failures**: [list if any]
```

The consuming command uses this result to decide whether
to proceed (hard-gate: stop on FAIL) or to include
failure context in AI review (ci-aware: continue with
context).
