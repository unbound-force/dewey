---
tag: test-quality
author: jay-flowers
category: gotcha
created_at: 2026-11-07T16:25:25Z
identity: test-quality-20261107T162525-jay-flowers
tier: draft
---

In Go tests that verify subprocess-dependent behavior, place tool-availability guards (exec.LookPath) before any assertion that invokes the subprocess. An error-expecting assertion placed before the guard passes vacuously on machines lacking the tool — the error arrives from exec-not-found instead of the condition under test — so the assertion silently verifies nothing exactly where the guard exists to acknowledge the tool may be missing. Observed in dewey's TestProductionAsOfResolver (PR #156): the non-rep
