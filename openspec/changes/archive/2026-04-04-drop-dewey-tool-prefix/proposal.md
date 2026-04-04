## Why

OpenCode prefixes MCP tool names with the server name: `{server}_{tool}`. Since the Dewey MCP server is named `dewey` in `opencode.json` and the Dewey-specific tools are registered as `dewey_semantic_search`, `dewey_similar`, `dewey_semantic_search_filtered`, and `dewey_store_learning`, agents see them as `dewey_dewey_semantic_search` — a redundant double prefix that is confusing and wastes prompt tokens.

The inherited graphthulhu tools (`get_page`, `search`, `traverse`, etc.) display correctly as `dewey_get_page` because they don't have the `dewey_` prefix in their registration name.

## What Changes

Remove the `dewey_` prefix from the 4 Dewey-added MCP tool registration names in `server.go`:

| Current Name | New Name |
|-------------|----------|
| `dewey_semantic_search` | `semantic_search` |
| `dewey_similar` | `similar` |
| `dewey_semantic_search_filtered` | `semantic_search_filtered` |
| `dewey_store_learning` | `store_learning` |

## Capabilities

### Modified Capabilities
- `semantic_search`: Renamed from `dewey_semantic_search` — same functionality
- `similar`: Renamed from `dewey_similar` — same functionality
- `semantic_search_filtered`: Renamed from `dewey_semantic_search_filtered` — same functionality
- `store_learning`: Renamed from `dewey_store_learning` — same functionality

## Impact

- **server.go**: 4 tool name strings changed in `registerSemanticTools()` and `registerLearningTools()`
- **Agent prompts**: Any agent or documentation that references `dewey_semantic_search` by name needs updating to `semantic_search`. However, since OpenCode agents call tools by the prefixed name (`dewey_semantic_search`), the effective tool name agents see remains the same — this is transparent to callers.
- **Backward compatibility**: MCP tool names are the contract. Any client hardcoding `dewey_semantic_search` as the tool name would break. In practice, OpenCode discovers tools dynamically so this is non-breaking.

## Constitution Alignment

Assessed against the Unbound Force org constitution.

### I. Autonomous Collaboration

**Assessment**: PASS

Tool names are the artifact-based communication interface. Removing the redundant prefix makes tool names clearer without changing the communication protocol or payload format.

### II. Composability First

**Assessment**: PASS

Dewey remains independently installable. The tool rename is internal to the MCP server registration — no external dependencies are introduced.

### III. Observable Quality

**Assessment**: PASS

All tool outputs retain their existing structure and provenance metadata. Only the registration name changes, not the response format.

### IV. Testability

**Assessment**: PASS

Tool tests reference tool names via the handler functions, not by string name. The rename does not affect test isolation or testability.
