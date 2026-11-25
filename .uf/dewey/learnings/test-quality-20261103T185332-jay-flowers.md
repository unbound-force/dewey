---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-03T18:53:32Z
identity: test-quality-20261103T185332-jay-flowers
tier: draft
---

When a test exists to pin contract constants (e.g., capability-scoped HTTP timeouts), enumerate the values from the requirement text (FR-NNN), not from the fields visible on the constructed object — constants not surfaced through struct fields (package-level probe/cache constants) get silently skipped, and tests that reference a constant symbolically to arrange state move with the value and pin nothing. Observed in dewey PR #144 (spec 017): TestClient_CapabilityTimeouts pinned FR-025's 30s embedding and 300s synthesis timeouts via httpClient.Timeout but missed the FR-025 one-second probeTimeout and FR-014 thirty-second availabilityCacheTTL; the cheap audit that catches both is a package-wide grep for each constant's name looking for test-file hits that assert a literal rather than use the symbol.
