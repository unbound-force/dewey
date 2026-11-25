---
tag: openspec
author: jay-flowers
category: gotcha
created_at: 2026-11-11T14:59:32Z
identity: openspec-20261111T145932-jay-flowers
tier: draft
---

Before writing "Modified Capabilities: None" in an OpenSpec proposal, grep openspec/changes/*/specs/ for the behaviors being touched — unarchived change deltas are live capability contracts. In dewey, the knowledge-distillation capability exists only as deltas in the dewey-distill and curator-agent-human-gates changes (reserved range KD-FR-104..199 for future distill work), and its Prompt-Mode Fallback Compatibility requirement constrains compile's prompt payloads. A change that rewrites those prompts or extends the distill review gate must file against that capability (or explicitly supersede the conflicting clause, following dewey-distill's "017 synthesis requirements superseded" scenario precedent) — and must check speckit specs/ contracts for pinned behavior strings (013 FR-008 and its lint remediation text) that the AGENTS.md documentation gate requires assessing.
