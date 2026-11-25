---
tag: vault-store-bridge
author: jay-flowers
category: pattern
created_at: 2026-11-07T19:04:13Z
identity: vault-store-bridge-20261107T190413-jay-flowers
tier: draft
---

In dewey's vault read-through bridge, bare-name shorthand resolution belongs in the store fallback behind the shared read conversion (findStorePage behind GetStorePage in vault/vault_store.go), not in individual tools: every name-based consumer (get_page, get_links forward direction, traverse start pages, get_whiteboard, bulk_update_properties' read stage) inherits it through the single seam, while UUID paths (get_block) and the write guard (StorePageMeta) stay exact by simply not routing through it. Startu
