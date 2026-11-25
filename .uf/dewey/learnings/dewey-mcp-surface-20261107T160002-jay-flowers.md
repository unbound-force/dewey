---
tag: dewey-mcp-surface
author: jay-flowers
category: gotcha
created_at: 2026-11-07T16:00:02Z
identity: dewey-mcp-surface-20261107T160002-jay-flowers
tier: draft
---

In dewey, 'get_page_blocks_tree' is not a registered MCP tool — server.go registers exactly 50 tools via mcp.AddTool and none carries that name. GetPageBlocksTree is a backend.Backend interface method (vault.Client and Logseq client implementations) that registered tools call internally: get_page (tools/navigate.go GetPage handler), search context assembly, journal_range, get_whiteboard, bulk_update_properties, and the decision tools. Documentation and specs have repeatedly framed it as an MCP read tool (RE
