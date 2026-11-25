---
tag: documentation
author: jay-flowers
category: pattern
created_at: 2026-11-07T20:14:53Z
identity: documentation-20261107T201453-jay-flowers
tier: draft
---

When documentation enumerates the consumers of a shared code seam, the enumeration drifts as new callers appear. Dewey's read-tools-bare-names change (PR #163) hit this: README, AGENTS.md, and the OpenSpec proposal fixed a four-consumer list (get_links forward direction, traverse start pages, get_whiteboard, bulk_update_properties), then journal_range was recognized as a fifth caller of the same GetPage/GetPageBlocksTree store-fallback seam. The durable fix, applied in the feedback iteration: name the seam itself as the blast radius ("every GetPage/GetPageBlocksTree caller"), mark consumer lists in prose as illustrative rather than exhaustive, and leave spec-delta "including ..." enumerations untouched since "including" is non-exhaustive by construction. Exhaustive prose lists outside requirements are what go stale; the requirement's own "including" list stays conformant.
