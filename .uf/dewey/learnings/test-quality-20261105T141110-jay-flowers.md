---
tag: test-quality
author: jay-flowers
category: gotcha
created_at: 2026-11-05T14:11:10Z
identity: test-quality-20261105T141110-jay-flowers
tier: draft
---

A negative-validation test case that names a specific rejection branch must seed the fixture state that makes that branch reachable; otherwise it silently collapses onto a sibling branch and its label overstates coverage. In dewey PR #156 (learning temporal validity), TestStoreLearning_InvalidTemporalInputWritesNothing's "non-learning tgt" case passed Supersedes: "README" against newTestStore's empty in-memory store, so GetPage("learning/README") returned nil and the case exercised the same not-found branch
