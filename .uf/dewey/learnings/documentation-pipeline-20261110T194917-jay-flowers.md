---
tag: documentation-pipeline
author: jay-flowers
category: pattern
created_at: 2026-11-10T19:49:17Z
identity: documentation-pipeline-20261110T194917-jay-flowers
tier: draft
---

Reconciling the user-owned AGENTS.md section in the same PR as the scaffold refresh that deployed the contradicting commands closes the documentation-pipeline loop: dewey PR #177 reworks "Learnings Direct-Commit Practice" into "Commit Scope (Learnings)" (learnings MUST NOT ride feature branches, stay untracked there, reach main via a separate chore/dewey-learnings-* PR, compiled articles still ride the feature PR, learnings stay out of .gitignore), which matches uf_finale_stage's learnings-excluding stage and /uf.address-feedback behavior — resolving the PR #167 contradiction where 4 of 5 reviewers flagged AGENTS.md documenting direct-commit of learnings that the deployed commands never do. Residual known gap left open: .uf/dewey/config.yaml remains gitignored while AGENTS.md documents it as git-tracked and shareable (upstream scaffold issue, explicitly scoped out of #177's PR body).
