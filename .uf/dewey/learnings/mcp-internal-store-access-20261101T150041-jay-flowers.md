---
tag: mcp-internal-store-access
author: jay-flowers
category: context
created_at: 2026-11-01T15:00:41Z
identity: mcp-internal-store-access-20261101T150041-jay-flowers
tier: draft
---

mcp-internal-store-access task 2.3 (vault GetBlock store fallback): the D2 fallback pattern from 2.2 (snapshot under c.mu.RLock, release, nil-store parity check, live GetStoreBlock/GetStorePage call, no caching, error surfaces unwrapped-as-error not masked not-found) is now established in GetPage, GetPageBlocksTree, and GetBlock — tasks 2.4 (GetAllPages merge), 2.5 (FullTextSearch merge), 2.6 (tag fallback) should mirror it but with D3 merge semantics (vault wins, dedupe by lowercased name, ListPagesExcludingSource("disk-local")). Non-obvious vault test fact: the Obsidian parser produces ONE root block per file whose Content is the full multi-line file body ("# Topic\n\nFrom the vault file." as a single block, children=0), so vault-side tests can grab a file block's UUID via GetPageBlocksTree(ctx, name)[0].UUID and it is always non-empty. Test suite conventions for this change live in vault/vault_test.go (newStoreBackedClient, blockTreeContains) and vault/vault_store_test.go (insertStorePageWithBlocks, assertBlockTreeEqual, newTestVaultStore); store-failure tests close the :memory: store to make queries fail deterministically.
