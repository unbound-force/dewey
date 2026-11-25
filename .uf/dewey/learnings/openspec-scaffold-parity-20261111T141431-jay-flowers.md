---
tag: openspec-scaffold-parity
author: jay-flowers
category: gotcha
created_at: 2026-11-11T14:14:31Z
identity: openspec-scaffold-parity-20261111T141431-jay-flowers
tier: draft
---

In the dewey repo, scaffolded agent and slash-command files are shipped from Go raw-string maps (deweyAgentDefinitions in agents.go, deweySlashCommands in slash_commands.go), and cli_test.go asserts the tracked Markdown is byte-identical to those entries. A change that edits .opencode/agents/*.md or .opencode/commands/*.md must update the Go map in the same task and list it in the Impact inventory; editing only the tracked file fails CI and makes dewey init scaffold stale content.
