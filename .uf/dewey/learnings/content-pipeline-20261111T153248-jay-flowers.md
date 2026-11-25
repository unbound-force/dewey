---
tag: content-pipeline
author: jay-flowers
category: gotcha
created_at: 2026-11-11T15:32:48Z
identity: content-pipeline-20261111T153248-jay-flowers
tier: draft
---

When a spec plans website-content reconciliation against a prior change's issues (docs/blog/tutorial), the closed-issue fallback must cover every content channel the open-case branch reconciles — or explicitly record the decision that a docs-only sync suffices. A fallback that files only the docs-labeled issue (the minimum the Website Documentation Sync rule requires) silently drops the blog and tutorial threads once those issues close; the published blog post and tutorial then describe a stale permission model with no tracking issue to catch the drift.
