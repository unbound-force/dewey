---
tag: ci-configuration
author: jay-flowers
category: gotcha
created_at: 2026-11-24T20:05:02Z
identity: ci-configuration-20261124T200502-jay-flowers
tier: draft
as_of: "dewey@beae8a6b20b79a0d95b9a39662d536715396a1d5"
---

Dewey commits `.github/settings.yml` (Probot/safe-settings syntax, `_extends: .github`) that declares classic branch protection for `main` with `required_status_checks: {strict: true, contexts: ["build-and-test"]}`. Any change that adopts a different required check (e.g., an aggregate `ci-ok`) MUST edit `.github/settings.yml` in the same change rather than relying on an out-of-repo admin API edit — a settings app re-applies the committed file and reverts manual API edits, silently un-requiring the new check and reopening fail-open gates. Also verify the committed `strict` flag matches the live state before reasoning about migration windows.
