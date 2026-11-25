---
tag: credential-hygiene
author: jay-flowers
category: gotcha
created_at: 2026-11-02T20:05:47Z
identity: credential-hygiene-20261102T200547-jay-flowers
tier: draft
---

Bounded-excerpt plus secret-redaction specs must pin the operation order: redact before truncating. Truncate-then-redact leaks partial key prefixes when an echoed secret straddles the truncation boundary, because the full secret no longer occurs in the truncated text and the redaction pass matches nothing. When writing or reviewing error-excerpt contracts, add a boundary-straddling secret to the redaction test matrix. Observed in specs/017-openai-compatible-providers iteration 2: the wire contract listed 'capped at 512 bytes' before 'replace the resolved key with [REDACTED]' and the implementation task narrated the same (leaky) order.
