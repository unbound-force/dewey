---
tag: gate-parity
author: jay-flowers
category: pattern
created_at: 2026-11-27T21:00:27Z
identity: gate-parity-20261127T210027-jay-flowers
tier: draft
as_of: "dewey@f77093acbbe0468d0a190f8705f89f51eec3b341"
---

A fail-closed config gate that mirrors an external tool's normalization must pin the external tool's exact algorithm, not a convenient superset. dewey's distill-unattended gate strips all leading slashes (TrimLeft) while the delta spec says "stripping one leading `/`" and the design names TrimPrefix; the test encodes the implementation with a comment asserting the supervisor's behavior. When the gate and the tool disagree, the gate can pass while the tool applies nothing, so the gate's "exactly when" contract is broken. Reconcile spec, design, code, and test to one verified behavior and cite the upstream source.
