---
tag: documentation-plan
author: jay-flowers
category: gotcha
created_at: 2026-11-11T14:51:08Z
identity: documentation-plan-20261111T145108-jay-flowers
tier: draft
---

Dewey's curator agent allowlist is restated in three tracked code/scaffold surfaces (.opencode/agents/curator.md, the agents.go embedded mirror asserted byte-identical by cli_test.go) AND in one documentation surface: README.md's Knowledge Compilation section paragraph describing the curator's least-privilege MCP surface ("lint, list_pages, compile, curate, store_compiled, and promote"). AGENTS.md restates the allowlist nowhere. A change that extends the allowlist must include the README restatement in its documentation tasks — specs that enumerate only the agent file and its mirror ship a README that misstates the curator's tool surface.
