---
tag: test-quality
author: jay-flowers
category: gotcha
created_at: 2026-11-07T17:47:43Z
identity: test-quality-20261107T174743-jay-flowers
tier: draft
---

When review feedback claims a boundary fixture "does not exercise the bound", verify the implementation's check ordering before accepting. On dewey PR #156, round-2 feedback asserted normalizeEvidence dedups evidence before enforcing the 50-entry cap, making a 51-entry all-identical fixture look unexercised; the code actually checks len(raw) > maxEvidenceEntries as its first statement, before trim/dedup. Consequences: (1) the all-identical over-limit fixture does hit the count check, which is the only error source for that input, so the assertion locks the bound; (2) the fixture is ordering-sensitive — reordering the cap after dedup collapses 51 duplicates to 1 and fails the test — which is desirable when the specified contract is the raw-input bound ("evidence MUST hold at most 50 entries"); (3) the feedback's proposed distinct-value fixture passes under both orderings and would silently decouple the test from the check ordering. General rule: for count bounds enforced on raw input before dedup, all-duplicate over-limit fixtures pin pre-dedup semantics; distinct-value fixtures are ordering-agnostic.
