---
tag: vault-store-bridge
author: jay-flowers
category: pattern
created_at: 2026-11-07T19:04:12Z
identity: vault-store-bridge-20261107T190412-jay-flowers
tier: draft
---

When broadening name resolution on a read-through seam, the security audit is a write-flow enumeration, not a read-flow one: list every write method on the same client and verify each resolves through a mechanism the broadening cannot touch. In dewey's read-tools-bare-names (PR #163), broadening GetStorePage to try compiled/<name> then learning/<name> on a bare-name miss is safe because every vault-client write (CreatePage, AppendBlockInPage, UpdateBlock, RemoveBlock, DeletePage, RenamePage) res
