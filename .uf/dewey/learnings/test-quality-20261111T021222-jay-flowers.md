---
tag: test-quality
author: jay-flowers
category: gotcha
created_at: 2026-11-11T02:12:22Z
identity: test-quality-20261111T021222-jay-flowers
tier: draft
---

A test for one predicate inside a multi-clause WHERE guard is worthless if the fixture also trips an earlier predicate in the same clause. store/store_test.go's new ClaimSupersession "non-learning source_id" case seeded `{"superseded_by":"..."}` on a non-learning row, so the properties predicate produced the zero-row refusal and the source_id predicate was never consulted — deleting `AND source_id = ?` would leave the suite green. Rule: for each conjunct that a test claims to pin, construct the fixture so every other conjunct evaluates true and only the target conjunct is false. The masked variant here is "row of the wrong source_id with unclaimed properties".
