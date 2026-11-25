---
tag: test-quality
author: jay-flowers
category: gotcha
created_at: 2026-11-02T21:27:30Z
identity: test-quality-20261102T212730-jay-flowers
tier: draft
---

When a tasks.md test task enumerates symmetric per-capability deliverables (e.g., spec 017 T035: doctor tests for "a complete openai-compatible embedding and synthesis configuration"), diff the delivered test function names against the enumeration before flipping the checkbox to [x]. The skipped twin is usually the one whose production code shipped in the same commit and looks covered by its sibling (embedding doctor tests existed; synthesis doctor tests did not). A quick audit hook: grep each new exported helper's call sites — an exported function whose only call site lives inside an untested branch (llm.HasAPIKeyField -> cli.go synthesis doctor branch) has zero effective coverage even though the package test count looks healthy.
