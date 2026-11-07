---
tag: concurrency
author: jay-flowers
category: pattern
created_at: 2026-11-06T17:35:38Z
identity: concurrency-20261106T173538-jay-flowers
tier: draft
---

Assessing a check-then-act race deferral: weigh where the guard lives against where the writes land, and weigh the fix's true scope against process discipline. In dewey PR #156 (store_learning supersession), the already-superseded guard runs at validation time (prepareTemporal), but the writes land later in two legs: linkSuperseded's UpdatePage from the validation-time snapshot, and markSupersededFile's read-modify-write — which re-reads fresh content yet blindly replaces the superseded_by key without re-checking its value, so a fresh read does not close a race whose guard ran earlier. The deferral was sound: impact is metadata-only and documented in the design's Risks section, the review council graded the race LOW, and the real fix needs new store-layer surface (superseded_by is JSON-embedded, so the guard becomes a conditional UPDATE plus a rows-affected check) — which the spec-first mandate and the minimal-changes-after-council-approval rule both route to a follow-up change, not a feedback-fix commit. Conditions that make such a deferral durable: track it in an issue, and scope the follow-up to every write leg — a store-only CAS leaves the markdown backup diverging, and because re-ingestion restores only pages missing from the store, the stale file leg silently wins after graph.db loss. Prefer the CAS over a per-target mutex: it matches the PR-145 pattern (learning/toctou-20261104T173140-jay-flowers), needs no lock registry, and also defeats writers the mutex cannot see.
