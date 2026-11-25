---
tag: scaffold-refresh
author: jay-flowers
category: gotcha
created_at: 2026-11-09T16:08:11Z
identity: scaffold-refresh-20261109T160811-jay-flowers
tier: draft
---

Refreshing scaffold-managed files from the meta repo (uf init / scaffold refresh PRs) can silently reverse repo-specific operational policies that AGENTS.md documents. In dewey PR #167, the refreshed .gitignore ignored .uf/dewey/config.yaml while AGENTS.md documented the per-vault config as git-tracked and shareable, and the refreshed commands reversed the learnings direct-commit practice while citing an AGENTS.md "Commit scope rule" the repo never contained. A scaffold refresh PR must diff incoming policy-
