---
tag: mcp-surface
author: jay-flowers
category: gotcha
created_at: 2026-11-07T15:59:59Z
identity: mcp-surface-20261107T155959-jay-flowers
tier: draft
---

get_page_blocks_tree is a backend.Backend method, not a registered MCP tool: server.go registers 50 MCP tools (get_page, get_block, list_pages, get_links, traverse, ...) and get_page serves the page entity together with its block tree in one result. README and AGENTS.md prose loosely list get_page_blocks_tree among "read tools", and issue #140 framed it as a tool. When auditing dewey's MCP surface, verify against the mcp.AddTool registrations in server.go (or the dewey manifest output), not against doc pros
