## Context

Dewey's `go.mod` declares `module github.com/unbound-force/dewey` but the project has been tagged through v3.2.0. Go's module system requires that modules at major version >= 2 include the major version suffix in the module path (e.g., `/v3`). Without it, `go install github.com/unbound-force/dewey@latest` resolves to v1.5.0 — a version that uses the obsolete `.dewey/` workspace path.

The proposal confirms this is a mechanical fix with no external consumers, no CI/GoReleaser changes, and constitution alignment (restores Composability First by fixing the `go install` path).

## Goals / Non-Goals

### Goals

- Fix `go install github.com/unbound-force/dewey/v3@latest` to resolve to the current v3.x release
- Update all internal import paths from `github.com/unbound-force/dewey/<pkg>` to `github.com/unbound-force/dewey/v3/<pkg>`
- Update documentation references (README.md, AGENTS.md) to reflect the new module path
- Maintain full backward compatibility — all tests pass, all MCP tools function identically

### Non-Goals

- Retag or modify existing v1.x/v2.x releases
- Create a `/v3` subdirectory (Go allows the module path suffix without a subdirectory when using modules)
- Support `go install github.com/unbound-force/dewey@latest` (without `/v3`) — this will continue to resolve to v1.5.0, which is correct per Go module semantics
- Update any downstream consumers (none exist)

## Decisions

### D1: Global find-and-replace for import paths

The rewrite is a mechanical substitution of `github.com/unbound-force/dewey/` to `github.com/unbound-force/dewey/v3/` across all `.go` files. No structural or API changes.

**Rationale**: This is the standard Go approach for major version bumps. The replicator project used the same pattern in `specs/003-rename-terminology/plan.md` for a similar import-path rewrite. The scope (64 files, 181 import lines, 15 subpackages) is well-bounded and verifiable with `go build ./...`.

### D2: Module path update in go.mod only — no `/v3` subdirectory

Go's module system supports major version suffixes without a corresponding directory when using modules (i.e., `go.mod` says `module .../v3` but the code stays at the repository root). This is the approach used by most Go projects.

**Rationale**: Creating a `/v3` subdirectory would require moving all source code, breaking the flat package layout, and adding unnecessary complexity. The module-only approach is simpler, well-documented in Go's official documentation, and widely adopted.

### D3: Update documentation in the same change

README.md's install instructions and AGENTS.md's module path references are updated alongside the code. Spec artifacts under `specs/` and `openspec/` are left as-is since they document historical decisions.

**Rationale**: Documentation must match the current install path immediately. Historical spec artifacts are snapshots — updating them would misrepresent what was planned at the time.

### D4: Verification via full CI-equivalent checks

After the rewrite, the implementation must pass `go build ./...`, `go vet ./...`, and `go test -race -count=1 ./...` locally before marking complete. This satisfies the CI Parity Gate.

**Rationale**: The mechanical nature of the change makes a post-rewrite build+test pass sufficient to verify correctness. No new tests are needed — existing tests validate the same behavior with updated import paths.

## Test Strategy

This change is a mechanical import path rewrite with no behavioral changes. The test strategy is to run the full existing test suite (`go test -race -count=1 ./...`) after the rewrite. All 64 rewritten files are exercised by existing tests. No new tests are required because no new behavior is introduced — the same code runs with updated import paths.

## Risks / Trade-offs

### Risk: String literals containing the module path

Some Go files may contain string literals (e.g., log messages, error strings) that reference the module path. The global find-and-replace will catch these, which is the desired behavior — user-facing messages should reflect the current module path.

**Mitigation**: Review the diff after rewriting to confirm no unintended substitutions in comments, test fixtures, or embedded data.

### Risk: Spec artifact references

Historical spec artifacts (under `specs/` and `openspec/`) reference the old module path. These are not updated.

**Mitigation**: Acceptable — spec artifacts are point-in-time documents. They describe what was planned, not the current state. AGENTS.md and README.md are the authoritative references for the current module path.

### Trade-off: Old `go install` path stops working

After this change, `go install github.com/unbound-force/dewey@latest` will still resolve to v1.5.0. Users must use `go install github.com/unbound-force/dewey/v3@latest` for the current version.

**Mitigation**: This is correct Go module behavior, not a regression. README.md will document the correct install command.
