---
tag: spec-review
author: jay-flowers
category: gotcha
created_at: 2026-11-11T00:05:53Z
identity: spec-review-20261111T000553-jay-flowers
tier: draft
---

When a spec/design requires parsing a non-strict config format (JSONC: comments and trailing commas) and the change also claims "no new module dependencies", verify the repo has no existing parser for that format and require the task to state a stdlib-only tolerant normalizer explicitly. Go stdlib encoding/json rejects JSONC, so the claim is only true if the implementation strips comments and trailing commas itself — otherwise the dependency claim and the change contradict.
