---
tag: dewey-architecture
author: jay-flowers
category: decision
created_at: 2026-11-11T02:08:01Z
identity: dewey-architecture-20261111T020801-jay-flowers
tier: draft
---

dewey gate auto-answer ships as a CLI subcommand despite the constitution's Development Standards Protocol rule that new capabilities MUST be exposed as MCP tools. The exception follows the dewey doctor/manifest/artifacts operational-command precedent: a startup gate must run pre-session (before the MCP-dependent pipeline it authorizes), needs its own exit code for command prose to consume (determinism pack DE-003), and must never touch the store lock. The proposal surfaces the exception for explicit human ratification instead of silently weakening the rule — the correct move under the Gatekeeping Value Protection rule.
