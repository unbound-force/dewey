---
tag: test-coverage
author: jay-flowers
category: pattern
created_at: 2026-11-05T16:25:48Z
identity: test-coverage-20261105T162548-jay-flowers
tier: draft
---

A unit test on a helper's return value does not cover the caller's wiring of that value into the response contract. On dewey PR #156, linkSuperseded's warning strings were unit-tested (TestLinkSuperseded_Warnings), but the StoreLearning branch that copies the warning into the MCP result (tools/learning.go:341-343, `result["supersedes_warning"]`) had zero coverage — dropping the branch or renaming the key kept the full suite green. When a tool response carries a partial-failure or warning field, force the partial failure through the public tool entry point and assert the field's presence and content at the response boundary. Cheap forcing lever in dewey's tools package: corrupt the target learning's markdown backup (write content without YAML frontmatter) so the file leg of the supersession back-link fails while the store leg succeeds.
