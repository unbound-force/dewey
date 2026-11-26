---
tag: dependency-upgrade
author: jay-flowers
category: gotcha
created_at: 2026-11-25T20:04:42Z
identity: dependency-upgrade-20261125T200442-jay-flowers
tier: draft
as_of: "dewey@23e930d74713be687f0fa0a6e5bdcce08912eb44"
---

When reviewing a modernc.org/sqlite version bump, the SQLITE_OPEN_NOMUTEX-vs-FULLMUTEX threading-mode change is safe if the store uses database/sql with SetMaxOpenConns(1): a single pooled connection is never shared across goroutines, so multi-thread (NOMUTEX) mode has no concurrent access to guard. Dewey's store/store.go sets SetMaxOpenConns(1)/SetMaxIdleConns(1) with WAL and busy_timeout=5000, so the concurrency signal in the review context is mitigated rather than a defect. The residual upgrade risk is opening a pre-existing graph.db created by the older engine, which tests do not exercise.
