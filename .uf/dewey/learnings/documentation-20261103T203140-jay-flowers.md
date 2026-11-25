---
tag: documentation
author: jay-flowers
category: pattern
created_at: 2026-11-03T20:31:40Z
identity: documentation-20261103T203140-jay-flowers
tier: draft
---

When a diff modifies quality-gate values quoted in documentation, verify alignment direction before flagging gate tampering. In dewey PR #144, AGENTS.md Gaze example changed from --max-gaze-crapload=18 --min-contract-coverage=70 to --max-crapload=48 --max-gaze-crapload=45; .github/workflows/ci.yml (source of truth) already carried the new values and was NOT in the diff → documentation-accuracy fix, not gate modification. Gate tampering requires the CI config itself to change or docs diverging away from CI to mask failure. Curator flags doc drift, Adversary flags gate tampering; direction-of-alignment check keeps them from colliding.
