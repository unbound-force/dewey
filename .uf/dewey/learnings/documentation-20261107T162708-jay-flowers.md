---
tag: documentation
author: jay-flowers
category: gotcha
created_at: 2026-11-07T16:27:08Z
identity: documentation-20261107T162708-jay-flowers
tier: draft
---

MCP server documentation (README, website sync issues, OpenSpec capability deltas) must name only tools that exist in the server's actual mcp.AddTool registry — verify each named tool against the registration source before publishing, because inherited prose is not evidence of callability. Backend interface methods consumed internally by tool handlers are not callable by clients: dewey's GetPageBlocksTree (backend/backend.go) backs get_page (tools/navigate.go), search context assembly, journal_range, get_whiteboard, bulk_update_properties, and decision tools, yet README prose has presented it as a read tool. The confusion propagates across doc generations from legacy contract artifacts (specs/004-unified-content-serve/contracts/mcp-tools.md lists dewey_get_page_blocks_tree as a tool). Related: learning/documentation-20261101T183008-jay-flowers (enumerate doc claims from the changed surface, not the doc's own tables).
