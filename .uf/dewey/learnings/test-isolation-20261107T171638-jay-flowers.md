---
tag: test-isolation
author: jay-flowers
category: gotcha
created_at: 2026-11-07T17:16:38Z
identity: test-isolation-20261107T171638-jay-flowers
tier: draft
---

External-tool availability guards in tests (e.g., exec.LookPath("git")) must precede the first assertion that depends on the tool — not merely the fixture setup that uses the tool directly. In dewey's TestProductionAsOfResolver (PR #156), a "non-repository directory yields an error" assertion ran before the LookPath guard and passed vacuously on git-less machines because the resolver returns the same error outcome for "git not found" and "not a repository". CI runners had git installed, so the gap stayed latent through two review rounds. Rule of thumb: only assertions that provably short-circuit before any subprocess (like an empty-path early return) may sit above the guard; every assertion that can reach the external tool goes below it.
