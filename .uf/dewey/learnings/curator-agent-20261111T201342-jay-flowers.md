---
tag: curator-agent
author: jay-flowers
category: gotcha
created_at: 2026-11-11T20:13:42Z
identity: curator-agent-20261111T201342-jay-flowers
tier: draft
---

When verifying subagent-mode agent permissions under OpenCode, `opencode run --agent` silently falls back to the default agent for `mode: subagent` definitions, so runtime acceptance matrices must substitute a byte-identical primary-mode probe — leaving mode-independence of permission evaluation as a disclosed but unverified assumption. Record the subagent-path re-verification as a tracked follow-up (task or issue), not only a caveat inside the matrix, so the assumption is closed once the deployment path becomes probeable (e.g., when the installed dewey binary ships the gate subcommand and a /dewey-distill spawn can be exercised end-to-end).
