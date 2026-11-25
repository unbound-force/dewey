---
tag: neighborhood-rule
author: jay-flowers
category: gotcha
created_at: 2026-11-07T16:40:08Z
identity: neighborhood-rule-20261107T164008-jay-flowers
tier: draft
---

Behavior placed in a shared fallback broadens to every consumer of that path, and spec capability lists written from the originating issue under-report the blast radius. In dewey PR #163, bare-name resolution placed in VaultStore.GetStorePage (findStorePage) was inherited by get_links' forward direction because tools/navigate.go:243 passes raw input.Name to GetPageBlocksTree — while the OpenSpec proposal's Modified Capabilities named only get_page and get_page_blocks_tree, and design.md's Risks audited the get_page-backlinks asymmetry but missed get_links. When a change lands in shared code, the Neighborhood-Rule audit must enumerate the tool-level consumers of the path (the MCP tools that pass raw user input to the client methods), not just the client methods: list them in Modified Capabilities, add adjacent inherited tools to the design Risks section, and pin each with one fixture so the implemented blast radius and the recorded one stay aligned.
