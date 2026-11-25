---
tag: dewey-agent-scaffolding
author: jay-flowers
category: gotcha
created_at: 2026-11-11T14:14:33Z
identity: dewey-agent-scaffolding-20261111T141433-jay-flowers
tier: draft
---

Dewey ships its tracked agent and slash-command files through raw string literals in agents.go (deweyAgentDefinitions) and slash_commands.go (deweySlashCommands), and cli_test.go asserts byte-identical parity between the tracked file and the map entry (cli_test.go around 4746-4751 for curator.md). Any spec task that edits .opencode/agents/*.md or .opencode/commands/*.md must pair the tracked-file edit with the mirror edit, or go test -race -count=1 ./... fails in CI. Spec reviews should check for the mirror task explicitly; command files have historically received one while agent files did not.
