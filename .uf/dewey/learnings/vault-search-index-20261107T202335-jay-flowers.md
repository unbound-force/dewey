---
tag: vault-search-index
author: jay-flowers
category: gotcha
created_at: 2026-11-07T20:23:35Z
identity: vault-search-index-20261107T202335-jay-flowers
tier: draft
---

When a search pipeline adds a lower-precision fallback pass (e.g., compound-term component fills) after an exact pass, the store-merge gate must key on the exact-pass match count, not the total hit count — otherwise fallback hits saturate the limit and silently displace store-only pages holding the full ID. The regression test must construct enough fallback-only hits to saturate the limit with zero exact matches; without that limit pressure the displacement bug is invisible to tests.
