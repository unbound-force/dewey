---
tag: documentation
author: jay-flowers
category: gotcha
created_at: 2026-11-07T16:27:17Z
identity: documentation-20261107T162717-jay-flowers
tier: draft
---

In dewey, only names registered via mcp.AddTool in server.go are callable MCP tools; backend.Backend methods like GetPageBlocksTree are internal surfaces that registered tools invoke. Documentation prose that names backend methods as tools — a conflation inherited from graphthulhu-era artifacts (specs/004-unified-content-serve/contracts/mcp-tools.md) and present in README overview prose and AGENTS.md change notes — misleads MCP clients, who discover capabilities through the tool registry (Constitution III). When reviewing or writing user-facing docs that mention read capabilities, verify each tool name against the server.go registry (the README tool tables are a reliable cross-check: a name absent from every table but present in prose is a suspect). Describe shared backend behavior as "get_page by name, with name-based internal reads sharing the resolution" rather than naming the method. PR #163 (read-tools-bare-names) reworded README.md:31, proposal.md Modified Capabilities, and the spec delta scenario after review feedback; the same conflation remains in pre-existing AGENTS.md prose outside that PR's scope.
