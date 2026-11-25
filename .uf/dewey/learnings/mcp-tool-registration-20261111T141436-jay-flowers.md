---
tag: mcp-tool-registration
author: jay-flowers
category: pattern
created_at: 2026-11-11T14:14:36Z
identity: mcp-tool-registration-20261111T141436-jay-flowers
tier: draft
---

In dewey, adding an MCP tool is not just server.go registration plus a tool file. It also requires updating hardcoded counts in AGENTS.md (Project Overview and Architecture: "50 MCP tools"/"50 tool registrations") and README (architecture and curator allowlist paragraph), any exact allowlist enumeration for the curator agent, and the dewey-distill command tests, which hardcode both the six-tool allowlist and the numeric gate-section headings (renumbering a step breaks the hardcoded gateSections map in TestDeweyDistill_CuratorBindingAndHumanGates).
