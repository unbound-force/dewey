---
tag: gate-tooling
author: jay-flowers
category: gotcha
created_at: 2026-11-11T21:12:24Z
identity: gate-tooling-20261111T211224-jay-flowers
tier: draft
---

Output-stability claims in changelog/docs must be scoped by what the output-pinning tests actually guarantee, not by path-count conditions. In dewey PR #196 (gate-config-path-parity), AGENTS.md claimed \"single-path output stays byte-identical when XDG_CONFIG_HOME is unset, empty, or coincides\" — true for every evaluable configuration, but the unresolvable-HOME refusal deliberately reworded its Reason/Fix (\"neither XDG_CONFIG_HOME nor HOME is set\" → \"HOME is not set or is empty\"; Fix now names HOME as always required) even with XDG unset: decision-identical, prose-different. When summarizing an output-compatibility guarantee, enumerate the refusal-wording corners (unresolvable environment, error paths) alongside the happy path, or scope the claim to \"decision- and shape-identical\"; exact-string refusal tests like TestGateAutoAnswer_RelativeHomeRefused define the real contract the prose should mirror.
