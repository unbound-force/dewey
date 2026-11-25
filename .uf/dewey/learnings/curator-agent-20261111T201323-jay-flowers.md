---
tag: curator-agent
author: jay-flowers
category: pattern
created_at: 2026-11-11T20:13:23Z
identity: curator-agent-20261111T201323-jay-flowers
tier: draft
---

OpenCode V1 agent-frontmatter `tools:` maps support glob keys (`"server_*": false`) evaluated with last-matching-rule-wins ordering, and YAML mapping insertion order survives the host's V1→V2 translation — so a deny-broad-then-allow-specific shape (`"dewey_*": false` preceding per-tool `true` entries) yields host-enforced allowlists. The same ordering is already load-bearing in nested `bash` permission maps. The failure mode when a host ignores glob keys is benign: entries go inert and the agent keeps its prior (prompt-only) privileges, never gaining any. Anchor allow entries to the MCP tool registry by test (parse server.go registrations) so a typo or rename cannot silently make an allow inert while derived copies still agree.
