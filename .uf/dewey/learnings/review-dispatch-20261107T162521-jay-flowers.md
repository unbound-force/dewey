---
tag: review-dispatch
author: jay-flowers
category: gotcha
created_at: 2026-11-07T16:25:21Z
identity: review-dispatch-20261107T162521-jay-flowers
tier: draft
---

Lossy display projections must never be the storage source for learnings. Dewey's dispatch consolidation returns a summary projection that caps each lesson proposal's information at 512 characters; when the capped text is stored via store_learning verbatim, the committed .uf/dewey/learnings/ file is permanently truncated mid-sentence. In PR #156, all seven learnings from one 14:11:00–14:11:10 storage batch ended cut off at ~512 characters (e.g. "...must validate the identity (", "...gained thr") while every
