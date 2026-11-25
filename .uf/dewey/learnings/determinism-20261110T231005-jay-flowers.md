---
tag: determinism
author: jay-flowers
category: pattern
created_at: 2026-11-10T23:10:05Z
identity: determinism-20261110T231005-jay-flowers
tier: draft
---

Deterministic procedures added to slash commands (config resolution across project/global scopes, plugin-entry lookup, agent-list comparison, pass/refuse decisions) are DE-001/MP-022 mechanical instruction: route them to an executable tool with tests, or record a human-authorized deviation. `dewey-publish` is the model — deterministic sweep in `dewey artifacts plan|sync`, judgment (human gate) in command prose. A prose-only startup gate depends on the LLM correctly re-implementing JSONC parsing, which is the failure mode the gate exists to prevent.
