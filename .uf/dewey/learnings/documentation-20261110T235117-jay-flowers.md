---
tag: documentation
author: jay-flowers
category: gotcha
created_at: 2026-11-10T23:51:17Z
identity: documentation-20261110T235117-jay-flowers
tier: draft
---

In the dewey repository, adding a CLI subcommand requires two AGENTS.md updates, not one: a row in the CLI Commands table and the subcommand name appended to the cli.go parenthetical comment in the Architecture section. The parenthetical conventionally lists every subcommand (e.g. `artifacts` was appended when `dewey artifacts` shipped), so a documentation task that names only the table row leaves the Architecture section stale. Spec documentation tasks for new subcommands should name both edit sites.
