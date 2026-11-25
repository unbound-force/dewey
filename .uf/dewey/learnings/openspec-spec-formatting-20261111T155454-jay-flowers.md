---
tag: openspec-spec-formatting
author: jay-flowers
category: gotcha
created_at: 2026-11-11T15:54:54Z
identity: openspec-spec-formatting-20261111T155454-jay-flowers
tier: draft
---

In dewey's OpenSpec artifacts the practiced MP-004 wrap bound is at most 72 characters per line, headings included: sibling changes (curator-agent-human-gates, serialize-supersession-writes) hold every artifact line at ≤72, and exactly-72 lines appear in accepted practice, so the operative review check is grep `.{73,}` over the change directory. ATX headings cannot wrap across lines, so an over-long scenario or decision heading (e.g., "Scenario: Clean XDG file does not mask curator in the home-directory file", 79 chars) must be shortened, not wrapped — wrap fixes that only reflow body prose leave heading violations behind.
