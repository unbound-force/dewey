# Design: docs-sync

## Context

A documentation audit using the Dewey MCP server identified 31 issues across README.md, AGENTS.md, spec artifacts, and the website. This change addresses the 12 issues in the dewey repo itself (the 19 website issues require a separate spec in the website repo).

## Goals / Non-Goals

### Goals
- Fix all factually incorrect documentation in README.md and AGENTS.md
- Update spec 004 status to reflect completion
- Archive the unarchived `ollama-hard-error` OpenSpec change
- Ensure documentation matches the v1.4.1 codebase

### Non-Goals
- Updating the website repo (separate spec needed there)
- Adding new documentation sections beyond what's stale
- Changing production code

## Decisions

**D1: Fix README.md environment variables table**

Change `DEWEY_BACKEND` default from `logseq` to `obsidian`. This was changed in the code long ago but the README was never updated.

**D2: Fix README.md Ollama install command**

The line `brew install ollama` at line 352 was already updated to `brew install --cask ollama-app` in the v0.5.0 release. Verify this is still correct.

**D3: Update AGENTS.md architecture section**

Add `dewey doctor`, `dewey reindex`, `dewey search` to the CLI commands list. Add `--verbose`, `--log-file`, `--no-embeddings` to the flags documentation. Update the package description for `vault/parse_export.go` (new file).

**D4: Update spec 004 status**

Change `**Status**: Draft` to `**Status**: Complete` in `specs/004-unified-content-serve/spec.md`. All 46 tasks are checked, all PRs merged, the feature is released as v1.0.0+.

**D5: Archive ollama-hard-error OpenSpec change**

Move `openspec/changes/ollama-hard-error/` to `openspec/changes/archive/2026-03-29-ollama-hard-error/`.

## Risks / Trade-offs

**Low risk**: Documentation-only changes. No production code affected. The only file system operation is the `mv` for the OpenSpec archive.
