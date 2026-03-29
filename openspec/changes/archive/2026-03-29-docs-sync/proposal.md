# Proposal: docs-sync

## Why

The Dewey codebase has undergone 10 releases (v0.5.0 through v1.4.1) in a single session, adding major features and fixing critical bugs. The documentation has not kept pace — the README, AGENTS.md, and spec artifacts contain stale information, and the OpenSpec change for `ollama-hard-error` was never archived. A documentation audit using the Dewey MCP server identified 31 issues across 5 categories.

Without this sync:
- The README says `DEWEY_BACKEND` defaults to `logseq` — it actually defaults to `obsidian`
- The README references `brew install ollama` — the Homebrew cask was renamed to `ollama-app`
- Spec 004 status is "Draft" despite all tasks being complete and merged
- The `ollama-hard-error` OpenSpec change sits in active changes instead of the archive
- AGENTS.md is missing documentation for `dewey doctor`, `dewey reindex`, `dewey search`, and multiple flags

## What Changes

- Fix stale content in README.md (default backend, Ollama cask name, missing command docs)
- Update AGENTS.md architecture and command references
- Update spec 004 status to Complete
- Archive the `ollama-hard-error` OpenSpec change
- No production code changes — documentation only

## Capabilities

### New Capabilities
- None

### Modified Capabilities
- None (documentation only)

### Removed Capabilities
- None

## Impact

- **README.md**: Fix `DEWEY_BACKEND` default, `ollama` → `ollama-app`, add `.dewey/dewey.log` and log truncation docs
- **AGENTS.md**: Add `dewey doctor`, `dewey reindex`, `dewey search` to architecture; add `--verbose`, `--log-file`, `--no-embeddings` to conventions
- **specs/004-unified-content-serve/spec.md**: Status Draft → Complete
- **openspec/changes/ollama-hard-error/**: Move to archive

## Constitution Alignment

### I. Autonomous Collaboration
**Assessment**: N/A — documentation only.

### II. Composability First
**Assessment**: N/A — documentation only.

### III. Observable Quality
**Assessment**: PASS — accurate documentation is a core aspect of observable quality. Stale docs reduce the system's self-describing capability.

### IV. Testability
**Assessment**: N/A — documentation only.
