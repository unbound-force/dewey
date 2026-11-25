---
tag: spec-workflow
author: jay-flowers
category: gotcha
created_at: 2026-11-11T03:41:06Z
identity: spec-workflow-20261111T034106-jay-flowers
tier: draft
---

When a change hardens a shared write path that another capability also uses (shared lock, shared backup writer, shared store column), that other capability's observable outcomes change even when the change's own summary says "outcomes unchanged" — a new refusal or warning cause on the shared leg is a contract shift for every caller. In dewey's serialize-supersession-writes change, guarding updateLearningBackup with a refusal policy changed lint's anchor-fix file leg (a new anchor_fix_warnings cause; backups recording a successor are preserved instead of overwritten), yet the proposal and tasks summarized the lint edit as "outcomes unchanged" and no lint-anchor-drift MODIFIED delta was filed. Rule: for every capability that writes through a newly guarded seam, re-read that capability's documented contract against the guard's refusal cases before writing the Impact summary, and declare a MODIFIED delta per capability touched — the summary line and the delta list should be derived from the same enumeration of guarded callers.
