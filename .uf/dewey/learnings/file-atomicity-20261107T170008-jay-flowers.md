---
tag: file-atomicity
author: jay-flowers
category: pattern
created_at: 2026-11-07T17:00:08Z
identity: file-atomicity-20261107T170008-jay-flowers
tier: draft
---

Dewey's file-backed knowledge artifacts (.uf/dewey/learnings/*.md backups, compiled articles) are durability copies whose documented purpose is surviving graph.db loss. The codebase's established write pattern for them: guard file type with os.Lstat + Mode().IsRegular() before reading (artifacts/sync.go acquireContent), create with O_CREATE|O_EXCL (tools writeLearningFile), and rewrite via temp-file-then-os.Rename in the same directory with os.Remove cleanup on failure (tools/compile.go writeArticle, vault atomicWrite). In-place os.ReadFile/os.WriteFile rewrites follow symlinks and truncate before writing — on a durability artifact they recreate the exact loss the artifact exists to mitigate. markSupersededFile (learning-temporal-validity, PR #156) shipped with the in-place form and was flagged in round-2 council review; any new learnings write path must use the Lstat guard + temp/rename pattern and carry tests for symlink refusal and no-partial-write.
