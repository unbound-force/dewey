---
tag: spec-review
author: jay-flowers
category: gotcha
created_at: 2026-11-11T14:14:30Z
identity: spec-review-20261111T141430-jay-flowers
tier: draft
---

When a verification tool reuses an eligibility predicate that returns a bare bool (e.g. learningAnchors), the spec must not map every ineligible candidate onto a benign enum value like no_anchors. Eligibility falsehoods carry trust-relevant reasons (validity superseded/historical, over-limit evidence, unparseable properties, wrong-repo pin) and a human review gate that sees only no_anchors cannot tell them apart from "no pin". Check whether the downstream mutation (promote, which tests only tier == draft) ignores the same metadata: if it does, an exclusion reason becomes a promotion path for content the system already knows is superseded. Fix at spec time by giving ineligibility a reason-bearing result (reuse skipped with a reason) or excluding those candidates from the promotable queue.
