---
tag: test-quality
author: jay-flowers
category: gotcha
created_at: 2026-11-10T15:33:22Z
identity: test-quality-20261110T153322-jay-flowers
tier: draft
---

Path-escape regression tests must derive the canary's location from the same path join the production code performs (vault + workspace dir + learnings + forged identity), never from a hand-counted '../' chain written independently in the fixture. In dewey PR #159's TestMarkStaleUnverified_RejectsUnsafeIdentity (tools/lint_anchors_test.go), the fixture joined vault + '.uf/dewey' + '../../escape.md' — placing the canary at vault/escape.md — while the production escape for identity '../../escape' resolves to v
