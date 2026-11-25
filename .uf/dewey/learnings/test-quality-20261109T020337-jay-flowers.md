---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-09T02:03:37Z
identity: test-quality-20261109T020337-jay-flowers
tier: draft
---

A count-based test assertion is non-discriminating for an aggregate-level cardinality contract when the fixture carries exactly one member of the aggregated collection: with one member, "one result per aggregate" and "one result per member" produce identical totals. To pin an aggregate-level cardinality against a plausible per-member regression, give the fixture at least two members so the two implementations produce different counts. Concrete case: dewey's lint anchor checks return one anchor_missing finding per learning when the as_of pin commit is unknown (spec-mandated), but the original lost-pin test fixture cited a single evidence path, so a per-anchor regression would have passed the count assertions undetected.
