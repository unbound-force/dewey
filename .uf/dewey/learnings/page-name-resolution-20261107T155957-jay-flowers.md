---
tag: page-name-resolution
author: jay-flowers
category: context
created_at: 2026-11-07T15:59:57Z
identity: page-name-resolution-20261107T155957-jay-flowers
tier: draft
---

Dewey page-name resolution now lives in three deliberately different policies: (1) reads — vault.findStorePage (vault/vault_store.go), exact fold-case lookup then compiled/<name> then learning/<name>, first-wins, slash-containing names skip the shorthand (issue #140, read-tools-bare-names); (2) promote — tools.resolvePromotePage (tools/promote.go), four tiers with ambiguity rejection because promote mutates a trust tier; (3) the write guard — vault.VaultStore.StorePageMeta, exact-only so a vault page can st
