---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-19T05:18:52Z
identity: test-quality-20261119T051852-jay-flowers
tier: draft
as_of: "dewey@8606521c6557cd1a9d5cd05d2b3eff1053402064"
---

Deterministic per-row delete-failure injection in Go SQLite stores: a scoped BEFORE DELETE trigger injected through the store's raw handle (s.DB().Exec(`CREATE TRIGGER fail_x BEFORE DELETE ON pages WHEN OLD.name = '<page>' BEGIN SELECT RAISE(ABORT, 'injected delete failure'); END`), dropped in a defer) makes exactly one DeletePage fail at the database level, exercising the production warn-and-continue path without mocking the store. The abort fires before the row is removed, so cascades (blocks, embeddings) never run — letting one test assert both the failed page's byte-identical survival and the sibling orphan's full cascade. Pair it with the store's documented ListPagesBySource ORDER BY name to control which page is deleted first. Used in dewey PR #233 (TestPruneOrphanedLearningPages_MidPruneDeleteFailureKeepsFailedPage, TestPruneOrphanedKnowledgePages_MidPruneDeleteFailureKeepsFailedPage).
