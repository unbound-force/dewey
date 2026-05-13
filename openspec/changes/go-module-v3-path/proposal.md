## Why

`go install github.com/unbound-force/dewey@latest` resolves to v1.5.0 instead of the current v3.2.0. This is because the `go.mod` module path is `github.com/unbound-force/dewey` (no `/v3` suffix), and Go's module system requires the major version suffix for tags >= v2.0.0. Without it, Go ignores all v2.x and v3.x tags.

This means users who install via `go install` — the recommended fallback for Linux users without Homebrew — get a version that uses the old `.dewey/` workspace path instead of `.uf/dewey/`, making it incompatible with `uf init` and the current scaffold.

Reported in [#60](https://github.com/unbound-force/dewey/issues/60).

## What Changes

Update the Go module path from `github.com/unbound-force/dewey` to `github.com/unbound-force/dewey/v3` and rewrite all internal import paths to match.

## Capabilities

### New Capabilities

- None — this is a bug fix, not a feature addition.

### Modified Capabilities

- `go install`: `go install github.com/unbound-force/dewey/v3@latest` will correctly resolve to the latest v3.x release.

### Removed Capabilities

- None.

## Impact

- **go.mod**: Module declaration changes from `github.com/unbound-force/dewey` to `github.com/unbound-force/dewey/v3`.
- **64 Go source files**: All internal import paths rewritten from `github.com/unbound-force/dewey/<pkg>` to `github.com/unbound-force/dewey/v3/<pkg>` (181 import lines across 15 subpackages).
- **Documentation**: README.md and AGENTS.md updated to reflect the new `go install` command.
- **No external consumers**: Dewey is distributed as a binary. No known downstream Go modules import it as a library.
- **No CI/GoReleaser changes**: GoReleaser uses `main: .` (relative path), not the module path. CI workflows reference commands, not import paths.
- **Existing v1.x/v2.x tags**: Remain valid for their respective module paths. No retroactive changes needed.

## Constitution Alignment

Assessed against the Unbound Force org constitution.

### I. Autonomous Collaboration

**Assessment**: N/A

This change modifies Go import paths only. It does not affect artifact-based communication, MCP tool interfaces, or self-describing outputs.

### II. Composability First

**Assessment**: PASS

Dewey remains independently installable and usable. This change *restores* the primary `go install` installation path, which was broken. No new dependencies are introduced.

### III. Observable Quality

**Assessment**: N/A

No changes to output formats, provenance metadata, or machine-parseable results. The module path is internal plumbing.

### IV. Testability

**Assessment**: PASS

All existing tests continue to work — import paths are rewritten mechanically. No test isolation or coverage changes. The fix is verified by running the full test suite after the rewrite.
