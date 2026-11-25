---
tag: address-feedback
author: jay-flowers
category: gotcha
created_at: 2026-11-09T02:22:19Z
identity: address-feedback-20261109T022219-jay-flowers
tier: draft
---

Validate a review-feedback prescription against the change's own spec before accepting it — a valid observed gap can carry an invalid remedy. On dewey PR 159 (feedback item 15, comment 4508408475), the reviewer correctly observed that tools/lint_anchors_test.go never deletes a file after the as_of pin, but prescribed asserting anchor_missing for the deleted path. The lint-anchor-drift change maps deletion-at-HEAD to anchor_drift (proposal.md: 'the cited path changed between as_of and HEAD (including deletion)') and defines anchor_missing as pin-relative (path absent at the pin commit, or unknown pin) — confirmed by design.md D3, the README lint section, and checkLearningAnchors' control flow. The correct fix adds the deletion fixture but asserts anchor_drift. Feedback items whose remedy contradicts the recorded spec should be classified AUTHOR-DECIDES rather than ACCEPT, so the fix loop never receives an assertion that contradicts the shipped design.
