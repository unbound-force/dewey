---
tag: openspec
author: jay-flowers
category: gotcha
created_at: 2026-11-01T14:25:50Z
identity: openspec-20261101T142550-jay-flowers
tier: draft
---

Before registering a capability under "New Capabilities" in an OpenSpec change, grep BOTH spec trees: openspec/specs/ AND specs/NNN-*/ (Speckit). Dewey's AGENTS.md documents two coexisting spec workflows in one repo, and a capability name reused across them creates dual governance with no cross-reference — e.g., registering a new openspec capability `unified-content-serve` while the completed Speckit spec `specs/004-unified-content-serve/` (including contracts/mcp-tools) governs the same behavior. The openspec/specs/ check alone is insufficient because the Speckit tree is the older, larger governance surface.
