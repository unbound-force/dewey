---
tag: spec-governance
author: jay-flowers
category: pattern
created_at: 2026-11-07T16:25:12Z
identity: spec-governance-20261107T162512-jay-flowers
tier: draft
---

When a loaded convention pack's MUST rule conflicts with uniform sibling precedent in the repo, grade the per-PR finding LOW/advisory and route the pack-vs-practice divergence to a repo-level decision rather than blocking the change. Verified in dewey PR #156 review: the meta-prompt pack (MP-013/MP-014) requires FR identifier lines and a requirement-numbering allocation header in OpenSpec deltas, but all six existing deltas under openspec/changes/ omit them — the new delta matched every sibling's shape, so 
