---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-11T22:41:59Z
identity: test-quality-20261111T224159-jay-flowers
tier: draft
---

Over-limit fixtures are required to cover presentation-bound logic. Tests that only assert the under-bound output shape (e.g., 'sources_omitted is absent', no truncation markers) leave cap counters, '…and N more' markers, omitted-count arithmetic, and truncation-at-limit branches regression-unprotected. When a function under test has presentation limits — max entries, max reported sources, rune or byte truncation — always add a fixture that exceeds every limit and assert the capped counts, marker text, and truncation outcomes exactly. Seen in the dewey verify tool review (PR #197): TestVerify_CompiledAggregatesSources pinned only the under-bound shape while the presentEvidence/boundSources/truncateLocator cap logic had zero coverage despite reachable over-limit paths.
