---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-07T20:46:22Z
identity: test-quality-20261107T204622-jay-flowers
tier: draft
---

In CLI output-rendering tests, when a FAIL branch uses an early return before a shared success line (if err != nil { printCheck(FAIL); return }; printCheck(PASS)), assert the ABSENCE of the success marker in FAIL-path output, not just the presence of the failure marker — a dropped return emits both markers and a "contains FAIL" assertion alone would still pass. Marker-exclusivity assertions turn rendering early-returns into a tested contract (pattern shipped in dewey doctor Vertex embedding tests, cli_test.go TestDoctorCmd_EmbeddingVertexMisconfigured, opsx/doctor-vertex-embedding round 4).
