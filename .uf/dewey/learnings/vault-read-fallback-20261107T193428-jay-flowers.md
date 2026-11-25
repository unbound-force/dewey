---
tag: vault-read-fallback
author: jay-flowers
category: pattern
created_at: 2026-11-07T19:34:28Z
identity: vault-read-fallback-20261107T193428-jay-flowers
tier: draft
---

Shared-seam changes fan out to every caller, and documentation enumerations derived from a change's test list understate the blast radius. In dewey PR #163 (read-tools-bare-names), bare-name store resolution was placed in VaultStore.findStorePage behind GetStorePage, whose only callers are vault Client.GetPage and Client.GetPageBlocksTree — so every tool reading a page by name through the vault client inherited it. The docs listed only the four tools with consumer-level tests (get_links forward, traverse start, get_whiteboard, bulk_update_properties); journal_range also calls GetPage with slash-free date-shaped names ("2026-11-07"), so compiled/<date> store pages surface in journal_range output — caught only in review. Derive consumer lists for shared seams from the call graph (grep the seam's callers), and write user-facing enumerations as explicitly illustrative ("including ...") with the blast radius stated as the seam itself.
