---
tag: test-isolation
author: jay-flowers
category: gotcha
created_at: 2026-11-07T17:34:40Z
identity: test-isolation-20261107T173440-jay-flowers
tier: draft
---

A constructor default that probes the environment (subprocess, network, filesystem walk) silently leaks into every pre-existing test that constructs the type without the new injection option — only sites passing an empty path escape via early-return guards. When adding such a default, grep the same package's pre-existing *_test.go for construction sites passing a real path and chain the disabling option (e.g., WithAsOfResolver(nil)) onto them; cover the probe itself in new tests with an injected stub. Precedent: dewey PR #156 — NewLearning's default productionAsOfResolver(vaultPath) (tools/learning.go:86) made six pre-existing learning_test.go constructions spawn git subprocesses per StoreLearning call, even though the PR's own field comment said the resolver was \"Injected so tests never depend on the runner's git state\"; only the newly written tests injected.
