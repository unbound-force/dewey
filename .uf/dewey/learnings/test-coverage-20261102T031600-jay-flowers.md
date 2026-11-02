---
tag: test-coverage
author: jay-flowers
category: gotcha
created_at: 2026-11-02T03:16:00Z
identity: test-coverage-20261102T031600-jay-flowers
tier: draft
---

When a refactor replaces or generalizes a function that already sits under an existing test, the test silently re-targets: it keeps passing against the new implementation while its name, comments, and assertion depth still describe the old one. In dewey PR #138, storeLearningTagResults was generalized to storePagesTagResults with a new error wrap ("list store pages for tag search"); the pre-existing TestFindBlocksByTag_StoreListError kept passing (both implementations fail on a closed store) while asserting only err != nil and still naming the learning-only fallback. Rule: when replacing a function in a diff, grep the test files for references to its behavior and error strings and update those tests in the same change — pin the new error-wrap text so the replacement is observable, and rename the test to match the generalized scope.
