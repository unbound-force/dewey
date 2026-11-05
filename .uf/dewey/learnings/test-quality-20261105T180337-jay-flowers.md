---
tag: test-quality
author: jay-flowers
category: gotcha
created_at: 2026-11-05T18:03:37Z
identity: test-quality-20261105T180337-jay-flowers
tier: draft
---

Adding a production-default subprocess resolver to an existing constructor (dewey PR #156: NewLearning now sets asOfResolver = productionAsOfResolver(vaultPath)) silently changes the hermeticity of every pre-existing test that constructs through it. The old tests keep passing because the resolver degrades gracefully, but each StoreLearning call now spawns `git -C <tmpdir> rev-parse`, and git's repository discovery walks parent directories — a runner whose TMPDIR sits inside a git work tree gets different as_of behavior.
