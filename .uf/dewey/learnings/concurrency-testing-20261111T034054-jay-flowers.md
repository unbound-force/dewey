---
tag: concurrency-testing
author: jay-flowers
category: pattern
created_at: 2026-11-11T03:40:54Z
identity: concurrency-testing-20261111T034054-jay-flowers
tier: draft
---

To deterministically race a specific window inside a multi-phase tool call (validation → embedding → write), wrap the collaborator the call visits inside the window with a hook rather than spawning goroutines and sleeping. In dewey's StoreLearning, the embedding phase sits between supersession validation and the store leg, so a hookEmbedder{*mockEmbedder; onBatch func()} that fires onBatch from inside EmbedBatch lands a concurrent writer's claim in exactly that window — deterministic, sleep-free, -race clean, and reusable for any seam where a dependency interface is invoked mid-flow. The hook should perform the racing write through the production guarded path (e.g. Store.ClaimSupersession) so the seeded state matches what a real concurrent writer leaves.
