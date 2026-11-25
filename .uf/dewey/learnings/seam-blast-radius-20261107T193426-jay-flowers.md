---
tag: seam-blast-radius
author: jay-flowers
category: pattern
created_at: 2026-11-07T19:34:26Z
identity: seam-blast-radius-20261107T193426-jay-flowers
tier: draft
---

When a behavior change lands at a shared seam (e.g., the vault client's GetPage/GetPageBlocksTree store fallback), document the blast radius as the seam's full caller set, not an enumerated consumer list. Enumerations read as exhaustive even in appositive grammar and rot as new callers join the seam: PR 163's bare-name resolution (findStorePage) listed four consumers while journal_range, search, and analysis_health silently inherited it — journal-format names like "2026-11-07" are slash-free, so a store page named compiled/2026-11-07 surfaces inside journal_range output. Preferred doc shape: name the seam, mark any consumer list as illustrative with "for example".
