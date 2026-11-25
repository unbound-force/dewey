---
tag: scaffold-governance
author: jay-flowers
category: gotcha
created_at: 2026-11-09T16:08:02Z
identity: scaffold-governance-20261109T160802-jay-flowers
tier: draft
---

Scaffold refreshes (uf init re-runs) deploy meta-repo policy into consuming repos whose user-owned AGENTS.md may document the opposite practice. When a refreshed command or tool cites a governance rule by section name (e.g. "the Commit scope rule in AGENTS.md"), verify the target repo's AGENTS.md actually contains that section — deployed cross-references dangle silently otherwise. In dewey PR #167 (chore/uf-scaffold-refresh), the learnings-policy reversal (never commit learnings on feature branches; separat
