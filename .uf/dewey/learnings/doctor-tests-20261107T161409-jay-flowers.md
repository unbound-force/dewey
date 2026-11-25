---
tag: doctor-tests
author: jay-flowers
category: pattern
created_at: 2026-11-07T16:14:09Z
identity: doctor-tests-20261107T161409-jay-flowers
tier: draft
---

dewey doctor check status renders as emoji only: printCheck (cli.go) maps PASS→✅, WARN→⚠️, FAIL→❌, INFO→ℹ️ and writes `emoji + %-20s name + description` — the words PASS/FAIL never appear in doctor output. Tests that verify doctor sections must assert the rendered marker plus check name (e.g. "✅ config" / "❌ config") to verify status-reporting; asserting description substrings alone leaves marker-inversion regressions undetected, because the description strings ride on the status line and pass content checks regardless of which marker precedes them. Spec scenarios phrased "MUST report PASS/FAIL" map to these rendered markers. Established precedent in cli_test.go: "✅ vault"/"❌ graph.db" (2986/2992), "✅ .uf/dewey/" (3149), "❌ .uf/dewey/" (3370), "✅ dcp.jsonc" (3445). Note: provider-layer gateway tests (TestDoctorCmd_EmbeddingGateway*) assert message content only — do not treat them as the convention when writing new doctor tests.
