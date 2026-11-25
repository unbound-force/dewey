---
tag: openspec
author: jay-flowers
category: gotcha
created_at: 2026-11-11T15:32:46Z
identity: openspec-20261111T153246-jay-flowers
tier: draft
---

In OpenSpec, when a new change's premise disproves a factual assertion inside a sibling change's requirement (e.g., "the current V1 agent format cannot hard-scope individual MCP tools"), merely superseding the sibling's *design* decision is not enough: both pending deltas land in the same main spec at archive time (in either archive order), so the merged spec carries both the stale assertion and the contradicting new requirement. While both changes are still un-archived (mutable), either amend the sibling delta's requirement text directly or record the supersession in the new change so the reconciliation happens at archive — otherwise the main spec self-contradicts.
