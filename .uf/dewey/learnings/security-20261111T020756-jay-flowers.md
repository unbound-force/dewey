---
tag: security
author: jay-flowers
category: decision
created_at: 2026-11-11T02:07:56Z
identity: security-20261111T020756-jay-flowers
tier: draft
---

Fail-closed resolution of user-global config directories must reject relative XDG_CONFIG_HOME/HOME values, not absolutize them against the process CWD: in an agent workspace the CWD is the vault, whose contents can include indexed untrusted material, so a relative value would let workspace content masquerade as the user's global plugin config and produce a false PASS. Dewey's gate.go (resolveOpenCodeConfigDir, PR #187) refuses with "not an absolute path" and the refusal is unit-tested (TestGateAutoAnswer_RelativeXDGRefused). Related documented residual: an env-assignment prefix on the single allowed bash command (XDG_CONFIG_HOME=<dir> dewey gate auto-answer) can redirect the same lookup if the host strips leading assignments when matching — record it in the acceptance matrix rather than claiming it closed.
