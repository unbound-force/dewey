---
tag: scaffold-refresh
author: jay-flowers
category: gotcha
created_at: 2026-11-09T16:54:29Z
identity: scaffold-refresh-20261109T165429-jay-flowers
tier: draft
---

Refreshing scaffold-managed command files from an upstream meta-repo can silently flip repo-local policy. The uf-scaffold refresh changed the learnings lifecycle to "never commit .uf/dewey/learnings/ on feature branches; separate chore/dewey-learnings-* PR (see the Commit scope rule in AGENTS.md)" — but the dewey repo's AGENTS.md documents the opposite direct-commit practice and carries no such Commit scope rule, leaving knowledge artifacts untracked (swept by `git clean -fd`) with no wired publishing path. Scaffold refreshes that change artifact-lifecycle or config-tracking policy must reconcile the target repo's AGENTS.md in the same change, or the refreshed commands cite rules that do not exist and contradict the repo's own documented practice.
