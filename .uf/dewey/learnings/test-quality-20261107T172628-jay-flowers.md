---
tag: test-quality
author: jay-flowers
category: gotcha
created_at: 2026-11-07T17:26:28Z
identity: test-quality-20261107T172628-jay-flowers
tier: draft
---

Test-oracle independence: when a test computes its expected value by calling a production helper that the code under test also calls, regressions inside that helper alter both sides equally and the assertion cannot fail. Verified in unbound-force/dewey PR #156: TestProductionAsOfResolver derived its expected SHA from gitOutput — the same helper productionAsOfResolver uses — so dropping gitOutput's strings.TrimSpace would have corrupted every stored as_of value (trailing newline in frontmatter and properties JSON) while the test stayed green. Fix pattern: compute subprocess-derived expectations in the test with a separate direct invocation (exec.Command plus its own trimming) or a pinned fixture, so the oracle stays independent of the helper's transformation logic. The same PR's TestReIngestLearnings_RoundTripFromStoreLearning documents this principle in its comment: per-side tests with shared expectations pass where an independent comparison fails.
