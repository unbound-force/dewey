---
tag: openspec-spec-review
author: jay-flowers
category: gotcha
created_at: 2026-11-11T15:17:40Z
identity: openspec-spec-review-20261111T151740-jay-flowers
tier: draft
---

When an OpenSpec task asserts a concurrency-safety outcome (for example "concurrent reverse-pair writes leave no cycle"), the spec must state the mechanism, inside the write lock, that makes the racing interleaving impossible. Validation-before-lock plus narrow per-side in-closure fast-fails do not cover two-sided races: each racing call mutates the key the other call's fast-fail does not re-read. In dewey's explicit-supersession-in-compile review, supersede_learning's in-lock re-validation re-read the target's superseded_by and the successor's supersedes, so concurrent A→B and B→A both passed and wrote a 2-cycle with both learnings marked superseded. Either re-run the whole validation ladder (cycle walk included) under the lock, or make the second write conditional on both sides' link state — and state that mechanism in the requirement, not only in the design.
