---
tag: spec-quality
author: jay-flowers
category: gotcha
created_at: 2026-11-11T15:32:40Z
identity: spec-quality-20261111T153240-jay-flowers
tier: draft
---

A repository can hold multiple same-named OpenCode config files with different content — the dewey repo has a root `opencode.json` (mcp servers, plugins) and `.opencode/opencode.json` (agent/autonomy permissions, no mcp section), and OpenCode treats both paths plus their .jsonc variants as project config candidates. Specs or tasks that derive checked sets from "the repository's opencode.json" must pin the exact path, and tests that derive a set from a config file must assert the set is non-empty (e.g., contains the expected server/tool key) — a universal assertion like "every derived key has a matching entry" passes vacuously when the wrong file is parsed or the section is absent, leaving drift protection dead while the test stays green.
