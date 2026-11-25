---
tag: vault-store-bridge
author: jay-flowers
category: pattern
created_at: 2026-11-07T19:04:20Z
identity: vault-store-bridge-20261107T190420-jay-flowers
tier: draft
---

Broadening a read-through bridge without broadening its write guard is the safe asymmetry direction, provided three conditions hold. In dewey's read-tools-bare-names change (PR #163, issue #140), reads now resolve bare names to compiled/<name> and learning/<name> while the write guard (StorePageMeta) keeps its exact lookup — so create_page("gateway") still succeeds next to a compiled/gateway store page. This is the inverse of the stale-row trap (learning vault-store-bridge-20261102T031557, where the guard r
