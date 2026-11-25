---
tag: test-isolation
author: jay-flowers
category: gotcha
created_at: 2026-11-07T17:16:35Z
identity: test-isolation-20261107T171635-jay-flowers
tier: draft
---

Dewey PR #156 feedback triage (tools/learning_temporal_test.go TestProductionAsOfResolver): an environment-availability skip guard (exec.LookPath("git") -> t.Skip) placed AFTER the first assertion that depends on the external binary produced a vacuous pass — productionAsOfResolver collapses all git failures (binary missing, not-a-repository, timeout) into one error return, so on git-less machines the "non-repository directory" assertion passed on exec-not-found rather than the intended failure mode, then the guard skipped the rest. Rule of thumb: a skip guard for an external binary must precede every assertion whose outcome depends on that binary being present; assertions that fail fast before any subprocess (e.g., empty-vault-path, which returns before shelling out) may safely stay above the guard. Diagnostic heuristic: when a test asserts only "error is non-nil" against a helper that swallows failure causes, ask whether any environment difference (missing binary, permissions, PATH) satisfies the assertion as cheaply as the intended failure — if yes, the assertion needs either the guard above it or a distinguishing check on the error itself.
