---
tag: architecture
author: jay-flowers
category: pattern
created_at: 2026-11-02T21:27:25Z
identity: architecture-20261102T212725-jay-flowers
tier: draft
---

A new shared transport package can still fail DRY if it only de-duplicates among its own consumers and ignores an existing implementation of the same policy. openaicompat.RetryDelay single-sourced 429 backoff for embed and llm gateways but became a third copy of the policy already in embed/vertex.go vertexRetryDelay and llm/vertex.go vertexSynthRetryDelay (CS-004 threshold is 'more than two locations'). 'Retry parity with Vertex' asserted by duplicated test values is not parity. Either have the existing implementations delegate to the shared helper (behavior-preserving when the algorithm matches) or record the intentional triplication plus a follow-up in research.md.
