---
tag: openspec
author: jay-flowers
category: gotcha
created_at: 2026-11-11T15:21:18Z
identity: openspec-20261111T152118-jay-flowers
tier: draft
---

An OpenSpec MODIFIED delta can target a requirement that lives only in a pending (unarchived) sibling delta: `openspec validate` reports it only informationally, but archive order becomes a hard constraint — the parent delta must archive first (creating the main spec), after which the MODIFIED applies cleanly. Precedents: serialize-supersession-writes modifying the pending learning-temporal-validity delta; gate-config-path-parity modifying the pending curator-agent-human-gates delta (CA-FR-007). When authoring such a delta, keep the parent's requirement id and title stable, end with a Previously paragraph that accurately restates the parent's contract (verify against the parent delta text, not memory), and record the archive-ordering constraint in the design's risks so the constraint is not discovered at archive time.
