---
tag: test-quality
author: jay-flowers
category: gotcha
created_at: 2026-11-07T16:34:17Z
identity: test-quality-20261107T163417-jay-flowers
tier: draft
---

Fill-to-limit test gap (dewey PR #157, vault/search_index.go): a two-pass search that caps its secondary pass at limit-len(primaryResults) is untested unless a test makes secondary-pass candidates strictly exceed the remaining slots. Tests that run the secondary pass with slack capacity (limit far above the match count) or that early-exit before it (primary results fill the limit) survive mutations that drop or miscompute the cap. When reviewing or writing tests for "append X up to the limit" clauses: (1) build a fixture where secondary candidates > limit - primary results and assert the exact total; (2) never assert which nondeterministic candidate fills the final slot when fixtures iterate maps — posting-list order randomizes it; (3) grep adjacent entry points (e.g., a FullTextSearch merge test) before claiming "no test binds this" — a merge-cap test can look like coverage while routing extra matches through a different path.
