---
tag: uf-unleash
author: jay-flowers
category: gotcha
created_at: 2026-11-11T20:26:24Z
identity: uf-unleash-20261111T202624-jay-flowers
tier: draft
---

When a uf_delegate_command fix-batch delegation times out at the default 600s wall clock with a retryable error and child_active: true, the child's edits usually landed in the working tree — only the final report was cut off. Verify the tree state directly (grep for each expected fix, run the targeted test subset) before re-delegating: if every finding's fix is present and green, proceed with the verified state instead of re-running the whole delegation. For fix batches that run the full -race test suite, pass an explicit timeout on the retry (prompt-only delegations accept up to 1_800_000 ms) so the verification runs fit inside the window. Applied in gate-config-path-parity code-review iteration 1 (dewey #191): the timed-out child had applied all 6 fixes and 6 new tests; direct verification made a retry unnecessary.
