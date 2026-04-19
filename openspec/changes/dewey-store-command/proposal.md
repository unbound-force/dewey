## Why

Developers frequently encounter useful information in Slack DMs, meeting chats, or other sources that aren't configured as Dewey sources. Today, storing this knowledge requires the developer to know the `store_learning` MCP tool API — specifically the `tag`, `category`, and `information` parameters. There's no discoverable slash command in the command palette.

A `/dewey-store` command gives developers a frictionless way to paste text and store it as searchable knowledge. When tag and category aren't provided, the agent analyzes the text and suggests appropriate values — the developer confirms or overrides with a single keystroke.

## What Changes

Create a new slash command at `.opencode/command/dewey-store.md` that:

1. Accepts text with optional `--tag` and `--category` flags (fully specified mode)
2. When flags are omitted, analyzes the pasted text and suggests tag and category with reasoning
3. Supports `--extract` mode that breaks a long conversation into multiple distinct learnings
4. After storing, suggests running `/dewey-compile` to synthesize with existing knowledge

No Go code changes — this is a command definition file only.

## Capabilities

### New Capabilities
- `/dewey-store` slash command: 3 interaction modes (fully specified, suggested, extract)

## Impact

- `.opencode/command/dewey-store.md`: New file (command definition only)
- No production code changes, no test changes, no schema changes

## Constitution Alignment

### I. Autonomous Collaboration
**Assessment**: PASS — Uses the existing `store_learning` MCP tool. No new runtime coupling.

### II. Composability First
**Assessment**: PASS — Command is a thin wrapper around an existing MCP tool. Works only when Dewey is available; no degradation needed for a store command.

### III. Observable Quality
**Assessment**: PASS — Returns the `{tag}-{sequence}` identity so the developer can verify storage.

### IV. Testability
**Assessment**: N/A — Command definition file, not code.
