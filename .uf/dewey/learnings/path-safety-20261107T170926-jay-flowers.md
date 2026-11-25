---
tag: path-safety
author: jay-flowers
category: pattern
created_at: 2026-11-07T17:09:26Z
identity: path-safety-20261107T170926-jay-flowers
tier: draft
---

When a PR adds a second writer to an existing filesystem location, extract the directory construction into one helper instead of joining the path inline at each site. Verified on dewey PR #156: markSupersededFile (tools/learning_temporal.go:244) joined {vaultPath}/.uf/dewey/learnings inline beside writeLearningFile (tools/learning.go:406). The security rationale goes beyond DRY: secondary writers — supersession back-links, backup-file updaters — fail softly (warning strings like supersedes_warning, not hard errors), so they are exactly the sites that silently bypass path hardening (containment checks, symlink guards, permission changes) later added to the primary writer. A single learningsDir()-style definition gives future hardening one anchor point covering every writer. Two boundary notes: (1) the per-component input guard (ValidLearningIdentity on the identity segment) is independent of the directory extraction and must stay at the call site — extracting the directory does not relax it; (2) a struct-method helper only deduplicates within its package — package main's reIngestLearnings (main.go) constructs the same directory independently and needs its own shared helper to be covered, so "one definition" claims should state their package scope.
