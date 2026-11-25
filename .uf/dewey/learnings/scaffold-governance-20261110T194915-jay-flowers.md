---
tag: scaffold-governance
author: jay-flowers
category: gotcha
created_at: 2026-11-10T19:49:15Z
identity: scaffold-governance-20261110T194915-jay-flowers
tier: draft
---

Scaffold deployments carry no .opencode test infrastructure: .opencode/lib/ (14 modules) and the plugin directories (index.ts only) contain zero test files, and .opencode/package.json has no test script or devDependencies. When a deployed module comment cites verification ("pinned by golden tests" in feedback-replies.ts), the tests live only in the upstream unbound-force repo — the claim is unverifiable from the deployment repo, and CI (Go build/test + MegaLinter) never exercises the TypeScript. The correct review disposition is to require the PR body to record the upstream test provenance (which upstream change carries the DE-007 seam tests: registration surface, golden output shapes, cache round-trips), never to write local tests for tool-owned scaffold files — local tests are divergence debt the next refresh overwrites. Note also that glob patterns do not traverse dotfile directories (.opencode, .uf) in this environment; verify paths under them with direct directory reads.
