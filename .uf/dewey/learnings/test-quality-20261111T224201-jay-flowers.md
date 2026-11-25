---
tag: test-quality
author: jay-flowers
category: gotcha
created_at: 2026-11-11T22:42:01Z
identity: test-quality-20261111T224201-jay-flowers
tier: draft
---

When one layer admits names case-insensitively (NOCASE index lookups, EqualFold prefix checks) and a downstream layer strips prefixes case-sensitively (strings.TrimPrefix), case-variant names pass admission but render unnormalized. Per-layer tests are insufficient: a store test can pin NOCASE admission and a tools test can pin lowercase stripping while the composed pipeline still mismatches on names like 'Learning/Caps'. Tests must push case-variant names through the full pipeline — admission plus rendering — and assert the final rendered identity.
