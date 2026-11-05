---
tag: test-conventions
author: jay-flowers
category: pattern
created_at: 2026-11-05T16:51:56Z
identity: test-conventions-20261105T165156-jay-flowers
tier: draft
---

dewey test convention for subprocess-spawning tests: include both an exec.LookPath skip (the binary may be absent) and a testing.Short() skip placed at the very top of the test — before any error-path calls that also shell out. In TestProductionAsOfResolver (tools/learning_temporal_test.go), the non-repository error-path check executes `git rev-parse --show-toplevel` just like the happy path, so a guard placed after the LookPath check would still leave subprocess calls running in short mode. Rule: go.md TC-011 [SHOULD]. Precedents: artifacts/import_closure_test.go:22 (module-graph walk) and tools/learning_temporal_test.go:199 (git resolver). CI runs full mode, so the guard is a developer-convenience/hermeticity convention, not a CI requirement.
