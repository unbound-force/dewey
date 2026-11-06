---
tag: documentation-pipeline
author: jay-flowers
category: gotcha
created_at: 2026-11-05T18:03:38Z
identity: documentation-pipeline-20261105T180338-jay-flowers
tier: draft
---

Docs-sync issue verification must cover enumerated values, defaults, and fallback claims, not just tool lists. dewey PR #156's website#380 body listed validity as "current | superseded | historical (default current)" while the shipped enum is current, stale-unverified, superseded, historical with absent meaning unknown (no default) — an invented default plus a dropped enum value, alongside a claimed as_of "wall-clock fallback" that does not exist (failure omits as_of entirely).
