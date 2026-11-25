---
tag: test-architecture
author: jay-flowers
category: decision
created_at: 2026-11-07T19:04:34Z
identity: test-architecture-20261107T190434-jay-flowers
tier: draft
---

When a shared resolution is a spec MUST for a tool whose observable output cannot distinguish whether it fired, verify the indistinguishability against the tool's source and document it in the test file rather than adding an implementation-coupled test. On dewey PR #163, bulk_update_properties always reports store-only pages in its failed list: a read miss yields empty blocks → failed, and a resolved read yields an UpdateBlock refusal (vault blockIndex miss or read-only guard, no identical-content short-cir
