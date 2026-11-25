---
tag: search-index
author: jay-flowers
category: gotcha
created_at: 2026-11-07T16:52:50Z
identity: search-index-20261107T165250-jay-flowers
tier: draft
---

vault/search_index.go indexes every block through indexBlocksLocked — once per block, recursive over children, on every BuildFrom/ReindexPage — so per-block allocations compound across a vault. withCompoundComponents therefore copies lazily: it returns the caller's input slice unchanged when no term is compound and copies only when the first compound is found. Gotcha for future editors: the result may alias the input slice — the existing caller is safe only because it passes tokenize(b.Content)'s fresh per-call slice and its later appends (links/tags) touch no other holder of that backing array. Preserve the lazy copy when editing; an unconditional copy reintroduces one allocation per block in the indexing hot path. Note the scope split: openspec/changes/index-pipeline-performance covers embedding batching, source-fetch, and index concurrency — not search_index.go allocations.
