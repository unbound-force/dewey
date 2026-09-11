---
name: pre-flight
description: "Shared pre-flight skill for CI detection and local tool execution. Supports hard-gate, ci-aware, and soft-gate execution policies."
---
<!-- scaffolded by uf v0.17.0 -->
# Skill: Pre-flight Checks

Shared logic for CI workflow detection, local tool
detection, CI coverage matrix generation, and local tool
execution. Consuming commands load this skill and select
an execution policy.

## Execution Policies

| Mode | Behavior | Typical consumer |
|------|----------|-----------------|
| `hard-gate` | Run all detected tools. Stop on first failure. | `/unleash` (phase checkpoints) |
| `ci-aware` | Build CI coverage matrix against PR check results. Skip tools CI already verified. Run the rest. | `/review-pr` |
| `soft-gate` | Run all detected tools. Classify failures as branch-caused vs pre-existing. Gate only on branch-caused failures. | `/review-council` |

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

## Phase 2a: File-Scope Filter

Before building the CI coverage matrix, filter out tools
that have no applicable files in the branch diff. This
avoids running tools whose scope does not intersect with
the changes on this branch.

### File-scope mapping

| Tool | In-scope file patterns |
|------|----------------------|
| `go test` | `*.go`, `go.mod`, `go.sum` |
| `golangci-lint` | `*.go`, `go.mod`, `go.sum`, `.golangci.yml`, `.golangci.yaml` |
| `ruff` | `*.py`, `ruff.toml`, `pyproject.toml` |
| `pytest` | `*.py`, `pyproject.toml`, `setup.py`, `conftest.py` |
| `yamllint` | `*.yml`, `*.yaml`, `.yamllint.yml`, `.yamllint.yaml` |
| `make check` | _always in scope_ |
| `pre-commit` | _always in scope_ |

Tools marked "always in scope" MUST NOT be skipped by the
file-scope filter. These are aggregate tools whose scope
cannot be reliably determined from file extensions alone.

Additional tools added to Phase 2's tool-to-command mapping
in the future MUST have a corresponding file-scope entry
added to this table.

Note: `go vet` and `go build` are CI commands discovered
in Phase 1 (CI Workflow Parsing), not independently detected
tools in Phase 2 (Tool Detection). They do not need scope
entries because they are not individually executed by the
pre-flight skill. They are covered by the `make check`
aggregate tool entry (always in scope) and, in the case of
`go vet`, by `golangci-lint`'s inclusion of vet rules.

File patterns use suffix matching against the full path
returned by `git diff --name-only`. A pattern `*.go`
matches any file whose path ends in `.go`, regardless of
directory depth (e.g., `internal/scaffold/foo.go` matches
`*.go`).

### Branch diff computation

Compute the list of files changed on this branch relative
to the default branch:

```bash
DEFAULT_BRANCH=<detected per Phase 4a: Baseline Establishment, "Detect the default branch" subsection>
MERGE_BASE=$(git merge-base HEAD origin/${DEFAULT_BRANCH})
CHANGED_FILES=$(git diff --name-only ${MERGE_BASE}...HEAD)
```

Use the same default branch detection logic described in
Phase 4a ("Detect the default branch" subsection). If the
default branch cannot be
detected, or if the diff command fails, skip the file-scope
filter entirely and proceed to Phase 3 with all tools
(conservative fallback).

If the diff is empty (no changed files) and the current
branch is NOT the default branch, skip the file-scope
filter and run all tools (conservative fallback), since
an empty diff on a feature branch may indicate a git state
anomaly. Report: "Empty diff detected — running all tools
as conservative fallback."

If the repository is a shallow clone (detected via
`git rev-parse --is-shallow-repository`), skip the
file-scope filter and run all tools, since the merge-base
computation may produce incomplete results in shallow
clones.

### Filter logic

For each detected and available tool from Phase 2:

1. Look up the tool's in-scope file patterns in the
   mapping table above.
2. If the tool is marked "always in scope," it proceeds
   to Phase 3 unconditionally.
3. Intersect the branch diff file list with the tool's
   file-scope patterns using suffix matching.
4. If zero diff files match the tool's patterns, mark the
   tool as "SKIP — no in-scope files."
5. If one or more diff files match, the tool proceeds to
   Phase 3 as normal.

### Output

Report the file-scope filter results:

```
File-scope filter (Phase 2a):
  Branch diff: 2 files (release.yml, .goreleaser.yaml)
  - go test: SKIP — no in-scope files
  - golangci-lint: SKIP — no in-scope files
  - yamllint: in scope (1 file matches)
  - make check: always in scope
```

---

## Phase 3: CI Coverage Matrix

Build and display a coverage matrix that maps each
detected local tool to the CI check that covers the same
verification. This matrix makes the skip/run decision
visible and auditable.

### Matrix construction

For each detected and available tool that has in-scope
files in the branch diff (i.e., tools that survived
Phase 2a's file-scope filter), determine which CI check
(if any) covers the same verification. Tools marked
"SKIP — no in-scope files" in Phase 2a are excluded from
the matrix and are not evaluated for CI coverage. Map
tool names to CI check names by matching on the tool's
purpose (e.g., `go test` maps to a CI check containing
"test", `golangci-lint` maps to a check containing
"lint").

### Decision rules (ci-aware mode)

| CI status | Run locally? | Rationale |
|-----------|-------------|-----------|
| PASS | No | CI already verified |
| FAIL | No | Failure already captured from CI; will be included in AI review context |
| NONE (no matching check) | Yes | No CI coverage for this tool |
| No CI checks at all | Yes (all tools) | Cannot determine CI coverage |

### Decision rules (hard-gate mode)

In hard-gate mode, all detected and available tools that
have in-scope files in the branch diff (per Phase 2a) are
marked "Run locally = Yes" regardless of CI status. Tools
marked "SKIP — no in-scope files" in Phase 2a are excluded
from the coverage matrix's "Run locally" decisions and are
not executed. The CI status column in the matrix shows the
actual status if available, or "N/A" if CI results were
not provided. The coverage matrix is still displayed for
visibility.

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

### soft-gate mode

Execute all detected and available tools that have
in-scope files in the branch diff (same scope filtering
as hard-gate, per Phase 2a). Do NOT stop on first
failure — record all exit codes and output for every
tool.

- If ALL tools pass: verdict is PASS. No baseline
  establishment is needed. Skip Phase 4a and 4b.
- If ANY tools fail: proceed to Phase 4a (Baseline
  Establishment) to classify each failure.

---

## Phase 4a: Baseline Establishment (soft-gate only)

This phase runs only in `soft-gate` mode, and only when
at least one tool failed during Phase 4 execution.

Establish a baseline for the default branch to determine
which failures are branch-caused vs pre-existing. Use a
two-tier strategy: CI API first, local worktree fallback.

### Detect the default branch

Before establishing a baseline, detect the repository's
default branch. Do NOT hardcode `main` — repositories
may use `master` or another default branch name.

```bash
DEFAULT_BRANCH=$(git symbolic-ref \
  refs/remotes/origin/HEAD 2>/dev/null \
  | sed 's|refs/remotes/origin/||')
```

If that fails (remote HEAD not set, which happens after
a fresh clone without
`git remote set-head origin --auto`), fall back to
checking for common names:

```bash
if [ -z "${DEFAULT_BRANCH}" ]; then
  if git rev-parse --verify origin/main \
    >/dev/null 2>&1; then
    DEFAULT_BRANCH="main"
  elif git rev-parse --verify origin/master \
    >/dev/null 2>&1; then
    DEFAULT_BRANCH="master"
  fi
fi
```

If neither resolves, treat the baseline as unavailable
and fall through to the conservative fallback (all
failures classified as `unknown` = branch-caused).

Record which baseline method was used: `CI API`,
`worktree`, or `unavailable`.

### Tier 1 — CI API baseline

Check if the `gh` CLI is available:

```bash
which gh
```

If `gh` is available, query the latest check-run results
for the default branch:

```bash
gh api \
  repos/{owner}/{repo}/commits/${DEFAULT_BRANCH}/check-runs \
  --jq '.check_runs[] | {name, conclusion}'
```

Use `--arg` for any dynamic values to prevent injection
(consistent with `/review-pr` Step 3a).

Map CI check names to local tool names using the same
coverage matrix logic from Phase 3. For each failing
tool from Phase 4, look up the corresponding CI check
conclusion on `${DEFAULT_BRANCH}`:

- `conclusion: "success"` → baseline PASS
- `conclusion: "failure"` → baseline FAIL
- No matching check → baseline NO DATA

If `gh` is not available, or the API call returns no
data, or the API call fails: proceed to Tier 2.

### Tier 2 — Local worktree baseline

Create a temporary detached worktree of the default
branch:

```bash
SHORT_SHA=$(git rev-parse --short=8 ${DEFAULT_BRANCH})
git worktree add /tmp/preflight-baseline-${SHORT_SHA} \
  ${DEFAULT_BRANCH} --detach
```

Run ONLY the tools that failed on the branch in the
worktree directory. Tools that passed on the branch
MUST NOT be run against the baseline — they are not
branch-caused by definition.

```bash
# For each failing tool, run it in the worktree:
cd /tmp/preflight-baseline-${SHORT_SHA}
<tool-command>
# Record exit code
```

After running all failing tools, clean up the worktree:

```bash
git worktree remove \
  /tmp/preflight-baseline-${SHORT_SHA} --force
```

Compare exit codes:
- Tool fails in worktree → baseline FAIL
- Tool passes in worktree → baseline PASS

### Fallback — conservative classification

If both Tier 1 and Tier 2 fail (e.g., `gh` unavailable
AND worktree creation fails due to disk space or dirty
state), or the default branch could not be detected,
classify ALL failures as `unknown`. The `unknown`
classification is treated as branch-caused
(conservative), matching `/review-pr` behavior.

Record which baseline method was used: `CI API`,
`worktree`, or `unavailable`.

---

## Phase 4b: Causality Classification (soft-gate only)

This phase runs only in `soft-gate` mode, after Phase 4a
has established a baseline.

For each failing tool from Phase 4, classify it using
the baseline result from Phase 4a:

| Baseline status | Branch status | Classification |
|-----------------|---------------|----------------|
| Pass            | Fail          | **branch-caused** |
| Fail            | Fail          | **pre-existing** |
| No data         | Fail          | **unknown** (treat as branch-caused) |

### Gate decision

After classifying all failures:

- If ANY failures are `branch-caused` or `unknown`:
  verdict is **FAIL (branch-caused)**. The consuming
  command MUST NOT proceed past the pre-flight gate.
- If ALL failures are `pre-existing`: verdict is
  **PASS**. The consuming command MAY proceed, with
  pre-existing failures reported as informational
  findings.

---

## Phase 5: Result Format

Present results in a standardized format.

### hard-gate and ci-aware modes

```
## Pre-flight Results

### CI Coverage Matrix
| Local tool | CI check | CI status | Run locally? |
|------------|----------|-----------|--------------|
| ...        | ...      | ...       | Yes          |
| ...        | ...      | ...       | SKIP — no in-scope files |

### Execution Results
| Tool | Command | Exit code | Status |
|------|---------|-----------|--------|
| ...  | ...     | ...       | ...    |
| ...  | ...     | —         | SKIP — no in-scope files |

### Verdict
- **Mode**: hard-gate | ci-aware
- **Result**: PASS | FAIL
- **Skipped — no in-scope files**: N tools
- **Failures**: [list if any]
```

### soft-gate mode

```
## Pre-flight Results

### CI Coverage Matrix
| Local tool | CI check | CI status | Run locally? |
|------------|----------|-----------|--------------|
| ...        | ...      | ...       | Yes          |
| ...        | ...      | ...       | SKIP — no in-scope files |

### Execution Results
| Tool | Command | Exit code | Status | Causality |
|------|---------|-----------|--------|-----------|
| ...  | ...     | ...       | ...    | ...       |
| ...  | ...     | —         | SKIP — no in-scope files | — |

### Verdict
- **Mode**: soft-gate
- **Result**: PASS | FAIL (branch-caused)
- **Skipped — no in-scope files**: N tools
- **Branch-caused failures**: [list if any]
- **Pre-existing failures**: [list if any]
- **Baseline method**: CI API | worktree | unavailable
```

Tools skipped by the file-scope filter (Phase 2a) use the
canonical status string `SKIP — no in-scope files` (em
dash) in both the coverage matrix "Run locally?" column
and the execution results Status column. Skipped tools are
counted as PASS for the overall verdict — a tool with zero
applicable files in the diff cannot produce findings.

The `Causality` column in the Execution Results table
contains one of: `branch-caused`, `pre-existing`,
`unknown`, `—` (for tools that passed), or `—` (for
skipped tools).

The `Result` field is:
- `PASS` if no branch-caused or unknown failures exist
  (even if pre-existing failures exist)
- `FAIL (branch-caused)` if any branch-caused or unknown
  failures exist

The consuming command uses this result to decide whether
to proceed:
- `hard-gate`: stop on FAIL
- `ci-aware`: continue with failure context for AI review
- `soft-gate`: stop on FAIL (branch-caused), continue
  with pre-existing failures as informational findings
