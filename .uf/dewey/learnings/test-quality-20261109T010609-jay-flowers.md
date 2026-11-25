---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-09T01:06:09Z
identity: test-quality-20261109T010609-jay-flowers
tier: draft
---

Error-path tests for dual-write operations (SQLite store + markdown backup) must pin the post-failure state of every sink, not just the error return and aggregate counts. In dewey's lint anchor fix, markStaleUnverified persists the store (UpdatePage) before writing the markdown backup (markStaleFile), so a backup failure — e.g. a learning file without YAML frontmatter, which setFrontmatterKeys rejects — leaves the store already marked validity:stale-unverified while the learning is excluded from fixed.anchor_drift. Error-presence and count assertions cannot detect that divergence; only per-sink state assertions can (pageProps for the store, os.ReadFile/fileFrontmatter for the file). PR #146's supersedes_warning set the precedent of surfacing partial dual-write failures in tool results; the same discipline applies to the tests that guard them.
