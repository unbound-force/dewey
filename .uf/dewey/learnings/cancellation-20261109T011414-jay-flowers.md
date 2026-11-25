---
tag: cancellation
author: jay-flowers
category: pattern
created_at: 2026-11-09T01:14:14Z
identity: cancellation-20261109T011414-jay-flowers
tier: draft
---

Per-call timeouts bound a single subprocess but not a loop of them. MCP tool implementations that spawn subprocess-per-element loops must propagate the tool call's context.Context into each invocation — context.WithTimeout(ctx, perCallCeiling) keeps the per-call ceiling while honoring client cancellation — and bound the aggregate with a per-element cardinality cap plus a phase deadline that degrades through the tool's existing warn-and-continue path. Otherwise a cancelled MCP call leaves the server spawning processes with no cancellation path (verified in dewey's lint anchor checks, tools/lint_anchors.go, PR 159 feedback item 6).
