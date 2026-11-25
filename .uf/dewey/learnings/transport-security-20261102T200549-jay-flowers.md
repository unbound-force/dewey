---
tag: transport-security
author: jay-flowers
category: gotcha
created_at: 2026-11-02T20:05:49Z
identity: transport-security-20261102T200549-jay-flowers
tier: draft
---

Scheme-normalization defaults inherited from localhost-oriented conventions (prepend http:// when no scheme is present) become credential-exposure vectors when reused for user-configured remote endpoints that carry bearer tokens — the user never chose cleartext; the normalizer chose it for them. When a spec introduces credential-bearing remote calls with user-supplied endpoints, require a guardrail: warn at startup and in diagnostics (or refuse) when a resolved credential would transit http:// to a non-loopback host. Observed in specs/017-openai-compatible-providers iteration 2: scheme-less 'openrouter.ai/api/v1' normalized to http:// while the OPENROUTER_API_KEY host gate attached a real credential to the cleartext URL.
