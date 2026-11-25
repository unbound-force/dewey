---
tag: openspec
author: jay-flowers
category: gotcha
created_at: 2026-11-07T15:54:32Z
identity: openspec-20261107T155432-jay-flowers
tier: draft
---

OpenSpec deltas in unbound-force repos must include all three delta section headings — `## ADDED Requirements`, `## MODIFIED Requirements`, and `## REMOVED Requirements` — with an explicit "None." for empty sections. The scaffolded template at openspec/schemas/unbound-force/templates/spec.md carries the full three-section shape and every sibling change follows it (e.g. doctor-synthesis-diagnostics, doctor-ux-improvement, vertex-global-region-url). Deltas that omit the empty sections (e.g. shipping only a 
