---
tag: test-quality
author: jay-flowers
category: gotcha
created_at: 2026-11-11T20:26:20Z
identity: test-quality-20261111T202620-jay-flowers
tier: draft
---

When a deterministic gate's environment-resolution precedence chain is rewritten so a new required variable (e.g. HOME) is checked before an existing one (e.g. XDG_CONFIG_HOME), pre-existing tests that inject only the old variable silently change meaning: they keep passing via the new short-circuit while their names still claim the old branch. After reordering a precedence chain, audit every existing env-injection test's actual reach, restore stranded branch coverage with a new test that maps the new variable validly, and document the short-circuit in the compensating test's comment rather than editing the old test (which doubles as the byte-identical regression guard). Pattern shipped in dewey gate-config-path-parity: TestGateAutoAnswer_RelativeXDGRefused was stranded by the HOME-first resolution rewrite; TestGateAutoAnswer_RelativeXDGWithValidHomeRefused restored the IsAbs(XDG_CONFIG_HOME) branch.
