---
tag: spec-review
author: jay-flowers
category: pattern
created_at: 2026-11-11T15:54:49Z
identity: spec-review-20261111T155449-jay-flowers
tier: draft
---

When an OpenSpec delta rewrites a requirement via MODIFIED, diff the new body against the original requirement's normative elements — refusal conditions, verification steps, output contract, every scenario — because a wholesale rewrite can weaken a requirement without any single line looking deleted. And verify a design's test-compatibility claims by grepping the test file rather than trusting them: check which seam each test injects (configDir vs lookupEnv) and which substrings the assertions pin. In gate-config-path-parity (iteration-2 spec review) both checks passed: all 11 original CA-FR-007 scenarios and every refusal condition survive the multi-path rewrite, gate_test.go:197 asserts the refusal Reason contains "config directory" (which is exactly why task 1.1 pins that phrase), and no existing test exercises absolute divergent XDG-first resolution, so the resolution rewrite breaks no unmodified test.
