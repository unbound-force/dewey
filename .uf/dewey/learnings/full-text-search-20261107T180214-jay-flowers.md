---
tag: full-text-search
author: jay-flowers
category: pattern
created_at: 2026-11-07T18:02:14Z
identity: full-text-search-20261107T180214-jay-flowers
tier: draft
---

Structure a fallback path so the inactive path is provably identical to the old behavior: return the exact count equal to total hits when the expansion is inactive, audit every downstream consumer under both the active and inactive case, and assert single-pass identity in tests. Dewey's compound-term search keeps no-compound queries byte-for-byte identical to the pre-change single-pass behavior, which is why the 37 inherited tool semantics held (review-council on PR unbound-force/dewey#157, guard finding).
