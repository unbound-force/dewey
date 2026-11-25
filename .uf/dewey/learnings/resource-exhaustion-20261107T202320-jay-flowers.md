---
tag: resource-exhaustion
author: jay-flowers
category: gotcha
created_at: 2026-11-07T20:23:20Z
identity: resource-exhaustion-20261107T202320-jay-flowers
tier: draft
---

When refactoring a merge/append loop into a pre-allocated slice, derive the capacity hint from the actual data sizes (the lengths of the inputs being merged), never from a caller-supplied limit/page-size parameter. `make([]T, 0, limit)` with an unvalidated request limit converts a benign large-limit request — which previously just returned more real rows via organic append growth — into a single-call fatal OOM, because the allocation becomes proportional to the requested value rather than the data. In dewey
