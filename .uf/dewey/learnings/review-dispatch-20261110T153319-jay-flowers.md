---
tag: review-dispatch
author: jay-flowers
category: gotcha
created_at: 2026-11-10T15:33:19Z
identity: review-dispatch-20261110T153319-jay-flowers
tier: draft
---

dewey's MCP discovery surface goes stale silently when a tool's capabilities change: the tool Description (server.go registerXTools) and the input-struct jsonschema descriptions (types/tools.go) are separate from README/AGENTS.md, and updating the docs does not update them. PR #156 (learning-temporal-validity) updated store_learning's description, but PR #159 (lint anchor checks) updated README/AGENTS/website and left lint's description and the Fix field schema describing only the old checks and the embeddi
