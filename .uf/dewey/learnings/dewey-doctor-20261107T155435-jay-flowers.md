---
tag: dewey-doctor
author: jay-flowers
category: gotcha
created_at: 2026-11-07T15:54:35Z
identity: dewey-doctor-20261107T155435-jay-flowers
tier: draft
---

dewey doctor behavior changes must update the README `dewey doctor` checks paragraph (README.md, `### dewey doctor`): it enumerates per-layer diagnostics and carries a provider-aware parenthetical ("only Ollama availability is probed; Vertex and OpenAI-compatible gateways are checked for configuration completeness") that doctor-synthesis-diagnostics added for the synthesis layer. The opsx/doctor-vertex-embedding change (issue #143) converted the Embedding Layer for vertex to configuration-completeness-only 
