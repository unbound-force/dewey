---
tag: spec-authoring
author: jay-flowers
category: gotcha
created_at: 2026-11-11T14:50:56Z
identity: spec-authoring-20261111T145056-jay-flowers
tier: draft
---

When a spec wants partial results to survive into an error payload, 'ride along as context only' is not a contract. The requirement must define the shape: which fields partial entries carry, whether verdicts are stripped or rewritten, and how the no-false-clean guarantee is asserted. A requirement that both allows partial candidates in the error result and forbids reporting any candidate with a verdict reads as contradictory, and the implementing test inherits the ambiguity because it must pick one interpretation to assert. Observed in dewey-distill-verify-step KV-FR-004 (error-result routing described as load-bearing, yet the partial-candidate shape undefined).
