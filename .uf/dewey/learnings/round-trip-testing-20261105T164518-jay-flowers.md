---
tag: round-trip-testing
author: jay-flowers
category: pattern
created_at: 2026-11-05T16:45:18Z
identity: round-trip-testing-20261105T164518-jay-flowers
tier: draft
---

When a serialization contract is dual-homed across packages — a writer rendering keys as string literals (tools/learning_temporal.go temporalFields.frontmatter) and a reader parsing the same keys as yaml struct tags (main.go learningFrontmatter) — package-local tests with hand-crafted fixtures verify each half but never the seam, and the halves can drift silently because each suite pins only its own copy. Full extraction cannot fix this when one half is yaml struct tags (Go tags are string literals and cannot reference shared constants), so the correct pinning mechanism is a single round-trip test: write through the production writer (StoreLearning), read through the production reader into a fresh store (reIngestLearnings over the directory the writer produced), and assert the restored values equal the supplied values. When the reader is unexported, the test lives in the reader's package (main_test.go). This generalizes the dewey drift-guards pattern (learning/drift-guards-20261104T005638-jay-flowers) to tag-based mappings where constant-sharing is impossible.
