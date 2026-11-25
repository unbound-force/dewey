---
tag: gate-parity
author: jay-flowers
category: gotcha
created_at: 2026-11-11T15:19:38Z
identity: gate-parity-20261111T151938-jay-flowers
tier: draft
---

When a deterministic gate derives its verification target from environment variables and is invoked by the very agent it polices, a path-parity fix that moves the authoritative lookup also moves the env-spoofing vector — the residual must be restated every time the lookup contract is rewritten. dewey #191 fixed `dewey gate auto-answer` checking XDG_CONFIG_HOME while the opencode-auto-question plugin always reads $HOME/.config/opencode (Node os.homedir() never consults XDG). But the parent change (curator-agent-human-gates D2) had documented that an env-assignment prefix on the gate's single allowed bash command (`XDG_CONFIG_HOME=<dir> dewey gate auto-answer`) matches the allow rule if the host strips leading assignments when matching, redirecting the lookup at an attacker-chosen directory for a false PASS. After the fix makes HOME always-checked and authoritative, the vector migrates to `HOME=<clean-dir>` — still a single variable when XDG_CONFIG_HOME is unset (the common case), and the target can be an absolute path into the vault (CWD masquerade; the relative-value refusal does not cover absolute prefixes). Rule: state the gate-process/plugin-process env-parity assumption explicitly in the gate's spec, and treat 'which env var redirects the check' as a property that changes with every rewrite of the resolution contract (related: learning/security-20261111T020756-jay-flowers).
