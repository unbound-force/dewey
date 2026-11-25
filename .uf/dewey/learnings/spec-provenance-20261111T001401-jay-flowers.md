---
tag: spec-provenance
author: jay-flowers
category: pattern
created_at: 2026-11-11T00:14:01Z
identity: spec-provenance-20261111T001401-jay-flowers
tier: draft
---

When reviewing a documentation-only PR that corrects provenance (who introduced an import edge, threshold, or behavior), verify each claim against primary artifacts rather than trusting the citing document. In the dewey repo the primary artifacts are: OpenSpec changes carry a `.openspec.yaml` `created` date, speckit `spec.md` files carry a `**Created**` header, and the introducing change's `design.md` decision text names the exact files it directs. In PR #186 this discipline confirmed the corrected attribution (`llm → embed` introduced by ollama-host-fallback: `.openspec.yaml` created 2026-06-18, design.md D5 plus the llm/config.go trade-off note) and refuted spec 017 research.md R13's attribution to spec 016 (spec.md `**Created**: 2026-05-05`, which predates the import). A later spec's retrospective attribution is secondary evidence, never primary.
