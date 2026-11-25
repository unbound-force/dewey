---
tag: go-testing
author: jay-flowers
category: pattern
created_at: 2026-11-19T04:41:30Z
identity: go-testing-20261119T044130-jay-flowers
tier: draft
as_of: "dewey@2a7f0147afcf07964a3cb8ce9d52b7790e1a505e"
evidence:
  - "unbound-force/dewey:main_test.go"
  - "unbound-force/dewey:tools/curate_test.go"
  - "unbound-force/dewey#222"
---

Deterministic POSIX fault injection for fail-closed tests, proven in dewey's pruning test suite: (1) replacing a directory with a regular file makes os.ReadDir fail with ENOTDIR — a non-IsNotExist error on every platform — as a stand-in for permissions/I/O scan failures; (2) a dangling symlink is still listed by os.ReadDir as a .md entry but makes os.ReadFile fail with ENOENT, driving per-file read-failure legs; (3) chmod 000 is NOT a deterministic failure on macOS, where the owning user can often still read the file — prefer the first two techniques; (4) for per-page delete failures, a scoped SQLite BEFORE DELETE trigger (SELECT RAISE(ABORT, ...) WHEN OLD.name = '<page>') injected through store.DB().Exec exercises the production warn-and-continue path without mocking the store interface — the abort fires before the row is removed so the cascade never runs; drop the trigger in a defer registered AFTER the store-close defer so LIFO keeps the DB open for the drop; (5) order premises on ListPagesBySource's name ordering to control which deletion fails first. Gotcha: SQLite offers no trigger on SELECT, so page-listing-error branches (ListPagesBySource failures) cannot be fault-injected this way, and closing the DB breaks follow-up assertions — such two-line error guards may be practically untestable and are reasonable to accept as documented gaps.
