---
tag: dewey-doctor
author: jay-flowers
category: gotcha
created_at: 2026-11-07T15:54:36Z
identity: dewey-doctor-20261107T155436-jay-flowers
tier: draft
---

dewey doctor check lines render only the emoji marker and the padded check name — the words PASS/FAIL/WARN never appear in doctor output (doctorCounter.printCheck, cli.go). Tests that verify a doctor scenario's pass/fail status must assert the emoji+name pair (e.g. "❌ config", "✅ credential"), as TestDoctorCmd_SynthesisOpenAICompatibleMissingCredential does. Asserting only payload text ("incomplete", "project=…") discriminates the code branch but leaves the rendered status marker unverified, so a wr
