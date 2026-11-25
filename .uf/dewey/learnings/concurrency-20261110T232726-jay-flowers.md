---
tag: concurrency
author: jay-flowers
category: pattern
created_at: 2026-11-10T23:27:26Z
identity: concurrency-20261110T232726-jay-flowers
tier: draft
---

Serializing each write leg of a two-leg store+file update under a shared mutex does not make the pair atomic. A writer whose store leg passed its in-closure guard can still run its file leg after another writer completed both legs in between, overwriting the newer value from stale knowledge — per-leg locking prevents mid-write interleaving (dropped keys) but not stale-knowledge overwrites. In dewey's supersession/lint-fix flows (learningPropsMu held separately over the store leg and the file leg), markStaleUnverified's file leg can revert validity: superseded written by a supersession that landed between lint's two legs, diverging the durable markdown backup from the store — and because reIngestLearnings restores only pages missing from the store, the stale file wins after graph.db loss. Fix shapes: hold ONE mutex acquisition across both legs per caller, or re-check the guarded value at file-write time (the same value-guard discipline the winning writer's leg uses). Rule of thumb: for every guarded two-leg write, either the guard's validity must span both legs (one lock hold) or each leg must independently re-verify.
