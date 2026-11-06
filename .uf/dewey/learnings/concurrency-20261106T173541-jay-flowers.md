---
tag: concurrency
author: jay-flowers
category: decision
created_at: 2026-11-06T17:35:41Z
identity: concurrency-20261106T173541-jay-flowers
tier: draft
---

Dewey's store lock serializes processes accessing one vault but not concurrent in-process MCP tool calls under streamable HTTP, so validation-before-write sequences that read shared state during validation and write it back afterward keep a check-then-act window even with the store lock in place. PR #156 (tools/learning_temporal.go): resolveSupersedesTarget reads the target's superseded_by during prepareTemporal; linkSuperseded writes the whole target page back via store.UpdatePage from that validation-time snapshot only after the successor's file write, store insert, and embedding generation — a seconds-wide window. Two concurrent StoreLearning calls superseding the same target both pass validation; the second UpdatePage overwrites the first successor, forking the one-successor chain (design D3), and markSupersededFile's read-modify-write on the markdown backup races identically. Deferral assessed sound (PR #156 feedback item 3): metadata-only impact (dangling supersedes claim; both learnings intact; nothing deleted), sequential callers still enforced (resolveSupersedesTarget rejects an already-superseded target), and the complete fix spans both legs — a compare-and-swap store update guarded on the properties JSON (superseded_by lives in the JSON blob per design D1, so the guard needs json_extract or a schema change) or a per-target in-process mutex, plus the file leg — a real increment deserving its own spec. Rule of thumb: when deferring a concurrency trade-off documented in design Risks, scope the risk bullet to the exact unserialized window (name the concurrency source the lock does not cover), note every leg that races, and record the candidate fix in the artifact, not just the review thread.
