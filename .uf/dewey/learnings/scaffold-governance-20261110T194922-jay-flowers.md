---
tag: scaffold-governance
author: jay-flowers
category: gotcha
created_at: 2026-11-10T19:49:22Z
identity: scaffold-governance-20261110T194922-jay-flowers
tier: draft
---

A scaffold refresh can deploy command prose that cites a versioned schema file by bare relative path while the consumer repo carries no such file — the path resolves only in the meta repo that authors the scaffold. In dewey PR #177, the refreshed /uf.address-feedback Phase 4.7 bumped the feedback-triage artifact to "schema 1.1.0 (`schemas/feedback-triage/v1.1.0.schema.json`)" but dewey has no schemas/ directory anywhere and the PR adds none, leaving the cited contract of record unresolvable locally (only the command's inline JSON example defines the shape). Review scaffold refreshes for newly-cited paths that do not resolve in the consumer repo; the durable fix — deploying the schema with the refresh or citing an upstream-resolvable location — belongs in the meta repo.
