---
tag: content-pipeline
author: jay-flowers
category: gotcha
created_at: 2026-11-11T20:26:15Z
identity: content-pipeline-20261111T202615-jay-flowers
tier: draft
---

Verifying website comment ids recorded in tasks.md from a curator review round has a tooling seam: `gh issue view --json comments` exposes GraphQL node ids (IC_kwDORWxzus...), not the REST databaseIds (e.g. 6804606610) captured when the comment was posted, and `gh api` is outside the curator persona's allowed bash surface — so numeric cross-verification of recorded ids is unavailable. Sufficient fallback, applied in gate-config-path-parity iteration 2 (dewey #191) against website#371/#376/#377: match the recorded comment descriptions against each open issue's change-scoped comments by content, timestamp clustering (all three posted within seconds of each other), and uniqueness — when each issue carries exactly one change-scoped comment, the id-to-comment mapping is unambiguous. Record this as the expected verification method when the numeric form cannot be reached, rather than treating the unmatched id form as a gap.
