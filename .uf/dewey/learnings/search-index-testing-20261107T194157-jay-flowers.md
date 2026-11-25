---
tag: search-index-testing
author: jay-flowers
category: pattern
created_at: 2026-11-07T19:41:57Z
identity: search-index-testing-20261107T194157-jay-flowers
tier: draft
---

Vault search-index tests that assert result order must control posting-list order: BuildFrom iterates a Go map, so cross-page result order is randomized between runs. Build fixtures as a single page with an ordered block slice (see compoundIndex in vault/search_index_test.go) for positional assertions, or assert membership by counting hits across an allowed page set (switch on PageName) instead of asserting positions. Multi-page positional assertions on search results will flake under go test -race -count=1.
