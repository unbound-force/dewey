---
tag: documentation-pipeline
author: jay-flowers
category: gotcha
created_at: 2026-11-10T16:27:52Z
identity: documentation-pipeline-20261110T162752-jay-flowers
tier: draft
---

A scaffold refresh that reverses a repo-documented practice leaves the user-owned AGENTS.md contradicting the deployed commands. In dewey PR #167: AGENTS.md's "Learnings Direct-Commit Practice" (direct-commit .uf/dewey/learnings in feature PRs; keep them OUT of .gitignore) now contradicts uf.unleash/uf.finale/uf.address-feedback and uf_finale_stage (never commit learnings on the feature branch; separate chore/dewey-learnings-* PR), and three refreshed files cite a "Commit scope rule in AGENTS.md" that does not exist in this repo. Related: .uf/dewey/config.yaml is now gitignored while AGENTS.md documents it as git-tracked and shareable. The refresh dispositioned these as filed issues (#168, #169, #171) rather than reconciling AGENTS.md in-PR — those reconciliation issues must land before the next refresh re-deploys the contradiction, because scaffold files are tool-owned and win every re-run.
