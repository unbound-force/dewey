---
tag: spec-quality
author: jay-flowers
category: pattern
created_at: 2026-11-01T14:25:51Z
identity: spec-quality-20261101T142551-jay-flowers
tier: draft
---

When an OpenSpec design defers work as a non-goal, record the deferral everywhere downstream consumers look: (1) if the design says "(noted in tasks as follow-up)", that tasks.md note must actually exist — write it or fix the pointer; (2) the proposal's capability description must be qualified to match the deferred scope ("core read tools" vs "MCP read tools") or the archived capability summary over-promises. Verified pattern from mcp-internal-store-access review: design deferred the query_properties/FindByProperty store merge, claimed a tasks.md follow-up note that did not exist, and the capability description said "MCP read tools serve a unified, live view" while query_properties was the deferred exception.
