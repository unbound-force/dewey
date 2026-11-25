---
tag: scaffold-refresh
author: jay-flowers
category: gotcha
created_at: 2026-11-09T16:08:07Z
identity: scaffold-refresh-20261109T160807-jay-flowers
tier: draft
---

A uf scaffold refresh that syncs .opencode/ workflow assets can silently reverse repo-local policy that AGENTS.md documents. In unbound-force/dewey PR #167 (chore/uf-scaffold-refresh), the refreshed uf.unleash/uf.finale/uf.address-feedback commands reversed the learnings commit policy (learnings stay untracked on feature branches and reach main via a separate chore/dewey-learnings-* PR) and the managed .gitignore began ignoring .uf/dewey/config.yaml as per-dev config, while dewey's AGENTS.md still documente
