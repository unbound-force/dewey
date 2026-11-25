---
tag: sibling-mechanism-regression-tests
author: jay-flowers
category: pattern
created_at: 2026-11-19T04:41:26Z
identity: sibling-mechanism-regression-tests-20261119T044126-jay-flowers
tier: draft
as_of: "dewey@2a7f0147afcf07964a3cb8ce9d52b7790e1a505e"
evidence:
  - "unbound-force/dewey:tools/curate_test.go"
  - "unbound-force/dewey:main_test.go"
  - "unbound-force/dewey#222"
---

When sibling code paths share a fail-closed invariant but achieve it by different mechanisms, each mechanism needs its own regression test. A path that fails closed via an explicit guard (e.g., a scanFailures counter that suppresses the destructive tail) is protected by a test that triggers the guard; a path that fails closed via statement ordering (e.g., recording directory-entry membership in the file set BEFORE the fallible os.ReadFile, so a read failure cannot orphan a live page) is regression-fragile — the ordering looks like incidental structure to a refactor — and needs a discriminating test with a true-positive control: a listed-but-unreadable file (kept page) alongside a genuinely deleted file (pruned page), where moving the recording line makes the count assertion fail exactly at the hazard. Found in dewey's pages-for-deleted-learning-and-knowledge review: the learning path (read-required key) used an explicit pass-level guard and got a two-leg regression test; the knowledge path (entry-name key, safe only through record-before-read ordering) initially had no test pinning the ordering, leaving the mass-delete-on-transient-fault hazard one refactor away from returning.
