---
tag: test-architecture
author: jay-flowers
category: pattern
created_at: 2026-11-11T20:13:38Z
identity: test-architecture-20261111T201338-jay-flowers
tier: draft
---

Registry-anchored allowlist testing: when several copies of an allowlist must stay in parity (agent frontmatter tools map, prompt-text list, embedded scaffold copy), set-equality between the copies cannot catch a shared typo — all copies agree on a stale or misspelled tool name while the host-level glob silently denies the real tool. Anchor at least one side to the source-of-truth registry instead: derive the registered tool names mechanically (dewey's cli_test.go parses server.go's MCP tool registrations via registeredMCPToolNames) and assert every allowlisted name resolves there, so a typo, a rename, or a stale name fails loudly. Combined with deny-all-then-allow ordering assertions over an order-preserving yaml.Node parse and exact key-set pinning, this turns a copy-sync check into a fail-closed boundary guard (see TestCuratorAgent_FrontmatterToolAllowlist, CA-FR-009/010/011).
