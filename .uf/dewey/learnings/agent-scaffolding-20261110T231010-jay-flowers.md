---
tag: agent-scaffolding
author: jay-flowers
category: pattern
created_at: 2026-11-10T23:10:10Z
identity: agent-scaffolding-20261110T231010-jay-flowers
tier: draft
---

Dewey ships reference agents by tracking `.opencode/agents/<name>.md` and mirroring the bytes into a `deweyAgentDefinitions` map in `agents.go`; `dewey init` scaffolds them idempotently inside the existing `.opencode/` composability guard, never overwriting user files, and a byte-identity test (mirroring `TestInitCmd_ScaffoldsDeweyDistillFromMap`) keeps the tracked file, the map entry, and scaffolded output equal. Adding an agent follows the same parity discipline already used for slash commands.
