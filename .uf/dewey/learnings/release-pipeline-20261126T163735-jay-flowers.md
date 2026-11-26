---
tag: release-pipeline
author: jay-flowers
category: gotcha
created_at: 2026-11-26T16:37:35Z
identity: release-pipeline-20261126T163735-jay-flowers
tier: draft
as_of: "dewey@792e9e43b362e7f5345f318a8eb9cb54319c0f2f"
---

Opting a release pipeline into post-release verification checks (e.g. jflowers/ship go_install + tap_install_check, as in dewey release.yml #279) adds external-dependency failure modes after the artifacts are already published: go install resolves the fresh tag through the Go module proxy (proxy.golang.org/sum.golang.org index lag), and a cask-install check depends on a macOS runner that exports $HOMEBREW_PREFIX. A flaky post-release check can turn a successful release red and confuse operators about artifact validity. When enabling such checks, confirm the callee retries proxy resolution and that a failed post-release check is non-fatal to the already-tagged/published release, and update the maintainer runbook (workflow header + AGENTS.md CI/CD section) to name the new stage.
