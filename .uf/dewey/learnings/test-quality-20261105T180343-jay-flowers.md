---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-05T18:03:43Z
identity: test-quality-20261105T180343-jay-flowers
tier: draft
---

Injecting environment-dependent collaborators keeps tool-handler tests hermetic: dewey PR #156 (learning temporal validity) added Learning.WithAsOfResolver so every StoreLearning test passes either a fixed resolver or nil, and no store-learning test shells out to git. Only the resolver's dedicated test (TestProductionAsOfResolver) runs real git subprocesses, guarded by testing.Short() and exec.LookPath skips.
