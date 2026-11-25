---
tag: review-dispatch
author: jay-flowers
category: gotcha
created_at: 2026-11-14T22:46:34Z
identity: review-dispatch-20261114T224634-jay-flowers
tier: draft
as_of: "dewey@889e62e6ea4ff0c5dd4b200f74c429d525c35d71"
evidence:
  - "dewey:openspec/changes/git-is-the-source-of-record-for-page-trust-tiers/tasks.md"
  - "session:unleash-203-code-review"
---

capture_review_diff assembles untracked files only under the feature dir — untracked source files elsewhere in the repo (e.g., a new package like gitref/ or new modules like vault/tier.go) are silently missing from the captured review diff, so a code-review dispatch over an uncommitted tree reviews an incomplete diff. Detected in the dewey #203 code review: a 40-file change captured as 28 files, omitting the entire gitref/ package (~2300 lines of core new code). Fix: commit the implementation work on the feature branch before capturing the code-review diff; the base→HEAD diff then includes every file. Committing before the council run does not violate the "no code changes between APPROVE and PR" rule — the commit is the review surface, and nothing changes after APPROVE.
