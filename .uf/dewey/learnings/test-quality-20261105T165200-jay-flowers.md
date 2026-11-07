---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-05T16:52:00Z
identity: test-quality-20261105T165200-jay-flowers
tier: draft
---

In unbound-force Go repos, subprocess-spawning tests carry a testing.Short() skip guard as the test's FIRST statement (go.md convention pack TC-011 [SHOULD]; precedent: dewey artifacts/import_closure_test.go:22). Placement matters: failing-path assertions that invoke the production code under test may already shell out (dewey's TestProductionAsOfResolver runs `git rev-parse` in its non-repo-directory assertion before the git-init setup), so a guard placed after the LookPath or setup section still leaks one subprocess into short mode. Prefer injection seams (e.g. dewey's Learning.WithAsOfResolver) to keep most tests subprocess-free, guarding only the one test that exercises the real production subprocess path. Caught as maintainer feedback on dewey PR #156 (tools/learning_temporal_test.go:199).
