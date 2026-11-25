---
tag: documentation-impact
author: jay-flowers
category: gotcha
created_at: 2026-11-10T23:46:55Z
identity: documentation-impact-20261110T234655-jay-flowers
tier: draft
---

When a change alters `store_learning`, the documentation impact plan must include the in-product MCP-facing surfaces, not just README/AGENTS.md/GoDoc. Concretely: `server.go`'s `store_learning` tool Description and the `Supersedes` jsonschema description in `types/tools.go` are the agent-facing contract (Constitution II: the MCP registry is the primary discovery surface), and the prior `learning-temporal-validity` change (task 6.1) updated `server.go`'s tool description for the same feature — establishing the precedent. Also note README has no section literally named `store_learning`: the supersession prose lives under `### Temporal Validity` (README.md ~778-789, `supersedes`/`superseded_by` row ~787) and the Learning tool table row ~150, so plans should cite the real heading. Automating the docs-surface enumeration (AST scan for the tool description + jsonschema strings tied to changed tool inputs) would remove this guesswork.
