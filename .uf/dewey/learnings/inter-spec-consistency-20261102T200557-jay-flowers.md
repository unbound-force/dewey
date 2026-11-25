---
tag: inter-spec-consistency
author: jay-flowers
category: context
created_at: 2026-11-02T20:05:57Z
identity: inter-spec-consistency-20261102T200557-jay-flowers
tier: draft
---

specs/013-knowledge-compile/contracts/llm-interface.md invariant 7 ("The llm package MUST NOT import any Dewey packages — leaf dependency") diverges from current reality: llm/provider.go and llm/config.go import embed (arrived with 016's provider-config consolidation). Specs that add new imports to llm (e.g. spec 017's llm → openaicompat) should record the divergence explicitly — the way research.md R12 in spec 017 records the 120s→300s synthesis-timeout divergence from the same 013 contract — or the 013 invariant should be marked superseded.
