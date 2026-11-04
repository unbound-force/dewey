---
tag: toctou
author: jay-flowers
category: pattern
created_at: 2026-11-04T17:31:40Z
identity: toctou-20261104T173140-jay-flowers
tier: draft
---

A check-then-act refusal over a shared store is only as strong as the set of writers that participate in its serialization. In dewey PR 145, store_compiled acquired the pipeline mutex and then checked the page tier before overwriting — but promote uses a standalone atomic conditional UPDATE and never takes that mutex, so a promote committing between the check and the write replaced human-validated content while the SQL-level CASE WHEN guard kept the validated label, bypassing the refusal. The deterministic fix is to enforce the refusal inside the same store operation that replaces content (conditional UPDATE with WHERE tier != 'validated' plus a rows-affected check, refusing on zero rows) — the same optimistic-concurrency pattern the racing writer itself uses — rather than widening the mutex, which a standalone-SQL writer cannot honor without a lock-ordering redesign. Rule of thumb: for every trust-boundary check-then-act, enumerate ALL writers to the same rows and verify each either holds the same lock or is defeated by an atomic guard on the write itself.
