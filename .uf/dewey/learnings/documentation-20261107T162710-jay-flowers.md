---
tag: documentation
author: jay-flowers
category: gotcha
created_at: 2026-11-07T16:27:10Z
identity: documentation-20261107T162710-jay-flowers
tier: draft
---

Dewey documentation must name the callable MCP surface, not backend.Backend methods. server.go registers the 50-tool MCP registry and `get_page_blocks_tree` is not among them: `GetPageBlocksTree` (backend/backend.go:17, implemented in vault/vault.go and client/logseq.go) is a Backend method that the registered `get_page` tool (tools/navigate.go:35) and several other handlers (search context assembly, journal, whiteboard, bulk_update_properties, decision tools) call internally. The loose naming inherits from the graphthulhu-era contract artifact (specs/004-unified-content-serve/contracts/mcp-tools.md lists `dewey_get_page_blocks_tree`) and still appears in AGENTS.md's mcp-internal-store-access changelog entry and older OpenSpec proposals. When documenting read behavior in the README overview, OpenSpec Modified Capabilities, or spec acceptance scenarios, cite `get_page` and note that name-based internal reads share the resolution.
