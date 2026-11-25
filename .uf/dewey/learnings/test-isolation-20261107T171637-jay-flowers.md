---
tag: test-isolation
author: jay-flowers
category: gotcha
created_at: 2026-11-07T17:16:37Z
identity: test-isolation-20261107T171637-jay-flowers
tier: draft
---

Place environment-availability skip guards (exec.LookPath) before the first test assertion whose code path invokes the guarded executable. In dewey PR #156's TestProductionAsOfResolver, the git guard sat after the non-repository assertion; on machines without git the resolver failed on exec-not-found and the assertion passed vacuously, because "git missing" and "not a repository" both surface as the same non-nil error. Correct ordering: subprocess-free assertions first (verify they short-circuit before touching the executable), then the guard, then subprocess-dependent assertions — an honest skip replaces a vacuous pass, and coverage on environments that have the executable is unchanged.
