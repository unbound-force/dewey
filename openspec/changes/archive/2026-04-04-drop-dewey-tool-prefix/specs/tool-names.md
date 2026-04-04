## MODIFIED Requirements

### Requirement: MCP Tool Registration Names

MCP tools added by Dewey (not inherited from graphthulhu) MUST be registered without the `dewey_` prefix. The MCP server name provides the namespace — tool names SHOULD NOT duplicate it.

Previously: Tools were registered as `dewey_semantic_search`, `dewey_similar`, `dewey_semantic_search_filtered`, `dewey_store_learning`.

#### Scenario: Agent discovers semantic search tool
- **GIVEN** an OpenCode session with Dewey configured as the `dewey` MCP server
- **WHEN** the agent lists available tools
- **THEN** the semantic search tool appears as `dewey_semantic_search` (server prefix + `semantic_search`), not `dewey_dewey_semantic_search`

#### Scenario: Agent calls store learning tool
- **GIVEN** an OpenCode session with Dewey configured as the `dewey` MCP server
- **WHEN** the agent calls `dewey_store_learning`
- **THEN** the tool executes successfully (the server routes `store_learning` to the correct handler)

#### Scenario: Inherited graphthulhu tools unaffected
- **GIVEN** an OpenCode session with Dewey configured
- **WHEN** the agent lists available tools
- **THEN** inherited tools like `get_page`, `search`, `traverse` appear as `dewey_get_page`, `dewey_search`, `dewey_traverse` (unchanged)

### Requirement: Tool Description Cross-References

Tool descriptions that reference other Dewey tool names MUST use the unprefixed registration name.

Previously: `dewey_store_learning` description referenced `dewey_semantic_search`.

#### Scenario: Store learning description references search
- **GIVEN** the `store_learning` tool description
- **WHEN** an agent reads the tool's help text
- **THEN** it references `semantic_search` (not `dewey_semantic_search`)
