---
tag: sqlite-nocase
author: jay-flowers
category: gotcha
created_at: 2026-11-11T21:46:10Z
identity: sqlite-nocase-20261111T214610-jay-flowers
tier: draft
---

A NOCASE name-range query (e.g. `name >= 'learning/' COLLATE NOCASE AND name < 'learning0' COLLATE NOCASE`) admits mixed-case namespaces while downstream helpers using `strings.TrimPrefix`/`strings.HasPrefix` remain case-sensitive. The mismatch silently produces wrong identity/prefix values and hides pages from prefix-based filters. Test the case-insensitive admission end-to-end through the consumer, not only at the store-query layer, and use case-insensitive strip/match helpers (or make the query BINARY) so the layers agree.
