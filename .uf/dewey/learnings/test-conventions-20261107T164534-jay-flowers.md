---
tag: test-conventions
author: jay-flowers
category: pattern
created_at: 2026-11-07T16:45:34Z
identity: test-conventions-20261107T164534-jay-flowers
tier: draft
---

Ordered fallback chains need per-pair coexistence fixtures. When a resolver tries candidates in sequence (vault findStorePage: exact name, then compiled/<name>, then learning/<name>), a scenario table that exercises each namespace in isolation leaves the ordering mutation-permissible — reordering the chain to try prefixes before the exact lookup passed the entire vault suite because no row stored a plain-named store page alongside compiled/<same-bare-name>. Pin each adjacent precedence pair with one fixture that stores both candidates and asserts the winner. The spec's trigger phrasing ("misses ... the exact store lookup") states the contract, but only the coexistence fixture enforces it.
