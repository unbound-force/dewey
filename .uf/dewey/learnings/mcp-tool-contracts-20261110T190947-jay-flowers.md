---
tag: mcp-tool-contracts
author: jay-flowers
category: gotcha
created_at: 2026-11-10T19:09:47Z
identity: mcp-tool-contracts-20261110T190947-jay-flowers
tier: draft
---

MCP tool behavior changes in dewey have three documentation surfaces that must move together: the static Description strings passed to mcp.AddTool in server.go's registerXTools functions, the jsonschema tags on the input structs in types/tools.go, and the README tool tables. PR #160 (validity-aware retrieval) updated the tags and README but left server.go:434 saying "ranked by cosine similarity" after default ranking became validity-weighted — a gap that survived a full council round because none of the three surfaces is derivable from the others. When authoring or reviewing a change to an MCP tool's behavior, grep server.go for the tool's registration block alongside the types/ and README edits.
