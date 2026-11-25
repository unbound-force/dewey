---
tag: content-pipeline
author: jay-flowers
category: gotcha
created_at: 2026-11-07T15:54:34Z
identity: content-pipeline-20261107T155434-jay-flowers
tier: draft
---

dewey doctor output changes are user-facing output-format changes under the Website Documentation Sync rule, and the sync obligation is structurally missed when OpenSpec tasks omit it: the doctor-vertex-embedding change (dewey PR #158, fixing dewey#143) rerouted the Embedding Layer for provider: vertex away from Ollama diagnostics, yet its tasks.md carried no documentation task at all — a complete omission, unlike dewey-distill's under-scoped docs task (content-pipeline-20261104T164825). Result: the README 
