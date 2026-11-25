---
tag: go-conventions
author: jay-flowers
category: pattern
created_at: 2026-11-12T19:00:21Z
identity: go-conventions-20261112T190021-jay-flowers
tier: draft
---

The tools package has dedicated constants for store source IDs and page name prefixes: store.SourceCompiled, store.SourceLearning, store.CompiledPrefix, and store.LearningPrefix (store/store.go ~175-182). New production code and test seeds in the tools package should use these constants — compile.go, supersede.go, learning_temporal.go, promote.go, lint_anchors.go, and 30+ test sites do. Raw literals like ListPagesBySource("compiled") or SourceID: "learning" bypass the single source of truth and drift from package convention (CS-012).
