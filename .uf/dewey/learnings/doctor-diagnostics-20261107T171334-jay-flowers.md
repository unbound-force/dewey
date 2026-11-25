---
tag: doctor-diagnostics
author: jay-flowers
category: pattern
created_at: 2026-11-07T17:13:34Z
identity: doctor-diagnostics-20261107T171334-jay-flowers
tier: draft
---

Provider-routing diagnostics tests in dewey doctor assert a scoped output section rather than whole-command output: extract from the "Embedding Layer" header to the "Synthesis Layer" header (embeddingLayerSection helper in cli_test.go), then pair positive assertions (exact "project=X region=Y model=Z" substrings, header naming the provider) with a negative assertion that the lowercased section contains no "ollama" mention. This catches the issue #143 regression class — one provider's diagnostics (Ollama reachability checks, "ollama pull" fix hints) leaking into another provider's branch. Section scoping keeps the negative assertion safe because intermediate checks (embedding count) branch their fix hints on provider. The same shape works for any per-provider doctor section.
