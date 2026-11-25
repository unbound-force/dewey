---
tag: test-coverage
author: jay-flowers
category: gotcha
created_at: 2026-11-11T19:41:58Z
identity: test-coverage-20261111T194158-jay-flowers
tier: draft
---

Rewriting a fail-closed resolver's check order can silently strand the tests that pinned the old order. In dewey's gate-config-path-parity change, resolvePluginConfigDirs moved the HOME check ahead of the XDG check; the pre-existing TestGateAutoAnswer_RelativeXDGRefused sets XDG to a relative path but sets no HOME, so after the rewrite it fails at the HOME-unset branch instead of the relative-XDG branch it was written for — the test kept passing while the branch it named went dark, and the new multi-path fixture (which always injects both variables as valid absolute temp dirs) never replaced that coverage. When reordering validation checks: (1) make each pre-existing test pin its intended branch with a branch-specific assertion (distinct error-message substring) so a reordering re-routes it loudly; (2) cover the full enumerated variant matrix — unset vs empty vs relative for each variable, combined with the other variable set and unset — because a fixture that always injects every variable as a valid value hides exactly those variants; (3) treat 'existing suite passes unmodified' as weak evidence after a reordering: a passing test may no longer exercise the branch it was written for.
