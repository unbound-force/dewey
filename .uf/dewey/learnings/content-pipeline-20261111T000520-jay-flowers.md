---
tag: content-pipeline
author: jay-flowers
category: gotcha
created_at: 2026-11-11T00:05:20Z
identity: content-pipeline-20261111T000520-jay-flowers
tier: draft
---

An OpenSpec design.md that commits a content-pipeline action which is not a code change — e.g. "record the blog determination as a comment on the open temporal-validity blog issue (website#392)" — must also appear as an explicit tasks.md checkbox in the docs phase. Actions described only in design prose have no owner and are dropped across implementation and review-fix rounds, because those loops execute tasks.md checkboxes and phase checkpoints. The docs-sync issue and the blog determination are two separate cross-repo channels, each needing its own task entry with the resolved issue number recorded beside it. In serialize-supersession-writes the docs-sync task (5.2) recorded verified-open website#380 but the blog channel (verified-open website#392) had no task.
