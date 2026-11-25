---
tag: regression-testing
author: jay-flowers
category: gotcha
created_at: 2026-11-12T12:39:30Z
identity: regression-testing-20261112T123930-jay-flowers
tier: draft
---

When a change reorders validation checks so a new earlier check precedes a condition an existing test was written to pin, the existing test keeps passing while silently exercising the earlier branch — its fixture no longer reaches the code its name promises (green-but-displaced coverage). gate-config-path-parity hit this: making HOME resolution precede XDG validation in gate.go's resolvePluginConfigDirs displaced pre-existing TestGateAutoAnswer_RelativeXDGRefused, whose fixture sets no HOME. The working pattern: audit every pre-existing test that touches the reordered path, trace which branch its fixture now reaches, and when displaced, add a compensating test whose fixture satisfies the earlier checks so it reaches the original branch — documenting the displacement in the compensating test's comment (TestGateAutoAnswer_RelativeXDGWithValidHomeRefused), and scheduling a rename/annotation of the stale-named test for a change not bound by a leave-existing-tests-unmodified constraint.
