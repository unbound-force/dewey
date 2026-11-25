---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-10T19:10:02Z
identity: test-quality-20261110T191002-jay-flowers
tier: draft
---

When a shared post-processing helper (filter/rank/trim, e.g. finishSemanticResults) is wired into multiple MCP tools, each tool's wiring needs its own discriminating integration test — coverage of the helper plus one sibling tool is not enough. Two patterns from dewey PR #160 (validity-aware retrieval): (1) Leakage detection — to pin an enrichment-only tool that shares a converter with ranking tools, build a fixture whose raw order differs from its weighted order (e.g. stale-unverified page with the highest raw similarity): tools/semantic_validity_test.go's TestSimilar_ValidityEnrichment fails the moment validity ranking leaks into `similar`, because weighted order (plain > new > stale) differs from the asserted raw order (stale > plain > new). (2) Per-tool displacement — for over-fetch wiring (fetch = max(3×limit, 30) when ranking/filtering is active), each tool needs a fixture whose matching-result count sits between the plain limit and the over-fetched limit, with the raw-nearest match down-weighted so it must be displaced: semantic_search is discriminated via Limit:1, but semantic_search_filtered's distinct boolean wiring has no such case (all filtered tests use ≤5 results with limit 10), so a constant-false over-fetch condition there would pass the whole suite.
