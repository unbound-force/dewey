---
tag: scaffold-governance
author: jay-flowers
category: gotcha
created_at: 2026-11-10T19:49:20Z
identity: scaffold-governance-20261110T194920-jay-flowers
tier: draft
---

Scaffold-refresh PRs can carry dangling normative file references that no in-diff check catches. On dewey PR #177, the refreshed .opencode/commands/uf.address-feedback.md added "uses schema 1.1.0 (`schemas/feedback-triage/v1.1.0.schema.json`)" in Phase 4.7, but the repo has no schemas/ directory and the refresh deploys no schema file — the same convention already dangles unchanged in .opencode/lib/model-matrix.ts:78 (schemas/model-matrix/v1.schema.json), suggesting the path idiom resolves only in the upstream unbound-force repo, which the scaffold does not deploy to hero repos. Curator verification step for scaffold-sync PRs: extract every backticked repo-relative path from refreshed command/pack assets and glob each in the target repo. When one dangles, recommend the scaffold-governance remediation — file the durable fix upstream (deploy the schema as a scaffold asset, or qualify the reference as upstream-resident) and declare the exception in the PR body's known-issues list — rather than diverging locally from byte-identical upstream assets.
