---
tag: documentation
author: jay-flowers
category: gotcha
created_at: 2026-11-10T19:09:55Z
identity: documentation-20261110T190955-jay-flowers
tier: draft
---

In dewey, MCP tool semantics are documented in two independent places: the jsonschema tags on input/result structs in types/tools.go, and the mcp.AddTool Description strings in server.go's register*Tools functions. A behavior change that updates the jsonschema descriptions leaves the server.go registration descriptions silently stale — after PR #160 landed validity-weighted ranking and the validity filter, semantic_search's registration still said "ranked by cosine similarity" and semantic_search_filtered's still omitted the filter, while both jsonschema descriptions were correct. Treat the server.go Description strings as a second documentation surface and update them in the same change that alters tool semantics.
