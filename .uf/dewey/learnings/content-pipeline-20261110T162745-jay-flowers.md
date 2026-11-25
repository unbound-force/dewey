---
tag: content-pipeline
author: jay-flowers
category: pattern
created_at: 2026-11-10T16:27:45Z
identity: content-pipeline-20261110T162745-jay-flowers
tier: draft
---

Re-review rounds must verify disposition issues by reading their bodies across repositories, not by confirming existence from local review state. On dewey PR #167 round 2, the round-1 summary said the heredoc fix was "filed upstream as unbound-force/unbound-force#1011"; only reading #1011's body revealed that it explicitly enumerates BOTH files' delimiters (TITLE_EOF/BODY_EOF/REPLY_EOF in uf.address-feedback.md AND COMMENT_EOF/CHILD_EOF in uf.triage-issue.md) — establishing that residual heredoc sites in triage-issue.md were already tracked rather than untracked gaps needing new findings. Disposition references that point into another repository's tracker cannot be verified from the local review state: run gh issue view against the cross-repo issue and read its scope enumeration before classifying a residual defect site as covered or uncovered. This prevents both duplicate findings on tracked items (the review-churn pathology scaffold refreshes exist to eliminate) and false coverage claims (missed gaps).
