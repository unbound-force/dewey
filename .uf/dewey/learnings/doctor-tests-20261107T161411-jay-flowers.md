---
tag: doctor-tests
author: jay-flowers
category: pattern
created_at: 2026-11-07T16:14:11Z
identity: doctor-tests-20261107T161411-jay-flowers
tier: draft
---

dewey doctor output renders check status only as emoji — doctorCounter.printCheck (cli.go:1162-1177) maps PASS→✅, WARN→⚠️, FAIL→❌, INFO→ℹ️; the words PASS/FAIL never appear in output. Doctor tests that assert only description substrings (e.g. "project=", "incomplete") leave the status dimension unverified: flipping the status argument at a printCheck call site keeps the tests green while doctor displays the wrong verdict. When testing doctor output, assert the rendered marker plus check name — strings.Contains(section, "✅ config") / "❌ config" — following the cli_test.go:2986/2992 precedent ("✅ vault", "❌ graph.db"). Acceptance scenarios phrased "MUST report PASS/FAIL" map to these marker assertions, since the emoji is the status's only rendered form.
