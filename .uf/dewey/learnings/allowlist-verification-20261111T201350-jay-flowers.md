---
tag: allowlist-verification
author: jay-flowers
category: pattern
created_at: 2026-11-11T20:13:50Z
identity: allowlist-verification-20261111T201350-jay-flowers
tier: draft
---

Anchoring allowlist tests to the source-of-truth registry closes a gap that copy-parity tests cannot see: when every derived copy (frontmatter allow set, prompt-text allowlist, command file) agrees on a stale or mistyped tool name, set-equality between the copies stays green while the allow entry is inert against the real registry. Deriving expected names from the registration source (parsing server.go's registered MCP tool names) and failing closed when the config source (repository-root opencode.json mcp map) is missing, empty, or missing expected keys makes both drift directions — document drift and registry drift — mechanically caught.
