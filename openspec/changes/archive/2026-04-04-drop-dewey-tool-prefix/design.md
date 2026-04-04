## Context

OpenCode prefixes MCP tool names with the server name. Dewey's 4 custom tools are registered with a `dewey_` prefix, resulting in `dewey_dewey_*` double-prefix in agent UIs. The 37 inherited graphthulhu tools have no prefix and display correctly.

## Goals / Non-Goals

### Goals
- Remove the `dewey_` prefix from 4 tool registration names so agents see `dewey_semantic_search` (not `dewey_dewey_semantic_search`)
- Maintain identical tool functionality — only the name string changes

### Non-Goals
- Renaming inherited graphthulhu tools (they're already correct)
- Changing tool descriptions, parameter schemas, or return types
- Modifying the MCP server name in `opencode.json`

## Decisions

**D1: Rename in `server.go` only.** The tool name is a single string literal in each `mcp.AddTool()` call. No schema, handler, or type changes needed.

**D2: No backward compatibility shim.** MCP clients discover tools dynamically via `tools/list`. No client hardcodes tool names. A clean rename is safe.

**D3: Keep the `dewey_store_learning` description reference.** The tool description says "dewey_semantic_search" — update this to "semantic_search" for consistency.

## Risks / Trade-offs

**Risk: Agent prompts referencing old tool names.** Any `.opencode/agents/` file or skill that mentions `dewey_semantic_search` by name would need updating. Mitigation: search and replace across agent/skill files.

**Trade-off: Loss of tool name uniqueness.** `semantic_search` is less globally unique than `dewey_semantic_search`. This is acceptable because MCP tool names are scoped to the server — the full identifier is still `dewey.semantic_search` in the protocol.
