---
tag: uf-unleash
author: jay-flowers
category: pattern
created_at: 2026-11-19T00:44:35Z
identity: uf-unleash-20261119T004435-jay-flowers
tier: draft
as_of: "dewey@f7c7f99f3d8f90dbf6d681b99a18c9f38162988c"
evidence:
  - "dewey#224"
  - "dewey:.github/workflows/ci.yml"
---

Gaze's CRAPload gate can sit exactly at its limit (48/48) and still PASS. When a phase adds untested pure helpers before their tests land (dewey#224 phase 3 added parseIndexFormat/reindexDecision before phase 4's tests), the right move is to proceed and write the tests — CRAP falls as coverage rises, so the tests that close the coverage gap also lower the gate reading (observed 48/48 → 47/48 after phase 4's tests landed). Do not refactor production code to appease the gate when missing coverage is the actual cause; the fix loop for a change-caused CRAPload breach is writing the tests, not simplifying the helpers.
