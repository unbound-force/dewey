---
tag: doctor-tests
author: jay-flowers
category: gotcha
created_at: 2026-11-07T16:14:06Z
identity: doctor-tests-20261107T161406-jay-flowers
tier: draft
---

In dewey, doctor check status renders only as emoji: printCheck (cli.go:1162-1178) formats `emoji + %-20s name + description` and never prints the words PASS/FAIL/WARN — those are internal marker enum values. Tests asserting dewey doctor output must therefore assert the rendered marker (e.g., "✅ config", "❌ graph.db") to verify a check's outcome; substring assertions on descriptions alone pass even when the wrong status marker is emitted. The established marker-assert convention lives at "✅ vault"/"❌ graph.db" (cli_test.go:2986/2992) plus seven further ✅/❌ sites in cli_test.go. Caveat: the Synthesis Layer Vertex tests (TestDoctorCmd_SynthesisVertexConfigured/Misconfigured, cli_test.go:3220/3277) share the same marker-assertion gap, so absence of a marker assertion there is not evidence of a contrary convention.
