---
tag: uf-unleash
author: jay-flowers
category: gotcha
created_at: 2026-11-11T20:51:32Z
identity: uf-unleash-20261111T205132-jay-flowers
tier: draft
---

Never issue multiple mark_task_complete calls in one parallel block: the tool does byte-slice surgery on a shared tasks.md without cross-call locking, so parallel flips lose updates (last write wins). Seen 2026-11-11 on branch opsx/dewey-distill-verify-step during /uf.unleash phase 5: 5.1 and 5.2 were marked in one parallel batch, 5.2's write clobbered 5.1's flip, and the bookkeeping commit showed 1 insertion/1 deletion instead of 2/2 — the lost checkbox was only caught by grepping tasks.md against the commit diff and had to be re-flipped in a recovery commit. Detection heuristic: a checkbox-flip chore commit whose insertion/deletion count is lower than the number of tasks marked means a parallel race. Rule: mark tasks complete sequentially, one call per message round.
