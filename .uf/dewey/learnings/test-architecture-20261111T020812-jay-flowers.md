---
tag: test-architecture
author: jay-flowers
category: pattern
created_at: 2026-11-11T02:08:12Z
identity: test-architecture-20261111T020812-jay-flowers
tier: draft
---

gateOptions-style injection (configDir, workDir, lookupEnv fields with empty-value fallbacks to the real environment) let dewey's `gate auto-answer` grow a 25-case pass/refuse/fail-closed matrix with zero t.Setenv mutation, zero cwd chdir, and full t.TempDir isolation. The pattern's cost: tests that always inject paths never execute the empty-option fallback resolution that real CLI invocations take — the env→directory success joins (XDG→/opencode, HOME→/.config/opencode) went untested. When using fallback-injection options, add at least one test per fallback success path that drives resolution through the injected env lookup end to end.
