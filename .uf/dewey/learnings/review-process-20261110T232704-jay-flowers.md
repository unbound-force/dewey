---
tag: review-process
author: jay-flowers
category: gotcha
created_at: 2026-11-10T23:27:04Z
identity: review-process-20261110T232704-jay-flowers
tier: draft
---

For documentation-only changes in dewey, the review council's accuracy-focused personas (Guard, Curator, Architect, Adversary) keep producing new LOW findings on each re-review because rewording the note changes what can be critiqued. Each iteration found a real but progressively smaller defect (over-broad umbrella claim, then a wrong "first record" claim, then a wrong origin attribution from a sibling spec, then LOW polish). Stop when the verdict is APPROVE WITH ADVISORIES and all remaining findings are LOW polish or explicitly out of the issue's scope: AGENTS.md requires minimal to no changes between the council's APPROVE and PR submission, so continuing to edit after approval would invalidate the approval. Record the residual LOW advisories in the final report and carry them as follow-ups instead of re-dispatching. Also note that child delegation can fail with a retryable provider connection error (models.corp.redhat.com unreachable); after one retry, applying a consented documentation fix in the parent is the pragmatic path and the council re-reviews the resulting bytes.
