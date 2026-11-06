---
tag: concurrency
author: jay-flowers
category: gotcha
created_at: 2026-11-06T17:35:44Z
identity: concurrency-20261106T173544-jay-flowers
tier: draft
---

When a design defers a known race, the risk note must name the serialization boundary that does NOT hold — not only the one that does. In dewey PR #156 (learning temporal validity), design.md's concurrent-supersession bullet read "of the same target by two processes is not locked; the second writer overwrites superseded_by. Dewey's store lock already serializes writers within one vault" — but the store lock serializes cross-process access only, and the window the reviewer demonstrated is concurrent in-process MCP tool calls under streamable HTTP sharing one store handle: check-then-act between resolveSupersedesTarget validation and linkSuperseded's UpdatePage, mirrored in markSupersededFile's read-modify-write on the markdown backup. The deferral itself was sound (outcome documented, impact metadata-only, maintainer framed the note as future work, review council rated it LOW), yet a future serialization change starting from that note would hunt for cross-process locking and miss the in-process gap. Rule: deferred-risk notes state which concurrency dimension is unprotected (processes vs. in-process calls vs. goroutines) and record the candidate serialization point (CAS UPDATE ... WHERE superseded_by IS NULL, per-target in-process mutex) so the follow-up change starts from an accurate problem statement.
