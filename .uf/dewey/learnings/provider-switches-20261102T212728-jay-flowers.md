---
tag: provider-switches
author: jay-flowers
category: gotcha
created_at: 2026-11-02T21:27:28Z
identity: provider-switches-20261102T212728-jay-flowers
tier: draft
---

When adding a new provider case to a provider-dispatching switch (e.g., `dewey doctor`'s embedding/synthesis layers), enumerate every already-supported provider explicitly rather than relying on a `default:` branch. A default that reports "unknown provider" will silently regress existing providers that were previously handled by unscoped code — in spec 017 the new embedding switch omitted `case "vertex"`, so a valid Vertex configuration starting printing `FAIL unknown provider "vertex" (supported: ollama, vertex, openai-compatible)` in `dewey doctor`, contradicting FR-017's "preserve existing behaviour for ollama and vertex". Check both parallel switches for symmetry and verify one case per supported provider value.
