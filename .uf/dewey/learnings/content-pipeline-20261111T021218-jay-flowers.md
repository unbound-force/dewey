---
tag: content-pipeline
author: jay-flowers
category: pattern
created_at: 2026-11-11T02:12:18Z
identity: content-pipeline-20261111T021218-jay-flowers
tier: draft
---

On dewey OpenSpec changes whose user-visible delta is below the blog/tutorial threshold, the three-channel content convention can be closed without filing new issues: record the docs-sync determination as a scoped comment on the already-open website docs issue and the blog determination as a scoped comment on the already-open blog issue, then record BOTH issue numbers and comment ids beside the docs task in tasks.md. serialize-supersession-writes task 5.2 did this with website#380 (docs, issuecomment-6784758201) and website#392 (blog, issuecomment-6784758479). Verify deterministically with gh issue view — state OPEN, expected label, comment present — instead of trusting the task prose.
