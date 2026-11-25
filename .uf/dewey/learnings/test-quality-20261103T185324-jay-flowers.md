---
tag: test-quality
author: jay-flowers
category: gotcha
created_at: 2026-11-03T18:53:24Z
identity: test-quality-20261103T185324-jay-flowers
tier: draft
---

A test named after a multi-value requirement must enumerate every value in the group — especially the value that bounds lock hold time. In dewey's openaicompat package, Available() holds the client write mutex while probe() runs GET /models under context.WithTimeout(probeTimeout); the one-second cap is the only bound on both the mutex hold and the serialization concurrent callers see (research.md R3 trade-off). TestClient_CapabilityTimeouts pinned FR-025's embedding 30s and synthesis 300s httpClient.Timeout values but omitted the 1s probeTimeout, and no availability test exercises a hanging gateway — so raising the constant or dropping the WithTimeout passed the entire suite while extending semantic-tool availability checks behind a hung gateway toward the 30s/300s capability client timeouts (SC-004 violation). Pin lock-bounding timeout constants with in-package constant assertions alongside their sibling capability timeouts, preferring the deterministic assertion over timing-based behavioral tests, matching the package's established white-box seams.
