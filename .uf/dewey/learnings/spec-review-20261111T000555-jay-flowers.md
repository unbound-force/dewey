---
tag: spec-review
author: jay-flowers
category: gotcha
created_at: 2026-11-11T00:05:55Z
identity: spec-review-20261111T000555-jay-flowers
tier: draft
---

In OpenSpec spec review, a "no regression" or compatibility scenario that varies only dependency presence (for example "no plugin and no supervisor") still needs to state every artifact prerequisite enforced by sibling requirements. Otherwise two acceptance criteria prescribe opposite outcomes for the same fixture state, and a test written from one fails the other. Also check whether scaffolding that "never overwrites" means a change never reaches existing adopters, and require an explicit upgrade or migration task.
