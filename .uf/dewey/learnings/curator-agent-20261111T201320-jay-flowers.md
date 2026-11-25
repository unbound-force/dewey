---
tag: curator-agent
author: jay-flowers
category: gotcha
created_at: 2026-11-11T20:13:20Z
identity: curator-agent-20261111T201320-jay-flowers
tier: draft
---

`opencode run --agent` cannot invoke `mode: subagent` agents — it silently falls back to the default agent with only a stdout warning, so runtime verification of subagent agent frontmatter (e.g. `tools:` glob enforcement) cannot use it directly. Verify subagent tool-glob enforcement either by spawning a throwaway `mode: subagent` agent through the task tool and having it report its toolset, or by running the command that binds the agent end-to-end; a primary-mode probe (flipping only the `mode` field) demonstrates enforcement solely under a mode-independence assumption. Environmental version skew (an installed dewey binary predating the `gate` subcommand) can block the end-to-end path — build the branch binary locally to unblock it.
