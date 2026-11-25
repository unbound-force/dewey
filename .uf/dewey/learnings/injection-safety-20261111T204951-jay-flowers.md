---
tag: injection-safety
author: jay-flowers
category: pattern
created_at: 2026-11-11T20:49:51Z
identity: injection-safety-20261111T204951-jay-flowers
tier: draft
---

Dewey's anchorChecker seam (tools/lint_anchors.go) is the canonical recipe for feeding untrusted markdown frontmatter into local git safely, and verify_candidates (issue #151) inherited every protection by reusing the seam rather than reimplementing git access: (1) validate revisions against ^[0-9a-fA-F]+$ before they ever become git arguments, so a malicious as_of pin like "--output=x" is rejected at the eligibility gate; (2) pass pathspecs after "--" prefixed with ":(literal)" magic so glob characters and pathspec magic in file names match only that file; (3) always use exec.CommandContext with argument lists, never a shell; (4) disable "--no-ext-diff --no-textconv" so repository-config-controlled diff drivers and textconv filters (which run attacker-chosen commands) cannot execute; (5) bound each subprocess with a 5s timeout and the whole pass with a 60s budget, honoring the request context between calls. Any new consumer of untrusted evidence locators (a doctor anchor pass would be the third) should reuse this seam; the rule-of-three extraction bar for the pass runner itself is documented in dewey-distill-verify-step design.md D2.
