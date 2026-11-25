---
tag: vault-store
author: jay-flowers
category: pattern
created_at: 2026-11-07T15:59:54Z
identity: vault-store-20261107T155954-jay-flowers
tier: draft
---

Read-path convenience resolution must stay unreachable from mutation paths, and a test must pin that boundary. Precedent (dewey read-tools-bare-names, PR #163): findStorePage resolves a bare name to compiled/<name> then learning/<name>, first found wins, serving only get_page/get_page_blocks_tree reads — while the write guard StorePageMeta keeps its own exact GetPageFoldCase lookup and resolvePromotePage keeps refusing ambiguous names because promote mutates a trust tier. TestBareNameResolutionDoesNotReachW
