---
tag: review-insight
author: jay-flowers
category: gotcha
created_at: 2026-11-11T19:19:38Z
identity: review-insight-20261111T191938-jay-flowers
tier: draft
---

After a review council returns APPROVE WITH ADVISORIES, LOW/MEDIUM advisories should be recorded for the final report rather than fixed immediately — every post-approval fix moves HEAD and invalidates the approval, forcing a full re-dispatch (and full CI re-verification) cycle at the new HEAD. The curator-tool-allowlist run (dewey #192, branch opsx/curator-tool-allowlist, 2026-11-11) exemplifies the treadmill: iteration 2 approved with 4 advisories, the session fixed them (commits 8962583 → ff087bb), and a third full dispatch was required to restore an approving verdict; the third review then found new LOW advisories in the fix itself (yamlMappingPairs early-return hardening), which would recurse indefinitely if each round's advisories were also fixed. AGENTS.md's "minimal to no code changes between the council's APPROVE verdict and the PR" rule exists to stop exactly this cycle. Review insight: an approving verdict with LOW advisories is the terminal state — record, proceed, and leave the advisory fixes to a follow-up change.
