---
tag: test-quality
author: jay-flowers
category: gotcha
created_at: 2026-11-19T05:18:54Z
identity: test-quality-20261119T051854-jay-flowers
tier: draft
as_of: "dewey@8606521c6557cd1a9d5cd05d2b3eff1053402064"
---

When a change's safety story is fail-closed semantics, enumerate every failure seam named in the requirement PROSE — not just the GIVEN/WHEN/THEN scenarios — and check each has a fault-injection test. Scenario lists typically cover the obvious faults (scan error, per-operation failure) while prose clauses carry additional seams (e.g., a page-listing error in dewey PR #233's LR-FR-003/CKS-FR-003) that silently end up with zero coverage because no scenario names them. The seam without a test is the one that regresses without a red build. Cheap deterministic injection for listing errors: call the helper on a closed store (queries return "sql: database is closed") with a buffer-backed logger and assert the skip warning fires.
