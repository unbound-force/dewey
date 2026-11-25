---
tag: uf-unleash
author: jay-flowers
category: gotcha
created_at: 2026-11-15T01:18:46Z
identity: uf-unleash-20261115T011846-jay-flowers
tier: draft
---

Running /uf.unleash on the quick tier for dewey #194 (branch quick/require-code-owner-review-for-compiled-k, observed 2026-11-14) exposed a diff-capture gap: capture_review_diff only includes untracked files under the feature dir, and the quick tier carries no feature dir, so a newly created untracked file — the change's .github/CODEOWNERS — was silently absent from the review diff (the files array showed only AGENTS.md, missing half the change). The fix is to run `git add -N <path>` (intent-to-add) before calling capture_review_diff: the file becomes visible to the tracked-changes diff assembly without committing anything, and the finale's staging sweep completes the add later. Always compare the tool's files array against `git status --short` before dispatching reviewers and re-capture once after applying the intent-to-add marker — an incomplete review diff means reviewers never see part of the change.
