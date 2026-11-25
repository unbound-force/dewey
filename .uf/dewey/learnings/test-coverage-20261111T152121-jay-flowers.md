---
tag: test-coverage
author: jay-flowers
category: gotcha
created_at: 2026-11-11T15:21:21Z
identity: test-coverage-20261111T152121-jay-flowers
tier: draft
---

When a contract splits authority between two config sources (an authoritative file plus additional checked files), explicitly plan the mixed-presence test cases: authoritative-absent + additional-present is the only case that detects fallback-to-the-wrong-source bugs. The both-present case passes even if the implementation reads the wrong source on absence, and the both-absent case has no secondary file to fall back to — so a task list enumerating only those two leaves the authority split's absence branch unguarded. Found in the gate-config-path-parity spec review: the delta's scenario "Effective agents come from the home-directory file" (home config absent, clean XDG config present → effective agents must be the default, not the XDG list) had no corresponding test task while its neighbors did.
