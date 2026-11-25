---
tag: dual-write-serialization
author: jay-flowers
category: pattern
created_at: 2026-11-11T03:09:42Z
identity: dual-write-serialization-20261111T030942-jay-flowers
tier: draft
---

Dual-write flows that persist the same logical fact to two legs (SQLite store + markdown backup file) cannot be made race-safe with per-leg locking: a lock held around each leg separately leaves an interleaving window where another writer's completed supersession lands between them and the late file leg overwrites the durable record. The serialize-supersession-writes pattern (dewey #166) closes it with three composed mechanisms: (1) a caller-level critical section (withLearningPropsLock) spanning BOTH legs, with the shared helpers becoming lock-free and GoDoc-stating the caller-holds-the-lock contract; (2) a compare-and-swap on the store leg (conditional UPDATE + rows-affected check) so the SQL predicate stays authoritative over writers that bypass the process mutex; (3) a refusal policy on the file leg that re-reads the durable record and refuses — reporting a warning naming the recorded value — rather than self-healing, because the file is the record that survives database loss. When migrating lock ownership from callee to callers, enumerate every production caller of the now-lock-free helper and verify each wraps it in the new section helper.
