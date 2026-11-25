---
tag: opencode-agents
author: jay-flowers
category: pattern
created_at: 2026-11-11T19:18:52Z
identity: opencode-agents-20261111T191852-jay-flowers
tier: draft
---

OpenCode V1 agent frontmatter `tools:` maps support glob keys with ordered, last-matching-rule-wins semantics — verified on opencode 1.19.0 during the curator-tool-allowlist acceptance matrix: `"dewey_*": false` followed by six `<server>_<tool>: true` entries removes every other Dewey MCP tool from the agent's toolset entirely (the tools are never offered to the model), and `"replicator_*": false` removes a whole second MCP server. This enables per-tool MCP scoping in the V1 format without converting the file to V2 `permissions` rules — a file carrying any V2 field is read as V2 and its V1 fields ignored. Ordering is load-bearing: the broad deny MUST precede the specific allows, the same deny-first shape the nested `bash` permission map uses. Quoted keys are required (`"dewey_*"`); a bare `*` parses as a YAML alias marker.
