---
tag: test-architecture
author: jay-flowers
category: pattern
created_at: 2026-11-07T19:04:25Z
identity: test-architecture-20261107T190425-jay-flowers
tier: draft
---

Consumer-level tests of vault store-fallback behavior (live read-through, bare-name resolution) cannot use tools/mock_backend_test.go's mockBackend: it implements backend.Backend directly and bypasses the vault client's store fallback entirely. Build a real store-backed client instead — the newBareNameStoreClient pattern in tools/navigate_tool_test.go (dewey PR #163): t.TempDir() vault dir + store.New(":memory:") + vault.New(dir, vault.WithStore(s)) + c.Load(), seed store rows directly with s.InsertPage/s.I
