---
tag: sqlite
author: jay-flowers
category: gotcha
created_at: 2026-11-10T23:27:36Z
identity: sqlite-20261110T232736-jay-flowers
tier: draft
---

SQLite json_extract raises a malformed-JSON error on an empty-string or otherwise invalid JSON argument, so COALESCE(json_extract(col, '$.key'), '') only neutralizes NULL results (NULL column, missing key, JSON null) — an empty-string column value errors out of the predicate instead of passing it as "unclaimed". Dewey's own GetPagesWithProperty (store/store.go) filters `properties IS NOT NULL AND properties != ''` before json_extract for exactly this reason. Compare-and-swap predicates over JSON property blobs must either pre-filter empty strings in the WHERE clause or deliberately accept the fail-closed error path — and the design's guard taxonomy must not list '' alongside NULL/missing-key as COALESCE-absorbed (serialize-supersession-writes design D1 self-contradicted on this point).
