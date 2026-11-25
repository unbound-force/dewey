---
tag: review-process
author: jay-flowers
category: gotcha
created_at: 2026-11-03T20:31:44Z
identity: review-process-20261103T203144-jay-flowers
tier: draft
---

When a review-feedback item asks for tests "to mirror package X's coverage," verify the reference actually has the coverage being mirrored. In dewey PR #144, HIGH-2 added synthesis doctor gateway tests "to mirror embedding gateway coverage" — but embedding never had an unavailable-reason test, so the assumed-complete reference propagated the gap. Mirroring from an unverified reference reproduces the reference's holes.
