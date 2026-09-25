## Why

PR #75 corrected compiled history summary truncation to operate on Unicode code points instead of bytes, preventing malformed UTF-8 when a multibyte character crosses the truncation boundary. The fix lacks regression coverage, so a future return to byte slicing could silently corrupt generated Markdown. Issue #81 requests focused coverage to satisfy convention rule TC-006 and preserve the corrected behavior.

## What Changes

- Add a table-driven regression test for compiled article history summary truncation.
- Cover multibyte boundary cases using an em dash, a CJK character, and an emoji, plus an exactly 80-rune non-truncating case.
- Assert that generated articles remain valid UTF-8 and that summaries contain the expected 80-rune prefix and ellipsis behavior.
- Verify the focused tools package tests and the repository's CI-equivalent quality gates.

## Capabilities

### New Capabilities

- `knowledge-compilation`: Capture the existing compiled history summary contract and require rune-safe truncation that preserves valid UTF-8.

### Modified Capabilities

None.

### Removed Capabilities

None.

## Impact

The implementation affects only regression coverage in `tools/compile_test.go` and does not alter production behavior, MCP contracts, CLI output, dependencies, or persisted data. No README, AGENTS.md, GoDoc, or website documentation update is required because the behavior is already implemented and this change is test-only.

## Constitution Alignment

Assessed against the Unbound Force org constitution.

### I. Autonomous Collaboration

**Assessment**: PASS

The regression test verifies compiled Markdown through the existing package contract and introduces no runtime coupling, shared memory, or direct cross-component calls. MCP tools and artifact-based collaboration remain unchanged.

### II. Composability First

**Assessment**: PASS

The regression test runs within the `tools` package without external services or new mandatory dependencies, preserving Dewey's standalone operation.

### III. Observable Quality

**Assessment**: PASS

The test verifies the observable compiled Markdown output, including valid UTF-8, the exact retained summary prefix, and ellipsis behavior. It protects the history section that provides learning provenance in compiled articles.

### IV. Testability

**Assessment**: PASS

The change directly strengthens isolated, deterministic test coverage for a confirmed defect. The test must fail when byte slicing is restored and pass with the current rune-safe implementation.

<!-- scaffolded by uf v0.17.0 -->
