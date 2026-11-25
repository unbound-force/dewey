---
tag: provider-architecture
author: jay-flowers
category: pattern
created_at: 2026-11-02T23:14:26Z
identity: provider-architecture-20261102T231426-jay-flowers
tier: draft
---

Pattern for extending a mirrored two-package provider architecture with a third backend: keep per-package ProviderConfig and factories (preserving the established configuration-independence decision), but extract the security-sensitive transport policy — credential resolution, retry, redaction, availability probing — into a new shared leaf package that imports only the standard library plus the logger, so the policy cannot drift between capabilities. Record any leaf-dependency invariant divergences (e.g., llm importing another Dewey package) in research.md with alternatives considered rather than silently extending them, and restate the invariant's status in the importing package's GoDoc. Observed in dewey specs/017: the openaicompat package sits beside unchanged Embedder/Synthesizer contracts, with research.md R13 documenting the extension of the 013 leaf-dependency invariant and R7 documenting the deliberate three-way retry-policy duplication justified by backward compatibility.
