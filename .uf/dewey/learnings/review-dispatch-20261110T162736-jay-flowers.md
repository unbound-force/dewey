---
tag: review-dispatch
author: jay-flowers
category: pattern
created_at: 2026-11-10T16:27:36Z
identity: review-dispatch-20261110T162736-jay-flowers
tier: draft
---

When a review-depth optimization (light check, fast path, sampled re-review) is gated on a sensitive-path list, derive the list from the repository's gate-bearing surfaces — CI workflows, the constitution, convention packs, agent definition files with permission frontmatter, and the command/plugin/skill files that implement the gates — rather than a generic category list. The change that most needs independent review is a small modification to a gate itself. In dewey PR #167, resolve_review_mode's SENSITIVE_PATH_PATTERNS covered .github/, AGENTS.md, constitution.md, and .opencode/uf/packs/ but not .opencode/commands/, .opencode/plugins/, or .opencode/agents/, so the PR's own security-fix push (a guard change in a command file, a single modified file under the 200-line delta cap, prior marker HIGH not CRITICAL) would itself have qualified for the parent-only light check instead of the full council.
