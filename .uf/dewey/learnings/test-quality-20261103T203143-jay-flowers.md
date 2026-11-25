---
tag: test-quality
author: jay-flowers
category: gotcha
created_at: 2026-11-03T20:31:43Z
identity: test-quality-20261103T203143-jay-flowers
tier: draft
---

Exported helper functions whose only contract tests live in a consuming package report 0% coverage in package-local go test -coverprofile runs (no -coverpkg), so gaze flags them uncovered even when both branches are exercised cross-package. Dewey embed/llm UnavailableReasonOf hit this in PR #144: reasoner branch tested via tools/ mocks, fallback via newMockEmbedder(false), yet gaze showed 0%. Fix by adding a small direct unit test in the defining package, or note the attribution artifact where gaze output is read.
