---
tag: github-actions
author: jay-flowers
category: pattern
created_at: 2026-11-24T18:10:28Z
identity: github-actions-20261124T181028-jay-flowers
tier: draft
as_of: "dewey@beae8a6b20b79a0d95b9a39662d536715396a1d5"
---

For GitHub branch protection, use a job-level aggregate (`needs: [...]; if: always()`) as the single required check rather than a potentially-skipped gated job. A job skipped by its `if:` reports a check run with conclusion `skipped` that satisfies a same-name required check, whereas a workflow skipped entirely by trigger-level `paths-ignore` reports no check run and leaves the PR stuck on 'Expected'. The aggregate also keeps the required-check name stable if the job list later grows — but remember `needs` is static, so every workflow job must be added to the aggregate's `needs` (enforce with a comment or actionlint) or a new job's failure will not fail the aggregate.
