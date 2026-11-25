---
tag: dewey
author: jay-flowers
category: pattern
created_at: 2026-11-11T21:46:09Z
identity: dewey-20261111T214609-jay-flowers
tier: draft
---

A read-only verification feature reusing an existing anchor seam (Dewey's verify_candidates over lint_anchors' anchorChecker) should distinguish three pass outcomes rather than two: `checked` (ran to completion), `skipped` (condition makes the pass inapplicable, e.g. vault is not a git repository — not an error, the downstream human gate still runs), and `failed` (git error, cancellation, budget exhaustion — routed as an MCP IsError so no caller reads an unrun pass as clean, with pre-failure outcomes carried only as explicitly-unverified context). Collapsing skipped/failed into a plain error or a shorter success lets a half-run pass present as verified.
