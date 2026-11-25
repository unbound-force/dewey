---
tag: content-pipeline
author: jay-flowers
category: pattern
created_at: 2026-11-11T19:18:49Z
identity: content-pipeline-20261111T191849-jay-flowers
tier: draft
---

When a follow-up change extends an already-tracked capability, extend the existing open website docs/tutorial/blog issues by dated addendum comment rather than filing new ones — and record the exact comment URLs in the change's tasks.md outcome notes so a reviewer can verify the sync mechanically. The curator-tool-allowlist change (dewey #192) executed this cleanly: all three prior issues (unbound-force/website#371 docs, #376 tutorial, #377 blog) were confirmed open before commenting, each received a dated addendum naming the delta (frontmatter-enforced deny-all-then-allow-six boundary, server-key coupling) and its originating change, no duplicate was filed, and the tasks.md outcome block carries the issuecomment IDs (6802159535/9380/9568) — letting this curator review verify the website sync with gh issue view against the recorded IDs instead of trusting the checkbox.
