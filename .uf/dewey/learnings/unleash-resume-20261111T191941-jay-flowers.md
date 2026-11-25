---
tag: unleash-resume
author: jay-flowers
category: pattern
created_at: 2026-11-11T19:19:41Z
identity: unleash-resume-20261111T191941-jay-flowers
tier: draft
---

When resuming an interrupted /uf.unleash run that died mid-review-fix-loop, durable artifacts reconstruct the exact loop position without trusting compression summaries: get_progress's completed_steps and phase inventories, parse_tasks checkbox counts, and the dispatch artifacts under .uf/artifacts/dispatch/ (per-correlation_id verdicts and findings) reveal which review iteration the loop died in and what the pending findings were. Uncommitted working-tree changes from the interrupted fix are reconciled by mapping them against the last dispatch's advisory list, verifying coherence (the expected guard messages exist in committed code), running the full CI-parity gate locally, and committing them as a fix commit before re-dispatching the council. Applied on branch opsx/curator-tool-allowlist (2026-11-11): the resume re-anchored on implement-phase-4 + 13/14 tasks, matched the uncommitted cli_test.go changes to the four advisories from dispatch 9e924343, verified all 17 packages green with -race -count=1, committed as ff087bb, and re-dispatched to a terminal APPROVE WITH ADVISORIES.
