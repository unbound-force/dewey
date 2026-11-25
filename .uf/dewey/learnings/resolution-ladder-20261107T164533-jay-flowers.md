---
tag: resolution-ladder
author: jay-flowers
category: pattern
created_at: 2026-11-07T16:45:33Z
identity: resolution-ladder-20261107T164533-jay-flowers
tier: draft
---

A resolution ladder (exact name first, then prefix shorthands such as compiled/<name> and learning/<name>) is not order-locked by table-driven tests that give each case a single existing candidate — reordering the ladder passes the suite because no fixture makes two candidates coexist. Lock each ordering boundary with one coexistence fixture: store an entity under both the exact name and the prefixed shorthand and assert the higher-priority candidate is returned. Seen in dewey PR #163 (read-tools-bare-names): TestGetPage_BareNameResolvesInternalStorePages pinned compiled-over-learning and case-folding, but a findStorePage reorder to prefixes-first passed the whole suite until review flagged the missing exact-vs-prefix fixture.
