---
tag: spec-authoring
author: jay-flowers
category: gotcha
created_at: 2026-11-07T20:14:57Z
identity: spec-authoring-20261107T201457-jay-flowers
tier: draft
---

Feedback-fix commits that re-flow or re-wrap specification prose can introduce new convention violations while fixing the original finding. In dewey PR #163 (read-tools-bare-names), the iteration-1 fix commits re-wrapped proposal.md and design.md bullets and produced lines of 73–74 characters, violating the meta-prompt pack's MP-004 72-character wrap ceiling for specification artifacts — a new MUST-rule violation introduced by the fix itself, requiring a follow-up re-wrap commit (ffe4c4f). After any edit that re-wraps lines in spec artifacts (proposal.md, design.md, tasks.md, delta specs), grep for lines exceeding the wrap ceiling (e.g. pattern ^.{73,}$ against the change directory) before requesting the next review iteration; verify only that changed lines comply, since pre-existing long lines are out of review scope.
