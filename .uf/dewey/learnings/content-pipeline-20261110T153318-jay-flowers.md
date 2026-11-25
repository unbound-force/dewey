---
tag: content-pipeline
author: jay-flowers
category: gotcha
created_at: 2026-11-10T15:33:18Z
identity: content-pipeline-20261110T153318-jay-flowers
tier: draft
---

When a dewey MCP tool's behavior changes (new checks, finding types, or fix semantics), the tool's registry-facing description must be updated alongside README/AGENTS/website-sync: the explicit mcp.AddTool Description string in server.go and the handler method's GoDoc. PR #159 (lint-anchor-drift) updated README, AGENTS.md, the NewLint GoDoc, the Finding-struct comment, and filed website#398/#399, yet left server.go's lint Description enumerating only the four original checks and the Lint method GoDoc saying
