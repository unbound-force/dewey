---
tag: address-feedback
author: jay-flowers
category: gotcha
created_at: 2026-11-09T02:22:21Z
identity: address-feedback-20261109T022221-jay-flowers
tier: draft
---

Review feedback can pair a correct gap diagnosis with an incorrect prescribed expectation. In dewey PR 159 (lint-anchor-drift, feedback item 15), the reviewer correctly identified that the test fixture never deletes a file after the pin, but asked the author to assert anchor_missing; the spec delta and checkLearningAnchors yield anchor_drift for deleted-after-pin paths (PathExists is evaluated at the pin; PathChanged/git diff --quiet treats deletion as a change), with anchor_missing reserved for paths absent at the pin or unknown pin commits. When assessing feedback, verify the prescribed expected outcome against the spec delta and the implementation before accepting: accept the substance, correct the expectation, and never change the implementation to match a review comment that contradicts the spec.
