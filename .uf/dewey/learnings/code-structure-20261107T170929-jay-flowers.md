---
tag: code-structure
author: jay-flowers
category: pattern
created_at: 2026-11-07T17:09:29Z
identity: code-structure-20261107T170929-jay-flowers
tier: draft
---

Package tools resolves each workspace artifact directory through exactly one method on the owning tool struct — Compile.compiledDir() (tools/compile.go) is the established pattern for .uf/dewey/compiled. PR #156's learning-temporal-validity initially added a second inline derivation of .uf/dewey/learnings in markSupersededFile beside writeLearningFile's; the address-feedback loop accepted extracting learningsDir() on Learning so the location has one in-package definition and relocation is a one-line change. Two boundary facts: reIngestLearnings lives in package main (main.go) and builds the same directory itself, so an unexported tools method cannot reach it — cross-package consolidation would need an exported helper or conscious acceptance of the second definition; and deweyWorkspaceDir is independently defined as an unexported constant in both main.go and tools/indexing.go. Rule of thumb: when adding a file that touches a workspace directory another function already builds, add or reuse a dir-resolution method on the owning struct rather than repeating the filepath.Join.
