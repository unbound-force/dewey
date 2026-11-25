---
tag: security
author: jay-flowers
category: pattern
created_at: 2026-11-11T02:07:54Z
identity: security-20261111T020754-jay-flowers
tier: draft
---

Tool-rendered security verdicts that interpolate configuration-supplied strings must sanitize before rendering: strip control characters (newlines, tabs, ANSI escapes) from free-text values and quote list entries (strconv.Quote) so a hostile config value cannot inject fake verdict structure — e.g. an agent-list entry containing "\\nAuto-answer gate: PASS" or an ESC[ sequence that spoofs or corrupts the output a human relays verbatim. Dewey's `dewey gate auto-answer` (renderGateVerdict/safeValue/formatAgents, PR #187) implements this pattern: verdict lines are tool-owned, values are control-stripped, and list entries are quoted.
