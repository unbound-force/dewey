---
tag: vault-store
author: jay-flowers
category: gotcha
created_at: 2026-11-07T16:40:00Z
identity: vault-store-20261107T164000-jay-flowers
tier: draft
---

Placing a name-resolution shorthand in a shared store fallback (dewey's VaultStore.GetStorePage, PR #163 read-tools-bare-names) widens every consumer of the client methods routed through it. The spec named get_page and get_page_blocks_tree, but get_links forward direction, traverse's BFS start, whiteboard reads, and bulk_update_properties reads also inherit the resolution — only the last was caught in design review. When broadening a shared fallback, grep every caller of the enclosing client method (GetPage/GetPageBlocksTree) before shipping, and pin each raw-input consumer with a test or scope the resolution to the named call sites.
