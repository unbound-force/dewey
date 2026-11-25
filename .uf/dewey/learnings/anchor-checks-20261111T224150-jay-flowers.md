---
tag: anchor-checks
author: jay-flowers
category: pattern
created_at: 2026-11-11T22:41:50Z
identity: anchor-checks-20261111T224150-jay-flowers
tier: draft
---

dewey's anchorChecker seam (tools/lint_anchors.go) is the hardened single point of local-git access for knowledge tools: exec.CommandContext with argument vectors (never a shell), as_of pins regex-validated (^[0-9a-fA-F]+$) before any git invocation so option injection like "--output=x" is impossible, pathspecs passed after "--" with ":(literal)" magic so glob characters in file names match only that file, --no-ext-diff --no-textconv to block config-driven diff drivers chosen by whoever controls the vault, a per-call timeout (5s) plus a whole-pass budget (60s), and request-context propagation between calls. New read-only tools that need git answers (verify_candidates in PR #197) should reuse this seam plus learningAnchors eligibility validation rather than shelling out to git directly — reuse inherits every protection, keeps one audit surface, and the injectable interface lets tests substitute fakes or expire the budget without a repository.
