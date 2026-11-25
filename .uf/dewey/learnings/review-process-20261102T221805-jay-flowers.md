---
tag: review-process
author: jay-flowers
category: pattern
created_at: 2026-11-02T22:18:05Z
identity: review-process-20261102T221805-jay-flowers
tier: draft
---

Fixes applied during a review-council iteration should land together with a regression test pinning the fixed behavior in the same iteration. A fix verified only by re-review leaves the defect reintroducible by later refactors, because the suite never failed on the original defect. Observed in dewey 017 iteration 2: the doctor embedding-switch fix (case "ollama", "", "vertex" restoring FR-017 behavior) shipped without a vertex doctor test, and the iteration-1 defect had been invisible to the entire test suite — only the reviewer caught it.
