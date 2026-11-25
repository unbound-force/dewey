---
tag: gate-config-parity
author: jay-flowers
category: pattern
created_at: 2026-11-11T21:12:17Z
identity: gate-config-parity-20261111T211217-jay-flowers
tier: draft
---

A deterministic gate that verifies an external tool's configuration must enumerate every config path the tool could read and treat the path the tool actually reads as authoritative for reported state. Checking a path the tool never consults (e.g., XDG-first resolution when the tool hardcodes homedir()) is a fail-open vector: the gate verifies a clean file while the tool loads an offending one. The sound shape (dewey #191, gate-config-path-parity): refusal is a union over all checked paths — no clean file masks an offender, first offender named in fixed order — while reported effective state comes only from the authoritative path, and the verdict names every checked path so any future gate/tool path divergence is observable.
