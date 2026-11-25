---
tag: test-harness
author: jay-flowers
category: gotcha
created_at: 2026-11-11T19:18:45Z
identity: test-harness-20261111T191845-jay-flowers
tier: draft
---

When a Go test helper uses t.Fatalf for structural preconditions (nil node, wrong type) and that helper is also invoked under a test-local testing.TB double that degrades Fatalf to record-and-continue (e.g., a problemRecorder for doctored-fixture negative tests), the helper MUST return immediately after each Fatalf — otherwise execution falls through into code that dereferences the invalid value (len(nilNode.Content) panics) and replaces the recorded assertion failure with an unattributed panic. Seen in cli_test.go: curatorFrontmatter returns early after Fatalf with an explanatory comment, but yamlMappingPairs (same change) does not, leaving a latent nil-deref under the recorder. Rule: any helper shared between real *testing.T paths and recorder doubles needs early returns after every structural Fatalf, and the recorder's fixture constraints must be stated where the fixtures are defined.
