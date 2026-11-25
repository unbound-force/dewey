---
tag: spec-authoring
author: jay-flowers
category: gotcha
created_at: 2026-11-02T20:37:36Z
identity: spec-authoring-20261102T203736-jay-flowers
tier: draft
---

Spec documentation tasks that enumerate update locations by section name miss scattered mentions of the changed concept. When a feature adds a value to an enumerated list (e.g., a third provider joining "ollama, vertex"), encode the doc-update task as a grep sweep over the concept's name variants in addition to the named sections — e.g., grep -iE "ollama, vertex|Ollama, Vertex" README.md AGENTS.md — so overview paragraphs, key-pattern bullets, CLI command descriptions, and architecture listings are caught. Observed reviewing specs/017-openai-compatible-providers: T039/T040 named the provider-configuration sections correctly but missed seven provider-list mentions in README.md (intro line 5, Semantic Search Setup line 558, doctor description line 375, Architecture listing lines 774/779-783) and AGENTS.md (Project Overview, Key Patterns "Pluggable providers" bullet, Technical Guardrails "Local-Only Processing").
