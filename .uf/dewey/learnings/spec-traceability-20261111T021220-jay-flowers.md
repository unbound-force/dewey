---
tag: spec-traceability
author: jay-flowers
category: gotcha
created_at: 2026-11-11T02:12:20Z
identity: spec-traceability-20261111T021220-jay-flowers
tier: draft
---

When a change modifies behavior reachable through a sibling capability's shared helper (e.g. serialize-supersession-writes changing markStaleUnverified in tools/lint_anchors.go via the shared updateLearningBackup), declare the sibling capability's delta in the same change even if the affected path is documented as "pre-fix residue" or an edge case. The OpenSpec archive syncs only declared deltas, so an undeclared sibling change leaves that capability's main spec silently inaccurate. In serialize-supersession-writes (#166) the lint file-leg refusal was implemented and tested but the lint-anchor-drift MODIFIED delta was deferred to a follow-up, leaving a traceability gap.
