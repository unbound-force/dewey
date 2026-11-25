---
tag: documentation
author: jay-flowers
category: pattern
created_at: 2026-11-11T15:17:39Z
identity: documentation-20261111T151739-jay-flowers
tier: draft
---

When a spec retires a behavioral claim, the documentation-impact inventory must be built by searching for the claim's text, not by recalling which prose files are canonical. In dewey, "resolves contradictions temporally (newer wins)" appears in README lines 431 and 757 AND in cli.go:2199-2200 as the `dewey compile` Long help; specs that correct the README occurrences routinely miss the CLI help, leaving `dewey compile --help` contradicting the README. The mechanical check: grep the falsified phrase plus its synonyms across README, AGENTS.md, and cli.go command Long/Short strings, then task every hit (or record why a hit is out of scope). The same applies in reverse for new capabilities: a new lint kind must be added to the enumerating `dewey lint` Long help (cli.go:2435) and to AGENTS.md's lint CLI row, not only README.
