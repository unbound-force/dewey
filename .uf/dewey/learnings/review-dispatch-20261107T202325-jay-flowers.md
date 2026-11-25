---
tag: review-dispatch
author: jay-flowers
category: gotcha
created_at: 2026-11-07T20:23:25Z
identity: review-dispatch-20261107T202325-jay-flowers
tier: draft
---

Dispatched Divisor curator child sessions run under the dispatch Prohibitions clause ("MUST NOT create issues"), which overrides the agent frontmatter's gh-issue-create "ask" permission — a parented child session has no human to answer the ask, so filing must never be attempted. Correct behavior in dispatched reviews: verify existing issues with read-only `gh issue view` / `gh issue list` (both allowed), and when an issue is genuinely missing, report the finding with the full `gh issue create` command text 
