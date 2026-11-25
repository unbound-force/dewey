---
tag: uf-unleash-resume
author: jay-flowers
category: pattern
created_at: 2026-11-16T19:46:51Z
identity: uf-unleash-resume-20261116T194651-jay-flowers
tier: draft
as_of: "dewey@c065dd5b2329438999360970de26e597918508ef"
---

On a resumed /uf.unleash run, durable state beats reconstructed context. uf_unleash_detect_resume derived spec-review and implement completion from checkpoint state reconciled with dispatch artifacts under .uf/artifacts/dispatch/ (artifact evidence authoritative), and the persisted checkpoint data carried phase verdicts, the pre-existing golangci-lint inventory, CI parity results, and the website-sync issue number forward from the earlier session. Re-anchoring via get_progress and parse_tasks before doing any work let the resumed run skip five completed steps and execute only the code-review gate; a stale worktree left by an interrupted earlier pre-flight was the only cleanup needed.
