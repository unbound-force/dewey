---
tag: openspec-conventions
author: jay-flowers
category: pattern
created_at: 2026-11-07T20:46:14Z
identity: openspec-conventions-20261107T204614-jay-flowers
tier: draft
---

In the dewey repo, OpenSpec deltas use MODIFIED-without-baseline as the established convention for behavior fixes: fix-* changes (fix-chunk-overflow, fix-web-link-extraction, fix-dewey-gitignore, fix-relative-source-paths) and doctor-ux-improvement carry MODIFIED-only deltas whose Previously paragraph describes the replaced behavior, even though no main spec exists under openspec/specs/ for those capabilities. ADDED is reserved for genuinely new sections (doctor-synthesis-diagnostics added the Synthesis Layer section; doctor-vertex-embedding modified the existing Embedding Layer's Vertex path, so it used MODIFIED DVE-FR-001 with a Previously paragraph). Deltas live as flat files specs/<capability>.md inside the change directory. doctor-vertex-embedding is also the first delta to carry the MP-014 requirement-numbering allocation header (DVE-FR-001, reserved 002-010); later changes should adopt it rather than the unnumbered older deltas. Architect reviewers should not flag MODIFIED-for-behavior-fix as a delta-structure defect in this repo. Related doctor convention: provider branches mirror the Synthesis Layer switch shape (ollama/"" → vertex → openai-compatible → default), with vertex validating config completeness through the provider factory — embed.NewEmbedderFromConfig and llm.NewSynthesizerFromConfig make no network calls at construction (NewVertexEmbedder only builds an http.Client and sets a lazy tokenFn).
