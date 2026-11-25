---
tag: documentation-accuracy
author: jay-flowers
category: gotcha
created_at: 2026-11-11T20:26:17Z
identity: documentation-accuracy-20261111T202617-jay-flowers
tier: draft
---

When a change claims byte-identical backward-compatible output, scope the claim to the outputs that actually evaluate the feature's inputs — resolution-failure refusals are outputs too. In dewey's gate-config-path-parity review (iteration 2), a council-required revision of the config-directory resolution Fix string quietly falsified the unscoped "single-path output stays byte-identical when XDG_CONFIG_HOME is unset" claim carried by AGENTS.md, README.md, and design.md, while the binding spec delta stayed accurate by promising only "the same verdict and output shape". Audit byte-identity claims against every reachable output in the named configuration, including failure-path messages that share a template with deliberately revised text.
