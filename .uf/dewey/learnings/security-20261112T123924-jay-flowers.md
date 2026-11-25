---
tag: security
author: jay-flowers
category: pattern
created_at: 2026-11-12T12:39:24Z
identity: security-20261112T123924-jay-flowers
tier: draft
---

A security gate that mirrors another component's configuration resolution fails open when it resolves a different path than the component actually loads. dewey gate auto-answer resolved the opencode-auto-question plugin config XDG-first while the plugin hardcodes join(homedir(), ".config", "opencode", "auto-question.jsonc") — a divergent XDG_CONFIG_HOME made the gate verify a file the plugin never read while the real config (listing the curator agent) went unchecked, auto-answering the human gates the gate exists to protect (dewey #191). Pattern for fixing verifier/consumer path divergence: (1) derive the candidate set from the consumer's actual load logic, verified against the consumer's source, not from a standard (XDG spec) the consumer ignores; (2) treat every candidate as a refusal input — no clean file may mask an offending or unverifiable one — while only the candidate the consumer actually loads is authoritative for reported state; (3) prefer over-checking to under-checking: decide path coincidence by lexical comparison of constructed paths (which can only read the same physical file twice, harmlessly) rather than filepath.EvalSymlinks (filesystem-dependent, and a retargeted symlink between compare and read reintroduces a verify-one-file-read-another race); (4) tighten fail-closed when a required input becomes unobservable (unset/empty/relative HOME refuses even when XDG is set, because the consumer's passwd-database homedir fallback cannot be verified from the environment).
