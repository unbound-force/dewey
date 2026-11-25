---
tag: store-fallback
author: jay-flowers
category: gotcha
created_at: 2026-11-07T16:40:03Z
identity: store-fallback-20261107T164003-jay-flowers
tier: draft
---

A name-resolution shorthand added to vault.VaultStore.GetStorePage (the shared live read-through behind vault Client.GetPage/GetPageBlocksTree) is inherited by every consumer of those backend methods, not only the tools the change's spec names. In dewey PR #163 (issue #140), get_links forward direction and traverse's BFS both pass raw user names through GetPageBlocksTree (tools/navigate.go:243 and :322) and thus resolve bare names to compiled/<name>/learning/<name>, while backlinks (GetPageLinkedReferences) keep the exact requested name. Broadening via the shared fallback is the clean placement — but the change's spec capability list, README, and tests must enumerate the inheriting consumers or the Neighborhood-Rule audit trail understates the blast radius. Audit pattern: when changing a shared fallback behind a backend interface, grep the interface's consumers (GetPageBlocksTree) and record each user-name-passing call site; design.md D3's audit of bulk_update_properties is the model — get_links and traverse were missed.
