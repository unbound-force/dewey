---
tag: openspec
author: jay-flowers
category: pattern
created_at: 2026-11-10T23:27:30Z
identity: openspec-20261110T232730-jay-flowers
tier: draft
---

In dewey's OpenSpec changes, a proposal's "Modified Capabilities" declaration is not always backed by a corresponding delta: lint-anchor-drift declared a modified `lint` capability with no lint delta, while add-utf8-truncation-regression-test did carry specs/knowledge-compilation/spec.md for the existing capability it touched. When a change materially amends an existing capability's requirement contract — changing when an invariant is enforced or what a caller receives — carry the MODIFIED delta for that capability (with the Previously paragraph per the meta-prompt pack), otherwise the capability's recorded requirement text goes stale and the proposal contradicts the change's own `## MODIFIED Requirements: _None._` section. Reviewer check: diff the proposal's Capabilities section against the specs/ directory contents and the existing capability's requirement text.
