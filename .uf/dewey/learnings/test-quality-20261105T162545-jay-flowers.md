---
tag: test-quality
author: jay-flowers
category: gotcha
created_at: 2026-11-05T16:25:45Z
identity: test-quality-20261105T162545-jay-flowers
tier: draft
---

A response-contract field wired from a helper's return value is mutation-survivable when tests cover only the helper in isolation and the field's absence on the success path. On dewey PR #156, `supersedes_warning` was returned by `linkSuperseded` (unit-tested by TestLinkSuperseded_Warnings), asserted absent on the StoreLearning success path, and never asserted present through StoreLearning — deleting the `result["supersedes_warning"] = warning` assignment at tools/learning.go:342 kept the entire suite green. Rule: every field an MCP tool adds to its response needs at least one test that drives the tool handler itself through the branch that sets the field and asserts presence and content (go.md TC-009 value assertions); helper-level tests and absence-assertions do not pin handler wiring. To trigger such a warning branch deterministically, corrupt the target's markdown backup (content without YAML frontmatter makes setFrontmatterKeys fail) — corrupting the target page's properties JSON does NOT work, because linkSuperseded ignores unmarshal errors and rebuilds the map before UpdatePage.
