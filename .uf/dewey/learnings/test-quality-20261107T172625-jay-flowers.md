---
tag: test-quality
author: jay-flowers
category: gotcha
created_at: 2026-11-07T17:26:25Z
identity: test-quality-20261107T172625-jay-flowers
tier: draft
---

Shared test oracle trap: when a test computes its expected value through the same helper the code under test calls, regressions inside that helper move both sides of the assertion equally and the test cannot fail. In dewey PR #156, TestProductionAsOfResolver derived its expected HEAD SHA from gitOutput — the exact helper productionAsOfResolver uses internally — so a TrimSpace regression would have silently corrupted every as_of pin (repo@sha\n written into learning frontmatter) while the suite stayed green, and no dedicated gitOutput test existed to catch it. Fix pattern: derive expected values through an independent path in the test (direct exec.Command plus the test's own trimming) so the oracle shares no code with the code under test.
