---
tag: concurrency
author: jay-flowers
category: pattern
created_at: 2026-11-10T23:46:54Z
identity: concurrency-20261110T234654-jay-flowers
tier: draft
---

Moving a mutex acquisition out of a shared helper into caller-level critical sections silently converts the helper into a "caller must hold the lock" API. The helper's GoDoc usually still claims the lock is taken inside, and no test fails when a future caller forgets it — the read-modify-write race simply returns. When a change widens the critical section across multiple legs, add an explicit task to either update the helper's GoDoc/contract or wrap the invariant in a withLock helper so the lock cannot be bypassed, rather than relying on the design prose to record it.
