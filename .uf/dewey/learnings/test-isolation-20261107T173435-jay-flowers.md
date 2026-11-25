---
tag: test-isolation
author: jay-flowers
category: gotcha
created_at: 2026-11-07T17:34:35Z
identity: test-isolation-20261107T173435-jay-flowers
tier: draft
---

Adding a defaulted, side-effecting dependency to an existing constructor silently re-couples every pre-existing test construction to the environment. In dewey PR #156, NewLearning gained asOfResolver defaulting to productionAsOfResolver(vaultPath) — with a field comment saying injection exists \"so tests never depend on the runner's git state\" — yet eight pre-existing StoreLearning-calling constructions across learning_test.go, server_test.go, and integration_test.go were never swept and began spawning git subprocesses probing TMPDIR on every call. When adding a defaulted dependency that shells out or probes the environment: grep every NewX(...) call site in *_test.go before merging; inject the documented disable option (e.g. WithAsOfResolver(nil)) at constructions that exercise the side-effecting path with a non-empty path argument (empty-path constructions often fail fast before the side effect); and keep the production default's real coverage in a dedicated test that constructs the environment deliberately (temp git repo, testing.Short()-guarded, exec.LookPath-skipped) rather than relying on incidental execution whose result no assertion checks.
