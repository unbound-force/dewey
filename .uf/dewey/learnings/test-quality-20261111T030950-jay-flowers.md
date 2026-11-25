---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-11T03:09:50Z
identity: test-quality-20261111T030950-jay-flowers
tier: draft
---

Deterministic race-window testing without goroutines or sleeps: when a layered write pipeline has a check-then-act window (validation reads state, the store leg writes later), inject the concurrent write through a callback fired from a stage that already runs inside the window — e.g., an embedder wrapper whose EmbedBatch claims the target between store_learning's validation and its store leg. The interleaving is then guaranteed rather than probabilistic, and the test stays sleep-free under -race (precedent: dewey serialize-supersession-writes, TestStoreLearning_LostClaimWarningNamesWinner / hookEmbedder).
