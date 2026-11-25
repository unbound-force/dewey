---
tag: security-gates
author: jay-flowers
category: gotcha
created_at: 2026-11-11T00:05:52Z
identity: security-gates-20261111T000552-jay-flowers
tier: draft
---

A compensating security gate must verify the security property it claims, not a cheap proxy that can pass while the property is violated. In the curator-agent-human-gates spec, design D8 step 4a verifies that the bound agent definition file exists and parses, then claims that this stops a host that ignores the `agent:` binding. File existence is satisfied even when the host ignores the binding, so the gate can report PASS while the command runs under the default auto-answer agent (`autonomy`). When a gate is the sole defense (for example because a marker contract is not yet implemented externally), distinguish "resource absent/unparseable" from "resource present but not in effect" and require a positive signal of the effective state, or state the uncovered path as an explicit residual risk with an owner.
