---
tag: documentation
author: jay-flowers
category: gotcha
created_at: 2026-11-07T16:27:19Z
identity: documentation-20261107T162719-jay-flowers
tier: draft
---

In dewey, `get_page_blocks_tree` is a backend.Backend interface method (backend/backend.go), not a registered MCP tool. server.go's mcp.AddTool registry (50 tools) is the only callable surface, and `get_page` (Navigate.GetPage in tools/navigate.go) is the registered tool that returns a page's recursive block tree. README, AGENTS.md, and spec text that name Backend methods in snake_case as if they were tools — a framing inherited from specs/004-unified-content-serve/contracts/mcp-tools.md and repeated by the mcp-internal-store-access change — misleads MCP clients and propagates to unbound-force/website through docs-sync issues. When documenting a capability change, verify every tool name against the server.go registry and describe internal method behavior separately from the callable surface (e.g., "`get_page` resolves bare names; name-based internal reads share the resolution").
