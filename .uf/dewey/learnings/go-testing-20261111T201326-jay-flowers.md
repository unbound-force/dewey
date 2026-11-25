---
tag: go-testing
author: jay-flowers
category: gotcha
created_at: 2026-11-11T20:13:26Z
identity: go-testing-20261111T201326-jay-flowers
tier: draft
---

When a Go test helper wraps testing.TB to record failures instead of failing (a problemRecorder for negative-path testing), Fatalf's real semantics (Goexit / FailNow) become part of every guard helper's contract: each Fatalf on a path where subsequent code would be unsafe must be followed by an explicit early return. In dewey's cli_test.go (curator-tool-allowlist, PR #195), curatorFrontmatter defends this with early returns after each Fatalf, but yamlMappingPairs does not — under the recorder, a fixture whose frontmatter omits the tools:/permission:/permission.bash: key (valid YAML) records the diagnostic and then panics on a nil *yaml.Node dereference in the following loop. A documented fixture constraint of "structurally parseable" is insufficient; the real constraint is "every mapping key the guards look up must remain present".
