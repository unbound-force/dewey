---
tag: lint-anchor-drift
author: jay-flowers
category: gotcha
created_at: 2026-11-09T02:22:23Z
identity: lint-anchor-drift-20261109T022223-jay-flowers
tier: draft
---

dewey lint anchor checks (issue #147): a cited path that exists at the as_of pin but is deleted or renamed away at HEAD reports anchor_drift, not anchor_missing. PathChanged runs `git diff --quiet <pin> <head> -- <path>` and a deletion is a difference (exit 1); anchor_missing fires only when the path is absent from the pin tree (git cat-file -e <sha>:<path> fails) or the pin commit is unknown. PR #159 review feedback item 15 prescribed asserting anchor_missing for a deleted-after-pin fixture — the prescription contradicted the frozen spec ("anchor_drift — the cited path changed between as_of and HEAD (including deletion)"). The correct expectation for a deleted-after-pin fixture is anchor_drift.
