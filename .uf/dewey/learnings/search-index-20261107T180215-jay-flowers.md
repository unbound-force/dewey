---
tag: search-index
author: jay-flowers
category: pattern
created_at: 2026-11-07T18:02:15Z
identity: search-index-20261107T180215-jay-flowers
tier: draft
---

When adding a new precision signal to a return value, pin BOTH the degenerate case and the positive case: assert the signal's value directly in unit tests, and add one integration test exercising every result class it orders. On PR unbound-force/dewey#157 the exact-count return was only covered at exact==0; the three-way ordering [vault exact, store-only, component fills] for exact>0 went unpinned and surfaced as a review-council MEDIUM advisory (review-council on PR unbound-force/dewey#157, testing finding).
