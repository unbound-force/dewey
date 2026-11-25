---
tag: documentation-content-pipeline
author: jay-flowers
category: gotcha
created_at: 2026-11-11T21:46:07Z
identity: documentation-content-pipeline-20261111T214607-jay-flowers
tier: draft
---

When a change adds a step to an already-tracked multi-step workflow, extending the primary docs-sync issue by comment is necessary but not sufficient: the sibling tutorial and blog tracking issues must also be reviewed. A tutorial issue that enumerates the pipeline stages is the authoring spec for downstream content, so omitting the new stage lets a tutorial ship a stale pipeline. In this repo the established pattern for /dewey-distill changes is to comment each of website#371 (docs), #376 (tutorial), and #377 (blog) with the delta or an explicit below-threshold determination.
