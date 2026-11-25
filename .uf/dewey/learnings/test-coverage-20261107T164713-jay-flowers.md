---
tag: test-coverage
author: jay-flowers
category: pattern
created_at: 2026-11-07T16:47:13Z
identity: test-coverage-20261107T164713-jay-flowers
tier: draft
---

In multi-pass query expansion (exact pass, then component-expanded pass), tests using only single-form queries cannot detect constraint-dropping regressions in the expansion function: dropping preserved plain terms broadens results silently because lone-compound queries have no plain terms to drop and still pass. Pin the AND semantics with a mixed query — one compound term plus one plain term — asserting both that a block containing the word and the components is returned and that a block containing only the components stays excluded (dewey PR #157, vault/search_index.go expandCompoundTerms).
