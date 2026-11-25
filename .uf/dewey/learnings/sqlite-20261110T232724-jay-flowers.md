---
tag: sqlite
author: jay-flowers
category: gotcha
created_at: 2026-11-10T23:27:24Z
identity: sqlite-20261110T232724-jay-flowers
tier: draft
---

SQLite's json_extract raises a malformed-JSON error on an empty-string document; COALESCE around json_extract neutralizes NULL results, not raised errors. A guard like COALESCE(json_extract(col, '$.key'), '') = '' therefore misclassifies empty-string rows as claim errors instead of unclaimed values. Any CAS or query predicate that must treat an empty-string JSON column as unclaimed needs an explicit `col IS NULL OR col = ''` disjunct before json_extract runs — the pattern dewey's GetPagesWithProperty already uses (store.go: WHERE properties IS NOT NULL AND properties != '' AND json_extract(...) IS NOT NULL, commented "Pages with NULL or empty-string properties are excluded"). Spec-level corollary: when a design claims a SQL predicate "matches" a Go decoder's semantics (decodeLearningProperties returns an empty map for raw == ""), verify against real SQLite behavior — the decoder forgives empty strings, json_extract does not.
