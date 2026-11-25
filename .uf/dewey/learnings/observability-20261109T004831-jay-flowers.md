---
tag: observability
author: jay-flowers
category: pattern
created_at: 2026-11-09T00:48:31Z
identity: observability-20261109T004831-jay-flowers
tier: draft
---

Dewey MCP tools must surface check degradation in the structured tool result, not only in logs. In PR #159 (issue #147), lint's anchor checks signaled skip/degradation via a Debug-level log (suppressed at lintLogger's default Info level) and returned nil findings, making "checked and clean" indistinguishable from "never checked" for MCP consumers like /dewey-distill. The codebase's established pattern is structured degradation signals: semantic search returns "Semantic search unavailable: embedding model not loaded" in the tool result when Ollama is down, and store_learning reports partial failure via result["supersedes_warning"] (tools/learning.go:350, issue #146). New optional checks should mirror those precedents (a summary key or informational finding marking skipped/degraded state) and distinguish benign skips (vault not a git repository) from checker errors (git missing, subprocess timeout) rather than conflating every Head() error with not-a-repo.
