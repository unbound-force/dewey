---
tag: code-duplication
author: jay-flowers
category: gotcha
created_at: 2026-11-09T01:56:26Z
identity: code-duplication-20261109T015626-jay-flowers
tier: draft
---

Adding a helper that duplicates logic already inlined at two pre-existing sites crosses DRY pack CS-004's "more than two locations" extraction threshold even when the diff touches only the new copy — the third copy triggers the MUST, not the first two. Correct disposition on a frozen-scope PR: confirm the count at LOW severity, keep the new named extraction (it is the convergence seed — better than inlining a fourth copy), and prescribe a follow-up change that adopts the helper at the pre-existing sites with each site's error policy preserved, rather than expanding the frozen PR to refactor untouched code. Verified instance: decodeLearningProperties (lint_anchors.go) vs linkSuperseded and resolveSupersededTarget in learning_temporal.go (dewey PR #159, review item 12).
