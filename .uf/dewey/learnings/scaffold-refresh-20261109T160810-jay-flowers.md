---
tag: scaffold-refresh
author: jay-flowers
category: gotcha
created_at: 2026-11-09T16:08:10Z
identity: scaffold-refresh-20261109T160810-jay-flowers
tier: draft
---

When a scaffold refresh changes a plugin tool's input contract, repo-local command files outside the scaffold set keep stale invocation guidance. In dewey PR #167 the uf-workflow git_push tool gained a required branch argument plus a BRANCH_MISMATCH refusal, but the repo-local dewey-publish.md still describes an argument-less git_push call (its raw `git push <remote> <branch>` fallback still works). Grep .opencode/commands and .opencode/skills for every tool whose contract changed before merging a scaffold 
