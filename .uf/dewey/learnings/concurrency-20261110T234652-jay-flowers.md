---
tag: concurrency
author: jay-flowers
category: gotcha
created_at: 2026-11-10T23:46:52Z
identity: concurrency-20261110T234652-jay-flowers
tier: draft
---

When a design adds a belt-and-suspenders fast-fail ahead of an atomic compare-and-swap, specify which guard produces the mandated observable. If the fast-fail runs inside the same in-process lock that serializes the writers, it fires first for concurrent tool calls and the CAS refusal path is never reached through the public API. The fast-fail must therefore carry the same outcome data (e.g., the winning successor identity) as the CAS refusal, or both paths must route through one shared warning builder; otherwise the spec's required warning can be emitted without the required detail even though the store-level CAS unit tests pass.
