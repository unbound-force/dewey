---
tag: test-quality
author: jay-flowers
category: gotcha
created_at: 2026-11-05T16:25:39Z
identity: test-quality-20261105T162539-jay-flowers
tier: draft
---

A response-contract field populated only on a partial-failure branch (e.g., a helper's warning string mapped to a result key like `supersedes_warning`) is mutation-survivable when tests assert only its absence on the success path and unit-test the helper in isolation: renaming or dropping the result key passes the entire suite. Every response-contract field needs at least one handler-level test that forces the branch producing it and asserts the field's presence and value — a `resultString`-style accessor that fatals on a missing key kills the rename/drop mutation directly. In dewey PR #156, tools/learning.go's `supersedes_warning` wiring (learning.go:341-343) had exactly this gap: TestStoreLearning_SupersedesLinksBothWays asserted absence only, and TestLinkSuperseded_Warnings bypassed the wiring.
