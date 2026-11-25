---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-07T16:47:16Z
identity: test-quality-20261107T164716-jay-flowers
tier: draft
---

When testing query expansion that decomposes compound terms (or any term rewriting), lone-compound and all-plain queries leave the preservation of un-rewritten terms unpinned. Include a mixed query (one compound + one plain term) with a decoy block that matches every expanded component but lacks the plain term, and assert the decoy stays excluded — a regression that drops preserved terms broadens pass-2 results without failing any other test. Found reviewing dewey PR #157 (vault/search_index.go expandCompoundTerms, issue #149).
