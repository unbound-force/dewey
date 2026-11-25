---
tag: concurrency
author: jay-flowers
category: gotcha
created_at: 2026-11-10T23:27:28Z
identity: concurrency-20261110T232728-jay-flowers
tier: draft
---

When a design cites an in-tree SQL precedent for a new predicate, verify the precedent's guards actually transfer. Dewey's GetPagesWithProperty excludes `properties IS NOT NULL AND properties != ''` before json_extract because SQLite raises a malformed-JSON error on empty-string input (modernc.org/sqlite matches stock SQLite; NULL input propagates to NULL, but '' is not valid JSON). A supersession CAS predicate citing that precedent claimed `properties = ''` passes a COALESCE guard as unclaimed — it errors and fails closed instead, making the specified test case unpassable and contradicting the design's own "malformed JSON errors" sentence. Rule: any json_extract predicate claiming empty-string tolerance must add an explicit `OR properties = ''` arm (mirroring the in-tree guard) or reclassify the case as fail-closed in both the design and the test matrix.
