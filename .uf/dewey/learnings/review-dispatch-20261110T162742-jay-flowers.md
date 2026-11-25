---
tag: review-dispatch
author: jay-flowers
category: pattern
created_at: 2026-11-10T16:27:42Z
identity: review-dispatch-20261110T162742-jay-flowers
tier: draft
---

For uf-init scaffold-refresh PRs, the highest-signal architectural check is cross-surface coherence between command prose and plugin code: every output shape a refreshed command describes must match the TypeScript constants and renderers that produce it. In dewey PR #167 this meant verifying that the dispatch summary prose (one-line format, 512-char error suffix, retryable marker omitted when unknown, pre-merge proposals count) matches dispatchSummaryLine; that preview caps (10 proposals, 512-char information) match buildCompactFinalization; that the command warning-kind allowlists match projectSummaryWarning's switch cases (duplicate_proposal_merged, proposals_overflow); that push-tool prose ({branch, expected_sha, BRANCH_MISMATCH, EXPECTED_SHA_UNKNOWN, --force-if-includes lease) matches the rewritten git_push/git_push_force_with_lease schemas; and that timeout recovery prose (kind/pending_question/resume) matches TimeoutDetails and readDelegateResumeState. Drift in any of these pairs breaks commands at runtime even when each file is individually well-formed, and byte-identity with upstream makes local prose-only fixes divergence debt.
