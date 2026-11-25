---
tag: error-observability
author: jay-flowers
category: pattern
created_at: 2026-11-09T00:04:10Z
identity: error-observability-20261109T000410-jay-flowers
tier: draft
---

A diagnostic tool's structured result must distinguish "check ran and found nothing" from "check never ran". In dewey's lint, a git subprocess failure mid-anchor-check drops all findings with only a stderr Warn, and a Head() failure of any kind is logged at Debug (suppressed at charmbracelet's default Info level) as "not a git repository" — MCP clients see status "clean" with no degradation signal. Degrade loudly in the result (summary key, finding, or warning field); stderr logs are invisible to MCP callers.
