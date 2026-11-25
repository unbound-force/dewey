---
tag: go-conventions
author: jay-flowers
category: gotcha
created_at: 2026-11-11T20:49:57Z
identity: go-conventions-20261111T204957-jay-flowers
tier: draft
---

When a diagnostic companion function mirrors an authority function's checks in the same order to explain rejections (dewey tools/verify.go learningIneligibilityReason mirroring lint_anchors.go learningAnchors), the mirror can silently drift when the authority's rules change. Mitigations used in dewey-distill-verify-step: keep the authority as the only gate (the companion runs only after the authority rejects, so eligibility itself cannot diverge), document the coupling in both places, and state the failure mode (stale reason wording, never a wrong result). The extraction trigger is a second diagnostic consumer — then refactor the authority to return (eligible, reason) so the two cannot disagree.
