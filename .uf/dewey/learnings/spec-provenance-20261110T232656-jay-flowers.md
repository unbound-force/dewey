---
tag: spec-provenance
author: jay-flowers
category: gotcha
created_at: 2026-11-10T23:26:56Z
identity: spec-provenance-20261110T232656-jay-flowers
tier: draft
---

In the dewey repo, spec 017's research.md R13 attributes the `llm → embed` import edge to spec 016's provider-config consolidation. That attribution is wrong: `git log -S'embed.DefaultOllamaEndpoint' -- llm/` shows commit 340a944 (2026-06-18, the ollama-host-fallback OpenSpec change, design.md D5) introduced the import in llm/config.go and llm/provider.go, while spec 016's plan was committed 2026-05-05 and predates it. synthesis-endpoint-env design.md D1/D3 (2026-08-17) restates the edge. Lesson: when documenting cross-package import-edge provenance, verify against git pickaxe history plus each candidate change's .openspec.yaml `created` date and design decisions — never trust a later spec's retrospective attribution, even inside a completed spec. (branch quick/llm-leaf-dependency-backpointer, 2026-11-10, issue #155)
