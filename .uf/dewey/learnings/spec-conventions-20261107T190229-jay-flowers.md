---
tag: spec-conventions
author: jay-flowers
category: pattern
created_at: 2026-11-07T19:02:29Z
identity: spec-conventions-20261107T190229-jay-flowers
tier: draft
---

When a convention-pack [MUST] rule predates an artifact but was never operationalized in the repository's workflow, weigh advisory findings about it as alignment judgment (author-decides), not per-artifact defects. Signal to check: corpus compliance rate. In unbound-force/dewey (PR 157, Nov 2026), the meta-prompt pack requires FR-NNN ids, a per-requirement id line, and an FR-range allocation header in OpenSpec deltas (MP-002/MP-013/MP-014), yet 5 of 8 deltas lack ids, 0 of 8 have allocation headers, and the main specs the deltas sync into use bare capability-style names. A single-delta retrofit in a feature PR produces inconsistency rather than the stable cross-referencing the numbering exists to deliver, and MP-014-style range allocation is a repo-level decision that should not be improvised piecemeal. The durable fix is a corpus-wide normalization change or a pack amendment scoping the rule — and post-council-APPROVE change discipline argues against touching an approved PR for a LOW advisory even when the edit would be re-review-exempt.
