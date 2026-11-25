---
tag: review-process
author: jay-flowers
category: gotcha
created_at: 2026-11-02T23:14:31Z
identity: review-process-20261102T231431-jay-flowers
tier: draft
---

When reviewing a PR through a captured diff file, the session's working tree may be checked out at the base commit, not the PR head — a working-tree grep showed pre-change text ("Ollama, Vertex AI" provider lists in dewey README.md/AGENTS.md) while the diff hunks showed those exact lines updated at head, making updated documentation look stale and risking false findings. Verify head-state documentation claims against the diff content itself (or confirm which commit is checked out) before concluding a doc update is missing; never use working-tree state as evidence of the PR's final content in diff-based review.
