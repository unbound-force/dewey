---
tag: uf-unleash
author: jay-flowers
category: gotcha
created_at: 2026-11-19T00:44:32Z
identity: uf-unleash-20261119T004432-jay-flowers
tier: draft
as_of: "dewey@f7c7f99f3d8f90dbf6d681b99a18c9f38162988c"
evidence:
  - "dewey#224"
  - "dewey:.uf/artifacts/dispatch/58a46e34-ca8e-4e23-8e97-34febd63f3e8.json"
---

consolidate_dispatch rejects detail:"summary" combined with provenance ("detail: summary is only valid without provenance"). To finalize a review dispatch, pass provenance (branch, commit, workflow_id) alone — it calls finalize_review_dispatch internally and returns the compact finalization result. When the returned review_verdict field reads ESCALATED, the operation_verdict field carries the canonical decision (observed: operation_verdict APPROVE WITH ADVISORIES in both the spec-review and code-review consolidations of dewey#224); treat operation_verdict as the verdict of record.
