---
tag: documentation
author: jay-flowers
category: gotcha
created_at: 2026-11-10T15:33:17Z
identity: documentation-20261110T153317-jay-flowers
tier: draft
---

When an MCP tool whose registry description enumerates capabilities gains a new capability, three documentation surfaces must move together: user docs (README/AGENTS), the handler's GoDoc, and the mcp.AddTool Description in the server registration file. The dewey Documentation Validation Gate names README/AGENTS/GoDoc/spec artifacts but not registry descriptions, so the `lint` tool description in server.go drifted stale one check at a time — after PR #159 (anchor checks) it still enumerates 4 of 8 check kin
