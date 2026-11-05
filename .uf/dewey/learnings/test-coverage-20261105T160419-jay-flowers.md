---
tag: test-coverage
author: jay-flowers
category: gotcha
created_at: 2026-11-05T16:04:19Z
identity: test-coverage-20261105T160419-jay-flowers
tier: draft
---

Compound boolean guards in Go short-circuit, so a later clause is unreachable unless the test fixture defeats every earlier clause: in `if page == nil || page.SourceID != want`, an empty-store test exits at the nil clause and never evaluates the SourceID check — a table case named for the second clause passes while adding zero coverage, and deleting the clause keeps the suite green. Probe for this when reviewing guard coverage: mentally delete each clause of a compound guard and ask which test fails; if none does, that clause is dead in coverage. Design one fixture per clause that satisfies all earlier clauses first (dewey PR #156, resolveSupersededTarget in tools/learning_temporal.go).
