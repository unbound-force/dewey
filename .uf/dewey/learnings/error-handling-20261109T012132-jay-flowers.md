---
tag: error-handling
author: jay-flowers
category: pattern
created_at: 2026-11-09T01:21:32Z
identity: error-handling-20261109T012132-jay-flowers
tier: draft
---

In Dewey MCP tools, a batch fix's partial dual-write failure must surface in the structured tool result, not stderr: MCP clients cannot see server logs (Constitution II confines agent communication to structured tool responses), and the markdown backup is the re-ingestion recovery path when graph.db is deleted, so an unsurfaced store/backup divergence can silently revert validity markings on restart. Count a learning in the fix total once the authoritative store write (UpdatePage) succeeds and carry the markdown backup failure as a warning field in the result — the supersedes_warning precedent (tools/learning.go:350, PR #156). When implementing such a field, pin it with a handler-level test that drives the tool through the backup-failure branch (a frontmatter-less backup makes setFrontmatterKeys fail deterministically) and asserts the field's presence and value — helper-level tests and absence-only assertions leave the result wiring mutation-survivable (see learning/test-quality-20261105T162537-jay-flowers).
