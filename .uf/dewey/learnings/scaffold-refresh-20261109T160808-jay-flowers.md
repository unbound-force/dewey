---
tag: scaffold-refresh
author: jay-flowers
category: gotcha
created_at: 2026-11-09T16:08:08Z
identity: scaffold-refresh-20261109T160808-jay-flowers
tier: draft
---

Scaffold refreshes that reverse repo-local policy desync the hero repo's AGENTS.md: uf init redeploys scaffold assets (commands, plugins, .gitignore) but never touches the user-owned AGENTS.md, so sections documenting the old policy survive as contradictory guidance, and scaffold citations of meta-repo AGENTS.md sections dangle (dewey PR #167: the new learnings separate-PR policy and .uf/dewey/config.yaml ignore both contradict dewey's "Learnings Direct-Commit Practice" and git-tracked-config sections, and 
