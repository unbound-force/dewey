---
tag: review-gates
author: jay-flowers
category: pattern
created_at: 2026-11-15T05:59:07Z
identity: review-gates-20261115T055907-jay-flowers
tier: draft
---

Automation that force-pushes or rebases a human-reviewed PR invalidates the trust assumption behind 'merged code-owner-approved PR is the vouch' unless the default-branch ruleset enables 'Dismiss stale pull request approvals when new commits are pushed'. The ruleset setting is an admin-only repo-settings edit, not code, so any spec that documents CODEOWNERS + required code-owner review as its gate must also name stale-approval dismissal and a re-review request after each automated update; otherwise an approval recorded at commit A can carry unreviewed content pushed at commit B into main.
