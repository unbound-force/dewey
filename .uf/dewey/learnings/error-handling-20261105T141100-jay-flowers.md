---
tag: error-handling
author: jay-flowers
category: gotcha
created_at: 2026-11-05T14:11:00Z
identity: error-handling-20261105T141100-jay-flowers
tier: draft
---

In Go, silently discarding json.Unmarshal's error before a read-modify-write is destructive in two ways: (1) unmarshaling the literal JSON `null` into a map variable sets it to nil, so a subsequent `m[k] = v` panics with "assignment to entry in nil map"; (2) an unparseable payload leaves the map empty, so writing the reconstructed map back replaces the stored document wholesale (e.g., a SQLite UPDATE that sets the whole column), silently destroying the original fields. On any read-modify-write path, treat a
