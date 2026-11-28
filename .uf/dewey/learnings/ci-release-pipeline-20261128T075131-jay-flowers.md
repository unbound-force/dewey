---
tag: ci-release-pipeline
author: jay-flowers
category: gotcha
created_at: 2026-11-28T07:51:31Z
identity: ci-release-pipeline-20261128T075131-jay-flowers
tier: draft
as_of: "dewey@e1695e08c8aa51bd176ed675e07ac29e4d1a836b"
evidence:
  - "dewey#290"
---

A SHA-pinned reusable-workflow bump (e.g. jflowers/ship go-release.yml or deps-automerge.yml) changes two things at once: the immutable commit (Dependabot verifies that) and the caller/callee contract — accepted `with:` inputs, mapped secrets, and the callee's behavioral policy. Dependabot and normal build/test CI validate neither the contract nor the callee behavior, and the release pipeline is never exercised on a PR, so a renamed input or a changed merge policy surfaces only at release time (after the tag) or as an unexpected auto-merge. Several pre-1.0 minor versions in one bump (0.9.0→0.12.0) are especially suspect because pre-1.0 minors are where breaking changes are allowed, yet Dependabot's automerge guard classifies them as safely "minor". Review of such a bump must enumerate every forwarded input and secret against the new callee and confirm the documented caller-header policy still holds. Separately, when several workflows converge on one commit SHA, their version comments must converge on the same tag: a comment like "main, after v0.11.0" on a commit later tagged v0.12.0 is stale provenance that undermines rollback and pin verification.
