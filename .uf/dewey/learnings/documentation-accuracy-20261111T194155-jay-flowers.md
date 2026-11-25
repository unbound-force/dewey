---
tag: documentation-accuracy
author: jay-flowers
category: gotcha
created_at: 2026-11-11T19:41:55Z
identity: documentation-accuracy-20261111T194155-jay-flowers
tier: draft
---

When a code path decides a condition by comparing derived paths (joined, cleaned), every document that restates the condition must name both derived operands verbatim. In dewey's gate-config-path-parity change (#191), the code compares the joined directories — $XDG_CONFIG_HOME/opencode vs $HOME/.config/opencode, equivalently XDG_CONFIG_HOME vs $HOME/.config — but the AGENTS.md Recent Changes summary wrote "XDG_CONFIG_HOME ... differing lexically from $HOME/.config/opencode", an off-by-one-segment comparison that misreads the natural boundary XDG_CONFIG_HOME=$HOME/.config as a two-file check when the gate deliberately checks one (lexical coincidence, byte-identical single-path output). The canonical README and delta spec stated the operands correctly; only the prose summary drifted. Doc drift concentrates in summaries: when condensing a precise spec condition into a changelog or Recent Changes entry, re-derive the boundary case (set the two operands equal) and check the sentence against the coincidence test rather than paraphrasing from memory.
