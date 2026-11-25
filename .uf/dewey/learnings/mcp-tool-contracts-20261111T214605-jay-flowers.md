---
tag: mcp-tool-contracts
author: jay-flowers
category: gotcha
created_at: 2026-11-11T21:46:05Z
identity: mcp-tool-contracts-20261111T214605-jay-flowers
tier: draft
---

When a slash command tells an agent to feed one MCP tool's output into another tool's identifier parameter, the instruction must name the exact identifier field, not a category of artifact. `/dewey-distill`'s verify step said to pass "article paths from `compile`", but `compile` reports a filesystem `path` and a human-readable `topic` ("Authentication"), never the `compiled/{auth}` page name that `verify_candidates` resolves by exact name; similarly `curate` reports only counts, and its pages are curated-tier, not the draft tier the verifier admits. Following the instruction literally makes the verifier return skipped for every candidate the step exists to check. Write command prose against the actual JSON field names each upstream result exposes, and verify each field resolves in the downstream tool.
