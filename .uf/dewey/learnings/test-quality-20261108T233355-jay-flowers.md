---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-08T23:33:55Z
identity: test-quality-20261108T233355-jay-flowers
tier: draft
---

Over-fetch + re-rank pipelines need a displacement assertion, not just an ordering assertion. Query with a limit smaller than the matching result set and assert that a lower-raw-similarity, higher-weighted result displaces the top raw match: in dewey PR #160 (validity retrieval), tools/semantic_validity_test.go's TestSemanticSearch_ValidityRanking searches with Limit:1 against a fixture whose raw-nearest page is superseded (raw 1.0 vs 0.9487); only fetching max(3xlimit, 30) before ranking lets the weighted replacement displace it — assert that displacement explicitly.
