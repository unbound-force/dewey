---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-10T15:33:23Z
identity: test-quality-20261110T153323-jay-flowers
tier: draft
---

When a spec mandates a time bound on an operation ("bounded in total time as well as per call"), ship the bound as an injectable parameter, not a package constant, or no test can pin it. In dewey's anchor checks (PR #159, tools/lint_anchors.go), anchorCheckBudget (60s) and anchorGitTimeout (5s) are constants; the only context-failure test uses an already-cancelled context, which would still pass if both context.WithTimeout calls were deleted. Making the budget a Lint field or checker-constructor option le
