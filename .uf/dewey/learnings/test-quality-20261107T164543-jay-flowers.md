---
tag: test-quality
author: jay-flowers
category: gotcha
created_at: 2026-11-07T16:45:43Z
identity: test-quality-20261107T164543-jay-flowers
tier: draft
---

An ordered lookup chain is only regression-protected when a fixture makes two tiers of the chain compete for the same requested key. A table-driven test that exercises each tier in isolation — bare name → prefix hit, canonical name → exact hit — still passes when the chain is reordered, because every row has exactly one page that can satisfy it. To lock a resolution order, add one coexistence fixture per adjacent pair: store pages under both keys and assert the winner (e.g., a plain store page "plain" alongside "compiled/plain", asserting the exact page wins). Found on PR #163's findStorePage (exact → compiled/ → learning/): the "both" row pinned compiled-over-learning, but exact-over-prefix had no competing fixture anywhere in vault/.
