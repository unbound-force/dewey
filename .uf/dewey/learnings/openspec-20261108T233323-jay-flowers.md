---
tag: openspec
author: jay-flowers
category: pattern
created_at: 2026-11-08T23:33:23Z
identity: openspec-20261108T233323-jay-flowers
tier: draft
---

In the dewey repo, an OpenSpec spec delta must carry all three delta section headings — `## ADDED Requirements`, `## MODIFIED Requirements`, and `## REMOVED Requirements` — using `_None._` placeholders when a section is empty (see openspec/changes/learning-temporal-validity and search-compound-terms for the precedent). A change that alters the behavior of an existing tool (e.g., the default ordering of semantic_search in the validity-aware-retrieval change) files that requirement under MODIFIED with a Previous/New split rather than ADDED (PR #160 review, correlation 10f81d8d).
