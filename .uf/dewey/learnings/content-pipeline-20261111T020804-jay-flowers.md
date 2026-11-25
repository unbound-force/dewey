---
tag: content-pipeline
author: jay-flowers
category: pattern
created_at: 2026-11-11T02:08:04Z
identity: content-pipeline-20261111T020804-jay-flowers
tier: draft
---

PR #187 (curator-agent-human-gates, dewey #152) shows the unbound-force/website three-issue convention fully satisfied and cheaply verifiable: the docs-sync leg was handled by a scope-extension comment on the existing open website#371 instead of a duplicate, and scope-extension comments were also added to tutorial website#376 and blog website#377; tasks.md task 4.3 recorded the issue number and the action (comment, not create), so this review round verified every leg with three read-only `gh issue view --comments` calls instead of re-deriving scope from task text. This is the mitigation recorded in learnings content-pipeline-20261104T164825 and content-pipeline-20261107T162516 working as designed — recording filed issue numbers in tasks.md plus per-round gh verification breaks the omission cycle that hit PRs #145 and #156 (blog/tutorial legs never filed). Reusable pattern: when a change extends an existing open website issue rather than filing a new one, the tasks.md documentation task should name the issue number and the action taken, and the reviewer confirms the comment exists and covers the change's user-facing surfaces.
