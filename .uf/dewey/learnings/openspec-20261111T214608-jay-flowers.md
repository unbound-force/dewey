---
tag: openspec
author: jay-flowers
category: gotcha
created_at: 2026-11-11T21:46:08Z
identity: openspec-20261111T214608-jay-flowers
tier: draft
---

When an OpenSpec change is rebased after a sibling change of the same capability lands on main, requirement-id allocation must move to the next free range and record every sibling allocation in the delta header (e.g. CA-FR-013..014 this change, CA-FR-001..008 and CA-FR-009..012 for the landed siblings) rather than reusing the ids the draft originally claimed. A collision is caught only at archive time otherwise; recording the ranges in the allocation header makes the renumbering auditable and keeps the archive-time reconciliation prose coherent.
