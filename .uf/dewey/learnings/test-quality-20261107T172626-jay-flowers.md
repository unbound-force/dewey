---
tag: test-quality
author: jay-flowers
category: gotcha
created_at: 2026-11-07T17:26:26Z
identity: test-quality-20261107T172626-jay-flowers
tier: draft
---

A test that computes its expected value by calling the same helper the code under test uses cannot fail for regressions inside that helper — both sides of the assertion change together. In dewey PR #156, TestProductionAsOfResolver derived its expected SHA from gitOutput, the exact helper productionAsOfResolver calls internally, so a trimming regression in gitOutput would have kept the suite green while corrupting every stored as_of pin. Fix pattern: derive subprocess oracles with a direct exec.Command in the test (mirroring the documented output contract, e.g. TrimSpace of rev-parse stdout) so the oracle stays independent of the helper under test; the same test's repo-setup leg already used direct exec.Command for git init/commit.
