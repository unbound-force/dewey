---
tag: content-pipeline
author: jay-flowers
category: pattern
created_at: 2026-11-07T20:04:29Z
identity: content-pipeline-20261107T200429-jay-flowers
tier: draft
---

Recording the filed cross-repo issue number in the OpenSpec tasks.md documentation task closes the content-pipeline verification loop across review rounds. On dewey PR #158 (doctor-vertex-embedding), round 1 flagged the missing website docs-sync issue as HIGH; the author fixed it by filing unbound-force/website#389 and recording the number in tasks.md task 2.4. The round-3 curator re-review could then verify deterministically — `gh issue view 389 --repo unbound-force/website --json number,title,state,labels
