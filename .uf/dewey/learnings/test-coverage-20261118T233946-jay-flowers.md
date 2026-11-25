---
tag: test-coverage
author: jay-flowers
category: gotcha
created_at: 2026-11-18T23:39:46Z
identity: test-coverage-20261118T233946-jay-flowers
tier: draft
as_of: "dewey@50e8ff8ff6df4f2334c0dd2d67b92d99b27f7c2d"
evidence:
  - "dewey:.uf/artifacts/dispatch/e0d980f4-8a3f-4e9b-a4a0-336431d61980.json"
---

When a delta spec enumerates scenarios, the test tasks must map every scenario to a named test before implementation — review by diffing the scenario list against the test list. In dewey's learning-reingest-on-file-change change, tasks.md 2.1/2.2 enumerated tests for four of the five new LR-FR scenarios and silently dropped the drifted-tier cheap-path convergence scenario (stored tier non-draft + matching hash → refresh to draft without block work); the implementation was correct but the scenario shipped with zero coverage, and the gap was only caught at code review because the cheap-path refresh guard's tier arm (existing.Tier != tier) is exercised by no test. The mechanical check — each `#### Scenario:` heading in the delta spec appears in some test's doc comment or the tasks list — catches this class of omission at authoring time for free.
