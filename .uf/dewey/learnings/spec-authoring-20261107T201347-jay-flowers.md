---
tag: spec-authoring
author: jay-flowers
category: gotcha
created_at: 2026-11-07T20:13:47Z
identity: spec-authoring-20261107T201347-jay-flowers
tier: draft
---

Advisory-fix rounds that re-wrap OpenSpec/spec prose can push lines past the 72-character specification wrap ceiling (meta-prompt pack MP-004). In the read-tools-bare-names feedback round (dewey PR #163), folding the trade-offs subsection into design.md's Risks / Trade-offs section and rewording the proposal's consumer enumeration produced a 74-character line (proposal.md) and a 73-character line (design.md) while every other reflowed line observed ≤72. After any content move between spec sections, re-check wrap deterministically: grep -E '^.{73,}$' over the change directory, then re-wrap the hits that sit on changed lines.
