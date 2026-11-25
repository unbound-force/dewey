---
tag: dewey-process
author: jay-flowers
category: gotcha
created_at: 2026-11-07T16:48:24Z
identity: dewey-process-20261107T164824-jay-flowers
tier: draft
---

dewey doctor output-format changes are user-facing and trigger the AGENTS.md website documentation sync requirement (an issue in unbound-force/website). The established pattern (spec 017 T044) is search-first: run gh issue list --repo unbound-force/website to find an existing dewey provider/doctor documentation issue (website#113 tracks dewey provider docs) and reference it, filing "docs: sync dewey <feature> documentation" with the docs label only when no match exists. The change's tasks.md should carry this as an explicit verification task so the gate is auditable at review time.
