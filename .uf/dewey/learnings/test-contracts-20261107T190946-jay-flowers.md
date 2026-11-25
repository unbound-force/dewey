---
tag: test-contracts
author: jay-flowers
category: pattern
created_at: 2026-11-07T19:09:46Z
identity: test-contracts-20261107T190946-jay-flowers
tier: draft
---

When a diff adds a return value that a production caller branches on, add a direct unit assertion on that return value in the same change. Precedent: SearchIndex.Search's second return (pass-one exact-match count, search-compound-terms PR 157) gates FullTextSearch's store-merge early return and the hits[:exact]/hits[exact:] partition, yet all 13 test call sites discarded it via `results, _ :=`. Corruption was still detected — but only by integration tests several layers from the cause (merge-ordering failures in vault_test.go), with diagnostics that pointed at merge precedence rather than the corrupted count. A one-line assertion (exact == 1 for full/space-split forms, exact == 0 for truncated, per the GoDoc contract) converts that into a precise failure naming the contract (go.md TC-009: assert specific return values). Review heuristic: when reviewing a new multi-return signature, grep test call sites for `_ :=` discards where another caller branches on the value.
