---
tag: spec-review
author: jay-flowers
category: gotcha
created_at: 2026-11-02T20:37:33Z
identity: spec-review-20261102T203733-jay-flowers
tier: draft
---

A quickstart expected outcome must not anchor to a repository-documented threshold that the same changeset deletes. specs/017's quickstart section 4 kept a "repository's local target of 70 percent" contract-coverage expectation whose only source was AGENTS.md's local Gaze example, while task T040 in the same changeset removes that example as stale (CI passes no coverage floor to gaze). Because a later task (T043) must confirm every quickstart expected outcome, the expectation becomes unverifiable exactly when validation runs. When a fix drops an attribution, check whether the retained value still has a source that survives the changeset.
