---
tag: mcp-internal-store-access
author: jay-flowers
category: gotcha
created_at: 2026-11-01T17:21:52Z
identity: mcp-internal-store-access-20261101T172152-jay-flowers
tier: draft
---

mcp-internal-store-access task 5.3 (constitution verification): when verifying the "deterministic no-sleep tests" principle on a branch with an uncommitted tree, a raw `grep -n "Sleep"` over vault/vault_test.go HITS six pre-existing sleeps (lines ~990-1136) inside the fsnotify watcher tests TestWatchFileCreate/Modify/Delete and TestConcurrentAccess — these predate the change and must not be flagged. The correct scope check is `git diff HEAD -- <test file> | grep "^+.*Sleep"` (zero matches across all eight touched test files = PASS). Other verified anchors: nil-store guards at vault/vault.go:513 (Reload), 563 (GetAllPages), 603 (GetPage), 640 (GetPageBlocksTree), 678 (GetBlock), 1633 (storePagesTagResults), 1893 (FullTextSearch); shared persistentStorageError at tools/persistent_storage.go:20 with 8 call sites; cli.go:2503 promoteStoreOpenError + cli.go:987 detectLockHolder for PID/command lock diagnostics; backend/ has zero diff vs upstream/main and HEAD. Completion notes recorded at openspec/changes/mcp-internal-store-access/completion.md — all four principles PASS.
