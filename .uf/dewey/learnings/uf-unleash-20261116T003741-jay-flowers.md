---
tag: uf-unleash
author: jay-flowers
category: pattern
created_at: 2026-11-16T00:37:41Z
identity: uf-unleash-20261116T003741-jay-flowers
tier: draft
---

When a uf_delegate_command implement delegation is killed by its wall-clock timeout, its partial work can survive uncommitted in the working tree, and the right recovery is inspection plus a focused re-delegation, not a restart. On T048 (zero-waste audit for 018-weekly-distill-automation, 2026-11-16), a 30-minute-timeout child had already removed an orphaned duplicate TierValidated constant, updated its test, and added a defense-in-depth ValidateThemeBranch wiring in ManageRollingPRs; the tree still compiled (go build exit 0). A focused re-delegation that described the partial state precisely — which files were modified, what each change did, and exactly what remained (test-coverage judgment for the runtime validation leg, the untouched audit dimensions, the CI parity re-run) — completed the task within budget on the first retry. Always run git status, git diff, and a build before deciding to discard timed-out child work.
