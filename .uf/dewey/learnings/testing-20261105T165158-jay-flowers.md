---
tag: testing
author: jay-flowers
category: gotcha
created_at: 2026-11-05T16:51:58Z
identity: testing-20261105T165158-jay-flowers
tier: draft
---

In dewey, a test that spawns subprocesses — directly or through the code under test — needs a top-of-function `if testing.Short() { t.Skip(...) }` guard (go.md TC-011 [SHOULD]; precedent artifacts/import_closure_test.go:22). Two placement gotchas from PR #156's TestProductionAsOfResolver: (1) an exec.LookPath availability skip does not cover short mode — availability and run-mode brevity are different concerns, keep both guards; (2) cheap-looking error-path assertions can still shell out (productionAsOfResolver runs `git rev-parse --show-toplevel` even when the directory is not a repository), so the Short() guard must precede the assertions, not just the explicit `git init` setup. CI runs `go test -race -count=1 ./...` without `-short`, so the guard is inert in CI and never weakens a gate.
