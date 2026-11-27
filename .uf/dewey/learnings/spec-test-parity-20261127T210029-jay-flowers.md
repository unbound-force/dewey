---
tag: spec-test-parity
author: jay-flowers
category: gotcha
created_at: 2026-11-27T21:00:29Z
identity: spec-test-parity-20261127T210029-jay-flowers
tier: draft
as_of: "dewey@f77093acbbe0468d0a190f8705f89f51eec3b341"
---

When a deterministic gate pins an external tool's contract, the normalization semantics must be stated identically in the spec, the design, the code, and the test. In dewey PR #286 (distill-auto-runner-neutral), the delta spec said "stripping one leading /" and the design named strings.TrimPrefix, while the code used strings.TrimLeft and a "double slash" test case pinned the all-slashes behavior with an opposite doc comment — six artifacts carrying two contracts for the gate that is the sole widening authority for the distiller allowlist. A test comment is not a substitute for requirement text: whoever later "fixes" the code to match the spec will break a test whose failure then reads as drift rather than correctness. Reconcile the wording in the same PR that introduces the contract.
