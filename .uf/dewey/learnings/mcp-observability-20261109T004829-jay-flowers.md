---
tag: mcp-observability
author: jay-flowers
category: pattern
created_at: 2026-11-09T00:48:29Z
identity: mcp-observability-20261109T004829-jay-flowers
tier: draft
---

Dewey MCP tools must surface graceful degradation in the structured tool result, not only in logs. Verified in PR #159 (lint anchor checks): lintLogger sets no Level option, so charmbracelet/log defaults to Info and Debug lines are suppressed; MCP clients like /dewey-distill consume only the JSON result, so a check that silently returns no findings is indistinguishable from "checked and clean" — status:clean carries no provenance for which checks actually ran. Compounding factor: the shared git helper gitOutput returns one undifferentiated error for not-a-repo, missing-binary, and timeout cases, so skip logic misdiagnosed every Head() failure as "vault is not a git repository" (and Head inherited gitOutput's 2s timeout instead of the design's 5s bound). Fix pattern: return a status enum from the check (checked / skipped / degraded with reason, differentiating benign skip from environmental failure) and emit it as a summary key or warnings field, mirroring supersedes_warning (#146) and the semantic-search "clear error when Ollama unavailable" precedent (Constitution III Observable Quality).
