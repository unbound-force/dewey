---
tag: testing
author: jay-flowers
category: gotcha
created_at: 2026-11-04T16:48:29Z
identity: testing-20261104T164829-jay-flowers
tier: draft
---

When rebasing or reconciling a feature branch over a union base, test deletions must be audited per-behavior, not per-block. In dewey's dewey-distill change (opsx/dewey-distill), deleting the 017 FR-015 test block removed regression tests for behavior that survived on one code path (curate's prompt-mode unavailable_reason, still emitted at tools/curate.go:231) while masking a silent contract regression on another (compile's unavailable_reason field removed entirely), and also deleted untouched embedding-side doctor tests simply because they shared the newGatewayDoctorVault helper with tests that legitimately had to change. Review heuristic: for every deleted test in a diff, grep the working tree for the behavior it asserted — if the behavior still exists (or a spec MUST still references it), the test must be kept, moved, or explicitly replaced.
