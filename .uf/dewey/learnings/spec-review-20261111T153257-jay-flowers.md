---
tag: spec-review
author: jay-flowers
category: gotcha
created_at: 2026-11-11T15:32:57Z
identity: spec-review-20261111T153257-jay-flowers
tier: draft
---

When a new OpenSpec change invalidates a factual premise embedded in a pending (unarchived) sibling change's requirement text, ADDED requirements alone are not enough: the archive-time merge produces a main spec that carries both the invalidated premise and the new mandate, contradicting each other inside one capability. Express the reconciliation explicitly — a MODIFIED requirement with a Previously paragraph, an amendment to the pending sibling delta, or a tasked reconciliation recorded in the new change — before either change archives.
