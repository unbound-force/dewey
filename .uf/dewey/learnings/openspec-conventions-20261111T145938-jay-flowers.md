---
tag: openspec-conventions
author: jay-flowers
category: gotcha
created_at: 2026-11-11T14:59:38Z
identity: openspec-conventions-20261111T145938-jay-flowers
tier: draft
---

An openspec capability is governed by its change deltas even before it is archived to openspec/specs/. Declaring 'Modified Capabilities: None' requires surveying openspec/changes/*/specs/ deltas, not just openspec/specs/ (which in dewey holds only documentation-accuracy, quality-ratchet, and homebrew-dependency). When a change modifies behavior of an un-archived capability — e.g., knowledge-distillation's KD-FR-101..103 governing /dewey-distill's gate questions and the pending 'Gated Distill Workflow' requirement — it must carry its own delta against that capability using ADDED requirements in the capability's reserved FR range; a MODIFIED operation is refused at archive time while no main spec exists (curator-agent-human-gates' KD allocation note documents both the refusal rule and the KD-FR-104..199 reservation for future distill changes).
