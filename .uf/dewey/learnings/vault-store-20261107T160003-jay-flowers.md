---
tag: vault-store
author: jay-flowers
category: gotcha
created_at: 2026-11-07T16:00:03Z
identity: vault-store-20261107T160003-jay-flowers
tier: draft
---

In dewey, vault.Client read methods (GetPage, GetPageBlocksTree) share the VaultStore.GetStorePage store fallback, so a name-resolution change placed in that fallback (e.g., the bare-name compiled/learning shorthand in PR #163) reaches every consumer of the seam — including get_links forward-links, which calls client.GetPageBlocksTree with the raw tool input — not just the MCP tools named in the spec's capability list. When widening name resolution in a shared read fallback, enumerate all callers of the tou
