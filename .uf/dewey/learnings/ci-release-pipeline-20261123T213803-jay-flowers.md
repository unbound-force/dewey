---
tag: ci-release-pipeline
author: jay-flowers
category: gotcha
created_at: 2026-11-23T21:38:03Z
identity: ci-release-pipeline-20261123T213803-jay-flowers
tier: draft
as_of: "dewey@a19e8b4a421bc10b10b469cb326aa10fe6b16201"
---

When a release pipeline is delegated to a cross-owner reusable workflow pinned by commit SHA (e.g. jflowers/ship/.github/workflows/go-release.yml@<sha>), the SHA pin guarantees code immutability but not availability: deletion, archival, privatization of the source repo, or loss of access to a personal account breaks every consumer's release path. Review should ask for a documented recovery path (org-owned mirror or re-home plan) alongside the pin, and note that a single reusable-workflow call grants the union of all internal jobs' permissions as an upper bound — least-privilege now depends on the called workflow re-narrowing per job.
