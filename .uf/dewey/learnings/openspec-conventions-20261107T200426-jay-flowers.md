---
tag: openspec-conventions
author: jay-flowers
category: pattern
created_at: 2026-11-07T20:04:26Z
identity: openspec-conventions-20261107T200426-jay-flowers
tier: draft
---

In unbound-force/dewey's openspec/changes/, delta-spec section choice splits by change kind: behavior-replacement changes put their requirement under ## MODIFIED Requirements with a "Previously:" paragraph naming the replaced contract, even when no main spec exists under openspec/specs/ (precedent: ollama-host-fallback's "dewey doctor endpoint resolution"; doctor-vertex-embedding's DVE-FR-001 replacing spec-017 FR-017's vertex-shares-Ollama behavior), while new-capability changes use ## ADDED Requirements (
