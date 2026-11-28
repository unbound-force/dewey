---
tag: ci-pipeline
author: jay-flowers
category: gotcha
created_at: 2026-11-28T00:06:35Z
identity: ci-pipeline-20261128T000635-jay-flowers
tier: draft
as_of: "dewey@f19e39629dbe2929c37a90947e94b1f88efb9c0a"
---

When CI job-skipping detection moves from an in-repo workflow to a cross-owner reusable workflow pinned by SHA, the skip predicate's fail-closed guarantees (PR-head/merge-parent verification, --no-renames, symlink/submodule mode rejection, empty-diff -> full CI, non-PR events -> code=true) and the per-PR self-test that exercised them leave the repository's audit trail. The required gate still depends on the classification, so a later upstream regression silently routes code PRs to a secrets-scan-only path that still satisfies the aggregate required check (zero build/test). Record the upstream guarantees and the location of the upstream tests in the PR body or AGENTS.md, and verify the first post-merge push to main still runs the full build and lint jobs before relying on the new gate.
