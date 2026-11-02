---
tag: concurrency
author: jay-flowers
category: gotcha
created_at: 2026-11-02T03:15:47Z
identity: concurrency-20261102T031547-jay-flowers
tier: draft
---

When a caching layer's write guard must consult an external store, and the architecture forbids I/O under the cache mutex (dewey's design D2: "store I/O never under c.mu"), the guard is inherently check-then-act: it runs before the write lock, so a concurrent inserter can create the guarded name inside the window and still get shadowed by a create-if-missing write. In dewey's vault write guard (checkStoreBackedWrite, mcp-internal-store-access PR 138), the fix for the deterministic shadowing bug (guard keyed on a cache the live read-through bypasses) necessarily left this microsecond TOCTOU residual because re-checking under the lock would violate the locking discipline and risk lock-ordering problems. Rule: when adding a guard that cannot share the write's lock, size and document the residual race explicitly (who can race, what the window is, whether the failure is recoverable), and prefer guards whose failure mode is integrity-preserving (shadowed but recoverable content) over data loss — then treat the residual as a known limitation in the change record rather than an undocumented gap.
