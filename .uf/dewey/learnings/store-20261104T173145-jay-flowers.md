---
tag: store
author: jay-flowers
category: pattern
created_at: 2026-11-04T17:31:45Z
identity: store-20261104T173145-jay-flowers
tier: draft
---

Hoisting production SQL statements to package-level constants lets EXPLAIN QUERY PLAN tests exercise the exact statements the code runs, so test copies cannot drift from shipped queries. dewey PR 145 applied this in store/store.go for ListPagesByNameFoldCase, ListKnowledgePagesFoldCase, and GetPageBySourceDoc: each statement is a named constant, and the store tests pin each query to an index SEARCH on idx_pages_name_nocase / idx_pages_source_doc_nocase rather than a full-table SCAN — catching both drift and accidental index-bypassing rewrites at CI time.
