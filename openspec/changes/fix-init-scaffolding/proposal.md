## Why

`dewey init` returns early with "already initialized" when `.uf/dewey/` exists, preventing slash command scaffolding from running on already-initialized repos. Users upgrading dewey never get the new slash commands.

Fixes GitHub issue #48.

## What Changes

Restructure `newInitCmd()` so the "already initialized" check only skips config/sources file creation. Slash command scaffolding always runs (idempotently).

## Capabilities

### Modified Capabilities
- `dewey init`: Always scaffolds slash commands into `.opencode/command/` when `.opencode/` exists, even if `.uf/dewey/` already exists

## Impact

- `cli.go`: ~10 line restructure of `newInitCmd()`
- `cli_test.go`: Update existing test + add test for scaffolding on re-init

## Constitution Alignment

### II. Composability First
**Assessment**: PASS — Slash commands only scaffolded when `.opencode/` exists.
