---
tag: opencode-agents
author: jay-flowers
category: gotcha
created_at: 2026-11-11T19:18:54Z
identity: opencode-agents-20261111T191854-jay-flowers
tier: draft
---

`opencode run --agent <name>` cannot invoke `mode: subagent` agents — it silently (to stdout) falls back to the default agent with a visible warning, so a naive runtime check of a subagent's permissions actually tests the wrong agent. To runtime-verify a subagent's tool boundary, either copy the agent file with only `mode: subagent` → `mode: primary` (a probe whose mode-independence must then be disclosed as an assumption, as the curator-tool-allowlist acceptance matrix did) or exercise the real command path that spawns the subagent end-to-end. Go test suites cannot exercise host permission evaluation at all — enumerate host-dependent checks as a manual acceptance matrix with per-check evidence, and record which checks were verified at-rest instead of at-runtime.
