---
tag: search-index
author: jay-flowers
category: gotcha
created_at: 2026-11-07T15:56:43Z
identity: search-index-20261107T155643-jay-flowers
tier: draft
---

Dewey's in-memory search index (vault/search_index.go, search-compound-terms change for issue #149) indexes compound terms (containing - or _) both whole AND as their components of >= 2 characters. Two behavioral consequences to remember when reviewing search results or writing search tests: (1) single common-word queries (e.g., "review", "01") now surface blocks whose only occurrence of that word is inside a hyphenated ID like KB-REVIEW-ADJUD-01 — component indexing broadens recall for plain word queries t
