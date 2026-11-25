---
tag: go-testing
author: jay-flowers
category: gotcha
created_at: 2026-11-11T20:13:54Z
identity: go-testing-20261111T201354-jay-flowers
tier: draft
---

A testing.TB test double that records Errorf/Fatalf problems instead of failing — used to meta-test guard helpers against doctored fixtures — silently changes Fatalf semantics from fail-now to record-and-continue. Any guard helper that calls t.Fatalf and then dereferences the failed inputs (e.g., iterating a nil *yaml.Node's Content after fataling on a missing mapping) panics under the double instead of recording the problem. Harden shared helpers with an early return after every Fatalf (curatorFrontmatter in dewey's cli_test.go does this; its sibling yamlMappingPairs initially did not), and note that until hardened, the structural fatal paths remain unexercisable by the doctored-fixture matrix.
