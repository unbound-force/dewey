---
tag: tool-observability
author: jay-flowers
category: pattern
created_at: 2026-11-09T00:48:34Z
identity: tool-observability-20261109T004834-jay-flowers
tier: draft
---

A log-only degradation signal never reaches MCP consumers. When a Dewey tool silently skips or partially fails a check (e.g., lint anchor checks outside a git repository, or a git checker error mid-run), the degraded state must surface in the structured result — a summary key, warnings field, or informational finding — following the supersedes_warning (#146) and semantic-search-unavailability precedents. Otherwise automation such as /dewey-distill cannot distinguish "checked and clean" from "never checked", weakening Observable Quality (constitution Principle III). Two implementation rules follow: distinguish the documented skip condition from unexpected errors (never label a Head() timeout as "not a git repository"), and pin any new response-contract field with a handler-level test that forces the degraded branch and asserts the field's presence and value — helper-level tests plus absence-assertions leave the result-key wiring mutation-survivable (the dewey PR #156 supersedes_warning gap).
