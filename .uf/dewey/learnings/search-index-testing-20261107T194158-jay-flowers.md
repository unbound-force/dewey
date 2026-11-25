---
tag: search-index-testing
author: jay-flowers
category: pattern
created_at: 2026-11-07T19:41:58Z
identity: search-index-testing-20261107T194158-jay-flowers
tier: draft
---

When a multi-pass search API returns an auxiliary count that gates downstream merges (SearchIndex.Search's pass-one exact-match count gating FullTextSearch's store merge), assert that count directly in unit tests (a wantExact table column) rather than discarding it with `results, _ :=`. Length-only assertions cannot distinguish pass-one exact matches from pass-two component fills, and the count is the contract the downstream merge gates on — a regression that inflates or deflates it changes merge precedence without changing result counts in most fixtures.
