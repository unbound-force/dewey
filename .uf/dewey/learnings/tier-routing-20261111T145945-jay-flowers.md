---
tag: tier-routing
author: jay-flowers
category: gotcha
created_at: 2026-11-11T14:59:45Z
identity: tier-routing-20261111T145945-jay-flowers
tier: draft
---

dewey's AGENTS.md Strategic vs Tactical table assigns new-MCP-tool changes to Speckit categorically (Speckit "Always", OpenSpec "Never"). The repo's history confirms the row: every tool addition arrived via Speckit (compile/lint/promote 013, curate 015, store_compiled 016, index/reindex 011) while OpenSpec changes only modified existing tools. A change that adds a new MCP tool under openspec/changes/ crosses a documented routing rule even when it otherwise fits the OpenSpec shape — it needs either a Speckit re-route or a recorded human-approved tier exception in the proposal. Check the table BEFORE choosing the opsx/ branch; problem affinity with a lineage of OpenSpec changes is not a routing signal.
