---
tag: review-feedback
author: jay-flowers
category: pattern
created_at: 2026-11-09T02:22:24Z
identity: review-feedback-20261109T022224-jay-flowers
tier: draft
---

Review feedback that prescribes a specific expected value can be wrong even when its gap observation is right. On dewey PR #159, feedback item 15 correctly identified that the anchor test fixture never deletes a file after the pin, but prescribed asserting anchor_missing where the frozen spec mandates anchor_drift. When addressing review feedback, validate the prescribed assertion against the spec and the implementation before writing the test: the gap diagnosis and the prescribed fix are independently valid or invalid.
