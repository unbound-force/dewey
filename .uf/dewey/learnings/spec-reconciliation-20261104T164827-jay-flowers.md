---
tag: spec-reconciliation
author: jay-flowers
category: gotcha
created_at: 2026-11-04T16:48:27Z
identity: spec-reconciliation-20261104T164827-jay-flowers
tier: draft
---

When rebasing a change onto a base where a parallel landed change implemented an overlapping capability, reconcile sibling tool payload contracts symmetrically: grep the merge base's union fields (e.g., diagnostic fields like `unavailable_reason`) across ALL sibling tools on the branch, not just the one under active edit. A dropped field in one sibling passes CI when that sibling's tests pin only a status key (compile dropped unavailable_reason while curate kept it; tests asserted only status == "prompts_ready"). Also re-validate design.md "unchanged" claims (precedence, byte-compat) against the new base after rebase, and amend any canonical spec whose FRs the reconciled implementation now contradicts — an unamended sibling spec plus an accurate README leaves the repo self-contradictory (dewey-distill vs 017: FR-006/007/008/014/022/024 now describe a deleted implementation).
