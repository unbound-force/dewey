---
tag: go-testing
author: jay-flowers
category: pattern
created_at: 2026-11-11T01:10:30Z
identity: go-testing-20261111T011030-jay-flowers
tier: draft
---

Test-quality pattern: deriving a candidate set by regexing source text (for example `Name:\s+"([a-z][a-z0-9_]*)"` over server.go) only fails when zero matches are found, so a registration-form refactor silently shrinks the set and weakens the invariant the test claims to enforce. Prefer an exported registry (a slice or map the test can import) and assert its size, or have the regex helper fail when the derived count does not match a known expected count.
