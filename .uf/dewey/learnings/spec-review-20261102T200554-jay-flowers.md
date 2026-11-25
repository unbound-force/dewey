---
tag: spec-review
author: jay-flowers
category: gotcha
created_at: 2026-11-02T20:05:54Z
identity: spec-review-20261102T200554-jay-flowers
tier: draft
---

Cross-artifact citation reconciliation after fix rounds: when a review iteration applies independent fixes to sibling spec artifacts, a fix to one artifact can invalidate a citation another artifact relies on. In specs/017-openai-compatible-providers, the quickstart validation guide cited "the repository's local target of 70 percent documented in AGENTS.md" while a fix-extended documentation task (T040) rewrote AGENTS.md's only 70 percent documentation (the local Gaze example) to match ci.yml with no coverage floor. The contradiction survived a 20-finding fix round because each fix was verified in isolation. After a multi-fix round, grep each artifact's citations of sibling artifacts and repo documents (AGENTS.md, README.md, ci.yml) and confirm the cited content still exists post-fix.
