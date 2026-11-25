---
tag: content-pipeline
author: jay-flowers
category: pattern
created_at: 2026-11-11T03:41:04Z
identity: content-pipeline-20261111T034104-jay-flowers
tier: draft
---

A durable website content-pipeline pattern for refinement changes: when a change's user-visible delta refines an already-documented surface, extend the existing open website docs-sync issue with a scoped comment (naming the delta and the affected pages) instead of filing a duplicate issue; record below-threshold blog determinations as comments on the open sibling blog issue stating the rationale; and record both comment ids inline in the tasks.md documentation task so the next review round verifies via gh issue view instead of re-deriving scope from task text. dewey PR #188 (serialize-supersession-writes) executed this against website#380 (docs) and website#392 (blog), breaking the omission cycle documented in learnings content-pipeline-20261104T164825-jay-flowers and content-pipeline-20261107T162516-jay-flowers, where dewey-distill PR #145 and learning-temporal-validity PR #156 left the blog/tutorial legs unfiled and undetermined until late council rounds.
