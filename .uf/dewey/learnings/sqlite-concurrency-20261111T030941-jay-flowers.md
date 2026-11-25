---
tag: sqlite-concurrency
author: jay-flowers
category: gotcha
created_at: 2026-11-11T03:09:41Z
identity: sqlite-concurrency-20261111T030941-jay-flowers
tier: draft
---

SQLite json_extract raises a malformed-JSON error on an empty-string document rather than returning NULL, so COALESCE alone cannot absorb the empty case — compare-and-swap predicates over a JSON properties column need explicit `properties IS NULL OR properties = ''` disjuncts (dewey store/store.go ClaimSupersession, mirroring GetPagesWithProperty). Additionally, SQLite's JSON parser accepts \x hex escapes (a JSON5 extension) that Go's encoding/json rejects, so a SQL guard and a Go re-read of the same row can disagree on parseability: the SQL guard refuses the claim while the Go decode fails. That divergence must fail closed as a hard error (never an empty winner), and deserves a pinned regression test on both sides of the seam.
