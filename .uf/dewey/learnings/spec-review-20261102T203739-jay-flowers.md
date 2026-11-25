---
tag: spec-review
author: jay-flowers
category: gotcha
created_at: 2026-11-02T20:37:39Z
identity: spec-review-20261102T203739-jay-flowers
tier: draft
---

Spec-review fix rounds must cross-sweep sibling artifacts after correcting a stale documented value. When a fix removes or rewords a reference to a stale figure (e.g., dropping an AGENTS.md attribution for a coverage floor), grep the change's other artifacts for residual appeals to that figure — and check whether another task in the same change deletes the figure's only documented source. In dewey spec 017 iteration 3, quickstart §4 was reworded to cite "the repository's local target of 70 percent" while task T040 simultaneously removed --min-contract-coverage=70 from AGENTS.md, leaving the quickstart's expected outcome unanchored at merge time.
