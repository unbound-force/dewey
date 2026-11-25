---
tag: agent-least-privilege
author: jay-flowers
category: gotcha
created_at: 2026-11-10T23:10:02Z
identity: agent-least-privilege-20261110T231002-jay-flowers
tier: draft
---

OpenCode agent frontmatter `tools:` booleans (read/write/edit/bash/webfetch) gate built-in tools only; they do not scope MCP server tools. An agent bound to an MCP server (e.g. Dewey) inherits the server's full tool surface, including mutating tools such as create_page, delete_page, delete_block, update_block, append_blocks, rename_page. A "least-privilege" or "knowledge-store-only writes" claim for an MCP-bound agent is false unless the mutating MCP tools are individually disabled (or otherwise scoped) and tested. Reviewer check: for any agent the spec calls read-only, enumerate the MCP server's write/delete tools and verify each is denied.
