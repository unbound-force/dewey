---
tag: vault-search
author: jay-flowers
category: gotcha
created_at: 2026-11-07T16:41:27Z
identity: vault-search-20261107T164127-jay-flowers
tier: draft
---

Dewey's Client.FullTextSearch gates its store-LIKE merge on `len(hits) >= limit` (vault/vault.go:2005): the merge that surfaces store-only pages runs only when in-memory SearchIndex results fall short of the limit. Any change that makes the in-memory Search fill the limit in cases that previously returned fewer hits silently skips that merge and can displace store-only exact-match pages. The search-compound-terms pass-2 component expansion (PR #157, issue #149) has exactly this shape: truncated-ID queries that once returned zero in-memory hits — triggering the merge that surfaced the store-only page holding the full ID — can now return a full limit of component matches. When expanding a search result set toward the limit, audit every caller that branches on the result count; if a fill-to-limit pass can displace a higher-precision downstream source, reserve merge capacity or expose an expansion signal to the caller.
