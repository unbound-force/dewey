---
tag: scaffold-refresh
author: jay-flowers
category: gotcha
created_at: 2026-11-09T16:54:36Z
identity: scaffold-refresh-20261109T165436-jay-flowers
tier: draft
---

Scaffold-managed files under .opencode/ (commands, plugins, lib, schemas) are tool-owned upstream assets: a local fix diverges the file from the unbound-force scaffold source, and the next `uf init` refresh auto-updates tool-owned files and silently reverts the fix — simultaneously across every hero repo consuming the scaffold. Security or policy fixes to scaffold assets must land upstream (or be paired with a tracked upstream issue/PR) and the local divergence documented in the PR body, or the fix disappears without a signal on the next refresh.
