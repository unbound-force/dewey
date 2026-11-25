---
tag: provider-architecture
author: jay-flowers
category: pattern
created_at: 2026-11-02T22:17:56Z
identity: provider-architecture-20261102T221756-jay-flowers
tier: draft
---

To extend a stable provider contract without breaking existing implementers or consumers, define the new behavior as a separate optional capability interface beside the frozen core interface, and give each owning package a single extraction helper. Observed in specs/017: `UnavailableReasoner` (single UnavailableReason() string method) sits beside the unchanged `Embedder`/`Synthesizer` interfaces, is implemented only by the new OpenAI-compatible providers, and `embed.UnavailableReasonOf`/`llm.UnavailableReasonOf` encapsulate the type assertion once so call sites in tools/, cli.go, and main.go never duplicate it. This preserves backward compatibility for inherited tool contracts while letting diagnostics surface provider-specific degradation reasons.
