---
tag: search-merge
author: jay-flowers
category: pattern
created_at: 2026-11-07T18:35:33Z
identity: search-merge-20261107T183533-jay-flowers
tier: draft
---

When an in-memory search index gains a second, lower-precision pass (e.g., component matching for compound terms), downstream merges with other result sources must gate and interleave on the pass-one exact-match count, not the total hit count. Gating on total hits lets fill results saturate the limit and silently displace higher-precision hits from merged sources: in dewey, pass-two component fills could crowd out a store-only page holding the full compound ID from vault.FullTextSearch's store merge (PR #15
