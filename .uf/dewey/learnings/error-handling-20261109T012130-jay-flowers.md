---
tag: error-handling
author: jay-flowers
category: pattern
created_at: 2026-11-09T01:21:30Z
identity: error-handling-20261109T012130-jay-flowers
tier: draft
---

Dewey tools that dual-write (SQLite store first, markdown backup second — store_learning's supersedes back-link and lint's anchor-drift fix) must treat the store write as the commit point: count/report the operation as done once UpdatePage succeeds, and surface a backup-write failure as a structured warning string in the tool result (the supersedes_warning pattern from #146, learning.go:350). stderr logging is invisible to MCP clients, and an undercounting result field misrepresents durable store state — the divergence can even silently revert after graph.db deletion, because orphaned markdown backups are re-ingested with their frontmatter. A missing backup file is already a non-error (best-effort "when present"); only existing-but-unwritable/malformed backups fail, and that failure belongs in the result, not the log.
