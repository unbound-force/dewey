## Context

`newInitCmd()` in `cli.go` has an early return at line 232-234 that prevents any code after it from executing on already-initialized repos.

## Goals / Non-Goals

### Goals
- Slash command scaffolding runs on every `dewey init`, not just the first
- Config/sources creation still skipped when already initialized

### Non-Goals
- No `--force` flag (scaffolding is already idempotent)

## Decisions

**D1: Replace early return with conditional block.** Wrap the config/sources/gitignore creation in an `if !alreadyInitialized` block instead of returning early. Slash command scaffolding runs unconditionally after.
