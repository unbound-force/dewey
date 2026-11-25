---
tag: review-dispatch
author: jay-flowers
category: gotcha
created_at: 2026-11-12T19:01:25Z
identity: review-dispatch-20261112T190125-jay-flowers
tier: draft
---

When a wall-clock backstop kills a /uf.unleash session mid code-review loop (dewey #150, opsx/explicit-supersession-in-compile, 2026-11-12), the stranded dispatch leaves NO finalized artifact under .uf/artifacts/dispatch/ — but if its findings were already harvested, fixed, and committed, the correct resume is NOT to reconstruct or consolidate the dead dispatch. It is a fresh re-dispatch of the full reviewer panel, which the council protocol requires after applied fixes anyway (re-run every included reviewer, not just failing or content-affected ones). Re-anchor first on get_progress + parse_tasks + git log before any dispatch work: the green phase verdicts and the fix commit (e85e787 "fix: address review council findings — iteration 1") are the durable record of what iteration 1 addressed; the fix commit message doubles as the findings record when the dispatch artifact is missing. Marking the review-council task (9.9) complete on the approving re-dispatch, while leaving the checkbox edit uncommitted, keeps HEAD equal to the dispatch artifact's recorded head SHA so resume detection stays consistent and /uf.finale's stage sweep carries the edit.
