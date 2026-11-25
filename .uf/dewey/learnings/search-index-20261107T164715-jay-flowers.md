---
tag: search-index
author: jay-flowers
category: pattern
created_at: 2026-11-07T16:47:15Z
identity: search-index-20261107T164715-jay-flowers
tier: draft
---

Two-pass search rewrites that preserve some query terms verbatim need a mixed-query regression test. In dewey's vault/search_index.go, expandCompoundTerms replaces compound terms with their components but preserves non-compound terms so pass 2 keeps whole-query AND semantics (design D2 of the search-compound-terms change, issue #149). The change's initial tests used only lone-compound queries ("KB-REVIEW-ADJUD", "foo-bar") and all-plain queries ("KB REVIEW ADJUD 01"), so a regression that dropped the preserved terms — silently broadening search results — would have passed every test (found in PR #157 review, LOW test-coverage item). The pinning test: a query mixing a compound term with a plain word, plus a negative assertion that a block matching only the components stays excluded. General pattern: when a rewrite pipeline has a preserved-term path and a rewritten-term path, test them together, not just each in isolation.
