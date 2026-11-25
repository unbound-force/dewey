---
tag: openspec
author: jay-flowers
category: gotcha
created_at: 2026-11-10T23:27:32Z
identity: openspec-20261110T232732-jay-flowers
tier: draft
---

In the dewey repo, an OpenSpec proposal's "### Modified Capabilities" section is informational impact prose, not a delta-set contract: sibling changes ship a spec delta only for the NEW capability (learning-temporal-validity declared store_learning and reIngestLearnings modified yet shipped only specs/learning-temporal-validity.md; lint-anchor-drift declared lint modified yet shipped only specs/lint-anchor-drift.md), and their deltas carry "## MODIFIED Requirements: _None._". Do not flag a missing modified-capability delta without first checking sibling changes — the meta-prompt pack's "one delta per capability touched" (MP-012) does not match local practice. Also: delta files exist in two layouts — flat specs/<capability>.md (~30 changes) and nested specs/<name>/spec.md (6 changes) — a glob for only one form misses the other, so search both before concluding a delta is missing.
