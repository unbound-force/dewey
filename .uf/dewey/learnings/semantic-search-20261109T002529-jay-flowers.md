---
tag: semantic-search
author: jay-flowers
category: pattern
created_at: 2026-11-09T00:25:29Z
identity: semantic-search-20261109T002529-jay-flowers
tier: draft
---

In dewey's tools layer, block-level semantic results share pages: toSemanticResults (tools/semantic.go) builds pageCache in a pre-pass specifically so per-page work runs once per unique page name ("to avoid N+1 lookups"). New per-page enrichment — e.g. decoding validity/as_of from page.Properties — must happen at cache-population time (widen the cache value to a {page, validity, asOf} tuple or keep a parallel map), not inside the per-result loop. Over-fetch (semanticFetchLimit = max(3×limit, 30)) multiplies result rows, so a per-row json.Unmarshal re-decodes the same Properties JSON up to 30 times per default query while the unique-page count stays small.
