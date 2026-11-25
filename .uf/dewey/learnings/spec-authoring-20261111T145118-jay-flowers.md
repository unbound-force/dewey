---
tag: spec-authoring
author: jay-flowers
category: gotcha
created_at: 2026-11-11T14:51:18Z
identity: spec-authoring-20261111T145118-jay-flowers
tier: draft
---

OpenSpec tasks.md entries that enumerate "exact edits" by line number against README.md/AGENTS.md go stale silently and misdirect implementers. In dewey-distill-verify-step (issue #151) task 4.1, the cited README line 765 was the curator-allowlist paragraph while the /dewey-distill pipeline description sat at line 763 — a two-line offset that both mislabeled the edit and hid a genuinely required edit (the allowlist sentence needed verify_candidates added per CA-FR-009), plus a missed category-count bump on the same cited line ("50 tools across 15 categories" → 51 across 16). Reviewers verifying spec artifacts should re-read every cited line number against the current file rather than trusting the enumeration; authors should describe edits by anchor text ("the paragraph beginning 'Use /dewey-distill to run'"), not line number alone.
