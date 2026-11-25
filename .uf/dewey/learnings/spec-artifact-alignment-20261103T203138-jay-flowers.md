---
tag: spec-artifact-alignment
author: jay-flowers
category: gotcha
created_at: 2026-11-03T20:31:38Z
identity: spec-artifact-alignment-20261103T203138-jay-flowers
tier: draft
---

A spec-artifact alignment pass after behavior-changing feedback fixes must sweep every artifact describing the changed behavior — including already-completed task text in tasks.md. Observed in dewey 017 round 3: fc66f07 updated spec/plan/research/wire-contract but left tasks.md T007 claiming default-redirect reliance, contradicting T013. Completed task bullets read as historical record but remain normative artifact text future audits compare against implementation.
