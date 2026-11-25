---
tag: resource-exhaustion
author: jay-flowers
category: pattern
created_at: 2026-11-09T01:14:09Z
identity: resource-exhaustion-20261109T011409-jay-flowers
tier: draft
---

Per-call subprocess timeouts do not bound aggregate work. Pattern for subprocess loops whose iteration count comes from untrusted data (dewey lint anchor checks, PR 159 item 6): (a) derive each call's timeout from the caller-propagated context — context.WithTimeout(ctx, perCall) instead of context.Background() — so cancellation and deadlines reach every spawn; (b) cap the per-unit iteration count at the data contract (e.g., 50 anchors per learning), re-validated at the consumption site since writer-side caps do not survive file round-trips; (c) wrap the whole phase in an aggregate budget that degrades gracefully (partial findings + warn log). A per-unit cap alone is insufficient — an attacker who can forge one oversized unit can forge many units (pages) just as easily, so the aggregate bound is load-bearing, not redundant.
