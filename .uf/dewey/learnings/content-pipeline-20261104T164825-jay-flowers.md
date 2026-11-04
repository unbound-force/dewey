---
tag: content-pipeline
author: jay-flowers
category: gotcha
created_at: 2026-11-04T16:48:25Z
identity: content-pipeline-20261104T164825-jay-flowers
tier: draft
---

Content-pipeline (issue-filing) findings are structurally fragile across review-fix cycles because their fixes live outside the diff. A change can fix every code finding, rebase, and still leave the three-issue unbound-force/website convention (docs sync + blog + tutorial, established by dewey-publish tasks 7.3+7.8) unimplemented: dewey-distill PR #145's tasks.md section 7 modeled only the docs-sync issue through round 2, the rebase onto main-with-017, and into the next review round, while a same-day learning (content-pipeline-20261104T005641) recorded the omission. Mitigations: (1) documentation tasks in tasks.md should record the filed issue numbers, so the next review round verifies via `gh issue view` instead of re-deriving scope from the task text; (2) each review round should re-diff the content-pipeline artifacts against the convention rather than trusting an "all findings fixed" summary, which tracks only in-diff code changes.
