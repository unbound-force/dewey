---
tag: performance
author: jay-flowers
category: pattern
created_at: 2026-11-09T00:25:27Z
identity: performance-20261109T002527-jay-flowers
tier: draft
---

In per-result enrichment loops that already maintain a per-unique-page cache (dewey's toSemanticResults builds pageCache 'to avoid N+1 queries'), derived per-page computations — such as decoding a page's Properties JSON — belong in the cache population step, not the per-result loop. A single helper call (pageValidity) placed in the row loop silently re-executes the same decode for every block hit from a page, up to the over-fetch ceiling (max(3×limit, 30)), undercutting the cache pattern the surrounding code documents. Cache the decoded tuple (page + validity + asOf) alongside the page so decode cost scales with unique pages; shared converters amplify the waste across every calling tool (semantic_search, semantic_search_filtered, similar).
