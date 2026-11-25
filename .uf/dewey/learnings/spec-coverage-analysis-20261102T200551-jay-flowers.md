---
tag: spec-coverage-analysis
author: jay-flowers
category: gotcha
created_at: 2026-11-02T20:05:51Z
identity: spec-coverage-analysis-20261102T200551-jay-flowers
tier: draft
---

When a spec promises provider-aware or config-aware diagnostics, enumerate every emission site in the codebase by grepping the message text before scoping requirements and tasks. In dewey's 017 spec review, FR-024 covered `dewey doctor` and main.go startup logging, but the `ollama pull` warning also lives in a shared helper (`createIndexEmbedder`, cli.go) used by five commands (index, reindex, compile, curate, lint) — the narrow scope let the plan's broader "CLI warnings" commitment ship with no covering task. Warning text in shared helpers reaches more commands than the surfaces named in the spec.
