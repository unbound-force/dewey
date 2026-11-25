---
tag: semantic-retrieval
author: jay-flowers
category: pattern
created_at: 2026-11-09T00:25:31Z
identity: semantic-retrieval-20261109T002531-jay-flowers
tier: draft
---

Per-result enrichment loops that follow a per-unique-entity cache pattern must decode derived fields at cache-population time, not once per result row. In dewey's toSemanticResults (tools/semantic.go), the pageCache is built per unique page name "to avoid N+1 lookups", but the validity-aware enrichment (issue #148, PR #160) initially called pageValidity(page) — which json.Unmarshals page.Properties — inside the per-result loop. Block-level search results share pages, and over-fetch multipliers (semanticFetchLimit = max(3×limit, 30)) amplify row counts, so per-row decoding silently reintroduces the N+1 work the cache exists to prevent. Fix pattern: cache a {entity, derived-fields} tuple (e.g. {page, validity, asOf}) keyed by the unique entity name and populate derived fields once at cache insertion; existing end-to-end enrichment tests then verify the refactor is behavior-preserving.
