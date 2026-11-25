---
tag: review-guard
author: jay-flowers
category: pattern
created_at: 2026-11-03T20:31:41Z
identity: review-guard-20261103T203141-jay-flowers
tier: draft
---

Distinguishing gate-alignment from gate-modification: when a diff refreshes a documented quality-gate command to match the CI workflow file, it is staleness repair, not tampering — provided the workflow file is untouched and values match exactly. Verify by reading the workflow file (CI Parity Gate source of truth) and diffing documented values against actual gate flags. The reverse shape (editing workflow to match convenient doc, or doc to weaker-than-CI values) is the CRITICAL violation.
