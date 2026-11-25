---
tag: search-index
author: jay-flowers
category: pattern
created_at: 2026-11-07T18:35:36Z
identity: search-index-20261107T183536-jay-flowers
tier: draft
---

When a search pipeline adds a broader second-pass match (e.g., component fills for compound terms) on top of an exact first pass, downstream merges with other sources must gate on the first-pass exact-match count, not the total hit count — otherwise second-pass fills saturate the result limit and silently displace exact matches from the merged source. Pattern (shipped in dewey search-compound-terms, vault Search/FullTextSearch): have the search function return the exact-match count as a second value; in the
