---
tag: search-index
author: jay-flowers
category: pattern
created_at: 2026-11-07T15:56:44Z
identity: search-index-20261107T155644-jay-flowers
tier: draft
---

Two-pass exact-first pattern for extending search semantics without breaking inherited contracts (shipped in dewey vault/search_index.go, issue #149): keep the existing exact-term AND query as pass 1, byte-for-byte unchanged; run the broadened query (compound terms expanded to components) as pass 2 only when the query actually contains an expandable compound AND pass 1 returned fewer than limit results; dedupe pass 2 against pass-1 UUIDs via a skip map consulted inside the shared match loop, and cap pass 2 
