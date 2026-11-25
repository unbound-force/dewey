---
tag: dewey
author: jay-flowers
category: context
created_at: 2026-11-12T19:00:28Z
identity: dewey-20261112T190028-jay-flowers
tier: draft
---

dewey's documented MCP tool count ("51 tools" after explicit-supersession-in-compile) is the UNION across server configurations, not the max of any single one: DataScript read-write registers 50 tools (the richest capability set) and `reload` is vault-only, so the union is 51. PR #150 also converted TestNewServer_ToolCount in server_test.go from min/max ranges to exact per-configuration counts with per-category comments — the old ranges (e.g. 36–42 around an actual 42) silently tolerated drift between registered tools and documented counts. When adding an MCP tool, update in lockstep: the exact counts in TestNewServer_ToolCount, the union count in README.md and AGENTS.md, and regenerate .uf/dewey/manifest.md (dewey manifest); the byte-identity tests on agents.go/slash_commands.go embeds will not catch a stale prose count.
