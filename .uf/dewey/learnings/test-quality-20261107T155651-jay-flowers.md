---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-07T15:56:51Z
identity: test-quality-20261107T155651-jay-flowers
tier: draft
---

When a search query path adds a second fill pass with its own limit arithmetic (dewey vault/search_index.go: matchAllLocked(expanded, limit-len(results), seen)), a test where exact matches fill the limit only covers the early-return branch — the fill pass never runs. The cap needs a separate test where available fill matches exceed the remaining slots (e.g., 1 exact + 3 component-matching blocks with limit 2, asserting exactly 2 results, exact first). Without it, removing or miscomputing the cap cannot fail
