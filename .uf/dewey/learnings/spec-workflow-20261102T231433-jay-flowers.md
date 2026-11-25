---
tag: spec-workflow
author: jay-flowers
category: gotcha
created_at: 2026-11-02T23:14:33Z
identity: spec-workflow-20261102T231433-jay-flowers
tier: draft
---

When a review-hardening pass adds or changes a requirement late in the pipeline (e.g., spec 017's FR-024 ignored-api_key warning, added in the fix commit after council feedback), every artifact that restates the affected design decision must be re-amended in the same commit — not only the spec, contracts, code, and tests. plan.md design-decision paragraphs and research.md R-numbered decision text tend to be skipped because they are not executed or tested, producing an internal contradiction where the spec and code say one thing (warn-only parse of config.yaml api_key) while the plan/research decision sentences still say the opposite (no api_key field is parsed). Observed in dewey PR #144 (017-openai-compatible-providers): the implementation was correct and spec-aligned, but plan.md D2 and research.md R2 were stale. Checklist for late requirement amendments: grep the feature directory for the old decision wording and update plan.md/research.md decision paragraphs alongside the spec FR they now contradict.
