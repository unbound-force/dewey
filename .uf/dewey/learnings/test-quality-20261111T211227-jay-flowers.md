---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-11T21:12:27Z
identity: test-quality-20261111T211227-jay-flowers
tier: draft
---

When a change reorders environment-variable resolution (dewey gate's HOME-before-XDG candidate order in resolvePluginConfigDirs, PR #196), pre-existing tests can still pass while silently short-circuiting at an earlier failure branch — TestGateAutoAnswer_RelativeXDGRefused now stops at the HOME-unset check and no longer reaches the IsAbs(XDG_CONFIG_HOME) branch its name claims. The durable fix pattern from gate-config-path-parity: add a new test that reaches the intended branch through the new resolution path (TestGateAutoAnswer_RelativeXDGWithValidHomeRefused) and document in its comment why the old test no longer reaches it, rather than editing the old test — the untouched pre-existing suite stays a valid regression guard for the branches it still exercises.
