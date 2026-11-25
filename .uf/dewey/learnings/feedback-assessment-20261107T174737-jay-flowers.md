---
tag: feedback-assessment
author: jay-flowers
category: gotcha
created_at: 2026-11-07T17:47:37Z
identity: feedback-assessment-20261107T174737-jay-flowers
tier: draft
---

Verify feedback items against the code at head before accepting them — feedback narratives can invert implementation facts. Round-2 feedback on dewey PR #156 claimed the evidence fixture of 51 identical entries was weak because normalizeEvidence "deduplicates first, then enforces the cap" and "51 distinct entries must be truncated to 50". At head, the cap check is the function's first statement (raw input length, pre-dedupe), the function returns an error rather than truncating, and AGENTS.md's "capped at 512 characters" is the per-entry length limit, not the entry-count bound. An all-identical over-bound fixture is not automatically weak: for a raw-count (pre-dedupe) bound it is the discriminating input — 51 identical entries error only under raw-count counting, while 51 distinct entries error under either ordering — so "fixing" it to distinct entries can silently reduce the test's sensitivity to cap/dedup ordering. When a fixture's intent is non-obvious, prefer a clarifying comment over changing the fixture.
