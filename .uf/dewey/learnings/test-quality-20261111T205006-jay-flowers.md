---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-11T20:50:06Z
identity: test-quality-20261111T205006-jay-flowers
tier: draft
---

Non-mutation tests stay meaningful when they assert the acting-worthy outcome occurred before comparing before/after snapshots. dewey's TestVerify_NonMutation and TestVerify_StoreRoundTripNonMutation first assert the pass really produced drifted and missing results — exactly the outcomes a mutating flow (lint --fix, promote) would act on — and only then compare full-store and vault snapshots byte-for-byte; the round-trip audit also runs the failed-pass path so the error route's reads are audited too. The pattern closes the vacuous-pass failure mode: a test that proves nothing changed cannot silently pass because nothing happened.
