---
tag: dewey-distill
author: jay-flowers
category: gotcha
created_at: 2026-11-11T15:17:35Z
identity: dewey-distill-20261111T151735-jay-flowers
tier: draft
---

Adding a new MCP tool to the /dewey-distill pipeline requires updating four coupled surfaces, not two: the tracked .opencode/commands/dewey-distill.md, its byte-identical embedded copy in slash_commands.go, the curator agent allowlist in .opencode/agents/curator.md AND its byte-identical embedded copy in agents.go (deweyAgentDefinitions), plus the allowlist slice in cli_test.go. TestInitCmd_ScaffoldsDeweyDistillFromMap fails in two independent ways if the allowlist is missed: the set-equality assertion on agentAllowlistEntries, and the fail-closed check that rejects any registered tool name appearing in the command but absent from the allowlist. Because agent tool-access settings are a constitution-protected surface, the allowlist extension must also be a deliberate, recorded decision rather than an incidental side effect of editing the command.
