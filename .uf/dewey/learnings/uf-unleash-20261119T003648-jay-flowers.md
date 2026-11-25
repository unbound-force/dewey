---
tag: uf-unleash
author: jay-flowers
category: gotcha
created_at: 2026-11-19T00:36:48Z
identity: uf-unleash-20261119T003648-jay-flowers
tier: draft
as_of: "dewey@50e8ff8ff6df4f2334c0dd2d67b92d99b27f7c2d"
evidence:
  - "dewey:openspec/changes/learning-reingest-on-file-change/tasks.md"
  - "dewey:.uf/artifacts/dispatch/e0d980f4-8a3f-4e9b-a4a0-336431d61980.json"
---

A mid-implementation spec amendment must extend the task decomposition in the same edit. During the learning-reingest-on-file-change change, the drifted-tier scenario was added to LR-FR-001 while resolving the supersede conflict against dewey-distill's no-demote scenario, but tasks.md was not extended to map the new scenario to a named test. The implementation was correct; the clause shipped with zero coverage and surfaced only as the sole MEDIUM finding of code-review iteration 1, costing a voluntary fix iteration (findings went 3 → 1 → 5 LOW across three review iterations, converging at APPROVE WITH ADVISORIES within the 3-iteration budget). Mechanical guard: after any delta-spec amendment, diff the `#### Scenario:` headings against the tasks' named tests — every heading must appear in some task before implementation proceeds. Related: learning test-coverage-20261118T233946-jay-flowers (the scenario-to-test mapping check this incident produced).
